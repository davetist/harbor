package supervisor

import (
	"context"
	"errors"
	"fmt"
	"github.com/creack/pty"
	"harbor/internal/model"
	"harbor/internal/pier"
	vr "harbor/internal/runtime"
	"harbor/internal/store"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

type Launcher func(string, ...string) *exec.Cmd
type child struct {
	cmd          *exec.Cmd
	tty          *os.File
	done         chan struct{}
	record       model.ProcessRecord
	release      func()
	stopping     bool
	initializing bool
}
type Supervisor struct {
	store     *store.Store
	launch    Launcher
	mu        sync.Mutex
	children  map[model.ID]*child
	ports     map[int]bool
	logs      map[model.ID]*LogBuffer
	logErrors map[model.ID]bool
	OnLog     func(model.Event)
	OnChange  func(model.Event)
}

func New(st *store.Store, l Launcher) *Supervisor {
	if l == nil {
		l = exec.Command
	}
	return &Supervisor{store: st, launch: l, children: make(map[model.ID]*child), ports: make(map[int]bool), logs: make(map[model.ID]*LogBuffer), logErrors: make(map[model.ID]bool)}
}
func (s *Supervisor) buffer(id model.ID) *LogBuffer {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.logs[id] == nil {
		s.logs[id] = &LogBuffer{publish: func(p []byte) {
			if s.OnLog != nil {
				if len(p) > 8192 {
					p = p[len(p)-8192:]
				}
				s.OnLog(model.Event{Type: "log", ShipID: id, Data: string(p)})
			}
		}}
	}
	return s.logs[id]
}
func (s *Supervisor) Logs(id model.ID) string { return s.buffer(id).String() }
func (s *Supervisor) Start(ctx context.Context, jobID, shipID model.ID) error {
	ship, e := s.store.GetShip(ctx, shipID)
	if e != nil {
		return e
	}
	args, e := vr.BuildArgs(ship, "start")
	if e != nil {
		return e
	}
	return s.LaunchShip(ctx, jobID, ship, args)
}
func (s *Supervisor) LaunchShip(ctx context.Context, jobID model.ID, ship model.Ship, args []string) error {
	return s.launchShip(ctx, jobID, ship, args, false)
}
func (s *Supervisor) LaunchNewShip(ctx context.Context, jobID model.ID, ship model.Ship, args []string) error {
	return s.launchShip(ctx, jobID, ship, args, true)
}
func (s *Supervisor) launchShip(ctx context.Context, jobID model.ID, ship model.Ship, args []string, newDestination bool) error {
	if err := s.ValidatePier(ship, newDestination); err != nil {
		return err
	}
	if ship.RecoveryBlocked {
		return errors.New(ship.RecoveryBlockReason)
	}
	s.mu.Lock()
	_, exists := s.children[ship.ID]
	s.mu.Unlock()
	if exists {
		return errors.New("ship already has an owned process")
	}
	if locked, e := pier.WriterLocked(ship.PierPath); e != nil || locked {
		return errors.New("pier has an external writer or unavailable lock")
	}
	release, e := pier.Guard(ship.PierPath)
	if e != nil {
		return errors.New("pier is already managed by another Harbor process")
	}
	mapArgs, e := vr.LMDBMapArgs(ctx, ship.PierPath, newDestination)
	if e != nil {
		release()
		return e
	}
	args = append(mapArgs, args...)
	port, releasePort, e := s.reserveHTTPPort(ctx, ship.HTTPBindAddress)
	if e != nil {
		release()
		return e
	}
	releasePier := release
	release = func() { releasePort(); releasePier() }
	ship.WebPort = port
	portSet := false
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--http-port" {
			args[i+1] = fmt.Sprint(port)
			portSet = true
			break
		}
	}
	if !portSet {
		args = append([]string{"--http-port", fmt.Sprint(port)}, args...)
	}
	digest, e := vr.FileDigest(ship.RuntimePath)
	if e != nil {
		release()
		return e
	}
	record := model.ProcessRecord{LaunchID: model.NewID(), JobID: jobID, ShipID: ship.ID, Kind: "ship", Operation: "start", Phase: "launching", BinaryDigest: digest}
	if e = s.store.PutProcess(ctx, record); e != nil {
		release()
		return e
	}
	ship.State = "starting"
	ship.Error = ""
	if e = s.store.PutShip(ctx, ship); e != nil {
		release()
		return e
	}
	cmd := s.launch(ship.RuntimePath, args...)
	cmd.Env = append(cmd.Environ(), "TERM=dumb")
	tty, e := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 100})
	if e != nil {
		release()
		record.Phase = "exited"
		exit := -1
		record.ExitCode = &exit
		s.store.PutProcess(context.Background(), record)
		ship.State = "failed"
		ship.Error = "Runtime could not start."
		s.store.PutShip(context.Background(), ship)
		return errors.New(ship.Error)
	}
	record.PID = cmd.Process.Pid
	record.StartIdentity, e = StartIdentity(record.PID)
	record.Phase = "live"
	c := &child{cmd: cmd, tty: tty, done: make(chan struct{}), record: record, release: release, initializing: newDestination}
	s.mu.Lock()
	s.children[ship.ID] = c
	s.mu.Unlock()
	log := s.JobLogWriter(ship.ID, jobID)
	go io.Copy(log, tty)
	go s.wait(ship.ID, c)
	if e != nil || s.store.PutProcess(context.Background(), record) != nil {
		s.hold(ship.ID, "Process identity could not be persisted; verify recovery before reuse.")
		return errors.New("process identity could not be recorded")
	}
	client := &http.Client{Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	timer := time.NewTicker(250 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case <-c.done:
			return errors.New("runtime exited before the ship became ready")
		case <-ctx.Done():
			return errors.New("ship readiness timed out; process remains tracked")
		case <-timer.C:
			address := ship.HTTPBindAddress
			if address == "0.0.0.0" {
				address = "127.0.0.1"
			}
			req, _ := http.NewRequestWithContext(ctx, "GET", "http://"+net.JoinHostPort(address, fmt.Sprint(ship.WebPort))+"/~/login", nil)
			resp, err := client.Do(req)
			if err != nil {
				continue
			}
			io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			if resp.StatusCode < 200 || resp.StatusCode >= 400 {
				continue
			}
			select {
			case <-c.done:
				return errors.New("runtime exited during readiness")
			default:
			}
			current, err := s.store.GetShip(context.Background(), ship.ID)
			if err != nil {
				return err
			}
			identityContext, cancel := context.WithTimeout(ctx, 10*time.Second)
			identity, lookupErr := vr.ReadIdentity(identityContext, current)
			cancel()
			if lookupErr != nil {
				continue // Public HTTP can answer while the worker is still initializing.
			}
			current.State = "running"
			current.Identity = identity
			identityContext, cancel = context.WithTimeout(ctx, 10*time.Second)
			if capabilities, lookupErr := vr.ProbeCapabilities(identityContext, current.RuntimePath); lookupErr == nil {
				current.RuntimeVersion = "Vere " + capabilities.Version
			}
			cancel()
			s.mu.Lock()
			if s.children[ship.ID] != c {
				s.mu.Unlock()
				return errors.New("runtime exited during readiness")
			}
			if c.stopping {
				s.mu.Unlock()
				return errors.New("ship start interrupted by graceful stop; process remains tracked")
			}
			c.initializing = false
			if err = s.store.PutShip(context.Background(), current); err != nil {
				s.mu.Unlock()
				return err
			}
			s.changed(current)
			s.mu.Unlock()
			return nil
		}
	}
}

func (s *Supervisor) ValidatePier(ship model.Ship, newDestination bool) error {
	return pier.VerifyRegistered(ship.PierPath, newDestination)
}
func (s *Supervisor) wait(id model.ID, c *child) {
	e := c.cmd.Wait()
	code := 0
	if e != nil {
		code = c.cmd.ProcessState.ExitCode()
	}
	c.record.Phase = "exited"
	c.record.ExitCode = &code
	s.store.PutProcess(context.Background(), c.record)
	c.tty.Close()
	c.release()
	s.mu.Lock()
	stopping := c.stopping
	delete(s.children, id)
	ship, err := s.store.GetShip(context.Background(), id)
	if err == nil {
		if ship.RecoveryBlocked {
			ship.State = "failed"
		} else if stopping && code == 0 {
			ship.State = "stopped"
		} else {
			ship.State = "failed"
			ship.Error = "Runtime exited. Inspect the terminal before restarting."
		}
		err = s.store.PutShip(context.Background(), ship)
	}
	s.mu.Unlock()
	if err == nil {
		s.changed(ship)
	}
	close(c.done)
}
func (s *Supervisor) changed(ship model.Ship) {
	if s.OnChange != nil {
		s.OnChange(model.Event{Type: "ship", ShipID: ship.ID, Data: ship})
	}
}
func (s *Supervisor) Stop(ctx context.Context, id model.ID) error {
	s.mu.Lock()
	c := s.children[id]
	if c != nil {
		c.stopping = true
	}
	s.mu.Unlock()
	if c == nil {
		ship, e := s.store.GetShip(ctx, id)
		if e != nil {
			return e
		}
		if ship.State == "externally_running" {
			return errors.New("external process must be stopped by its owner")
		}
		locked, e := pier.WriterLocked(ship.PierPath)
		if e != nil || locked {
			return errors.New("pier has an external writer")
		}
		return nil
	}
	identity, e := StartIdentity(c.record.PID)
	if e != nil {
		select {
		case <-c.done:
			return nil
		default:
			return errors.New("process exit has not been confirmed")
		}
	}
	if identity != c.record.StartIdentity {
		return errors.New("process identity changed; refusing to signal")
	}
	s.mu.Lock()
	ship, e := s.store.GetShip(ctx, id)
	if e == nil {
		if !ship.RecoveryBlocked {
			ship.State = "stopping"
		}
		e = s.store.PutShip(ctx, ship)
	}
	s.mu.Unlock()
	if e != nil {
		return e
	}
	// Vere's first boot can receive TERM before its pier is registered. That
	// request is ignored by its empty pier list. Wait for a private worker reply
	// before signaling a new boot, including when its launch context was canceled.
	s.mu.Lock()
	initializing := c.initializing
	s.mu.Unlock()
	if initializing {
		for {
			select {
			case <-c.done:
				if c.cmd.ProcessState.ExitCode() != 0 {
					return errors.New("runtime did not exit cleanly")
				}
				return nil
			case <-ctx.Done():
				return errors.New("graceful bootstrap stop interrupted; process remains tracked")
			default:
			}
			query, cancel := context.WithTimeout(ctx, 10*time.Second)
			_, err := vr.ReadIdentity(query, ship)
			cancel()
			if err == nil {
				break
			}
			select {
			case <-c.done:
				continue
			case <-ctx.Done():
				return errors.New("graceful bootstrap stop interrupted; process remains tracked")
			case <-time.After(250 * time.Millisecond):
			}
		}
		// Confirm ownership again after waiting; never signal a recycled PID.
		identity, e = StartIdentity(c.record.PID)
		if e != nil || identity != c.record.StartIdentity {
			select {
			case <-c.done:
				if c.cmd.ProcessState.ExitCode() == 0 {
					return nil
				}
			default:
			}
			return errors.New("process identity changed or exit is unconfirmed; refusing to signal")
		}
	}
	if e = c.cmd.Process.Signal(syscall.SIGTERM); e != nil {
		return e
	}
	select {
	case <-c.done:
		if c.cmd.ProcessState.ExitCode() != 0 {
			return errors.New("runtime did not exit cleanly")
		}
		return nil
	case <-ctx.Done():
		return errors.New("graceful stop interrupted; no utility launched")
	}
}
func (s *Supervisor) WriteInput(ctx context.Context, id model.ID, input []byte) error {
	if len(input) == 0 || len(input) > 8192 {
		return errors.New("terminal input must contain 1–8192 bytes")
	}
	s.mu.Lock()
	c := s.children[id]
	s.mu.Unlock()
	if c == nil {
		return errors.New("terminal is not attached; restart an exclusively stopped ship to attach")
	}
	c.tty.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_, e := c.tty.Write(input)
	return e
}
func (s *Supervisor) ResizeTerminal(ctx context.Context, id model.ID, rows, cols uint16) error {
	if rows < 1 || cols < 1 || rows > 500 || cols > 500 {
		return errors.New("invalid terminal size")
	}
	s.mu.Lock()
	c := s.children[id]
	s.mu.Unlock()
	if c == nil {
		return errors.New("terminal is not attached")
	}
	return pty.Setsize(c.tty, &pty.Winsize{Rows: rows, Cols: cols})
}

// HoldFailedRestart serializes recovery state with owned-process exit and stop
// updates. A readiness failure must not be erased by a stale exit snapshot.
func (s *Supervisor) HoldFailedRestart(ctx context.Context, id model.ID, failure error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ship, err := s.store.GetShip(ctx, id)
	if err != nil {
		return err
	}
	ship.RecoveryBlocked = true
	ship.State = "failed"
	ship.Error = failure.Error()
	if ship.RecoveryBlockReason == "" {
		ship.RecoveryBlockReason = "Utility completed but its restart failed. Pier and checkpoints remain held for recovery verification."
	}
	return s.store.PutShip(ctx, ship)
}
func (s *Supervisor) hold(id model.ID, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, e := s.store.GetShip(context.Background(), id)
	if e == nil {
		v.RecoveryBlocked = true
		v.RecoveryBlockReason = reason
		v.State = "failed"
		s.store.PutShip(context.Background(), v)
	}
}
func (s *Supervisor) Observe(ctx context.Context) error {
	records, e := s.store.ListOpenProcesses(ctx)
	if e != nil {
		return e
	}
	for _, p := range records {
		ship, e := s.store.GetShip(ctx, p.ShipID)
		if e != nil {
			return e
		}
		identity, e := StartIdentity(p.PID)
		if p.StartIdentity != "" && e == nil && identity == p.StartIdentity {
			ship.State = "externally_running"
			ship.Error = "A recorded process survived. Its terminal cannot be reattached."
		} else if p.Phase == "launching" || p.Kind == "utility" {
			ship.RecoveryBlocked = true
			ship.RecoveryBlockReason = "A previous process launch or utility has an uncertain outcome."
			ship.State = "failed"
		} else {
			p.Phase = "exited"
			if e = s.store.PutProcess(ctx, p); e != nil {
				return e
			}
			ship.State = "stopped"
		}
		if e = s.store.PutShip(ctx, ship); e != nil {
			return e
		}
	}
	ships, e := s.store.ListShips(ctx)
	if e != nil {
		return e
	}
	for _, ship := range ships {
		if locked, e := pier.WriterLocked(ship.PierPath); e != nil {
			return e
		} else if locked && ship.State != "externally_running" {
			ship.State = "externally_running"
			ship.Error = "Pier is locked by another runner."
			if e = s.store.PutShip(ctx, ship); e != nil {
				return e
			}
		}
	}
	return nil
}
func (s *Supervisor) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	ids := make([]model.ID, 0, len(s.children))
	for id := range s.children {
		ids = append(ids, id)
	}
	s.mu.Unlock()
	var errs []error
	for _, id := range ids {
		if e := s.Stop(ctx, id); e != nil {
			errs = append(errs, e)
		}
	}
	return errors.Join(errs...)
}
