package jobs

import (
	"context"
	"errors"
	"harbor/internal/model"
	"harbor/internal/pier"
	vr "harbor/internal/runtime"
	"harbor/internal/scheduler"
	"harbor/internal/store"
	"harbor/internal/supervisor"
	"os/exec"
	"sync"
	"time"
)

type Executor struct {
	store         *store.Store
	supervisor    *supervisor.Supervisor
	mu            sync.Mutex
	ready         bool
	runMu         sync.Mutex
	Hooks         map[model.Operation]func(context.Context, *model.Job) error
	OnChange      func(model.Event)
	verifyOffline func(context.Context, model.Ship, model.ID) error
	resources     func(context.Context, string) (scheduler.Resources, error)
}

func New(st *store.Store, s *supervisor.Supervisor) *Executor {
	e := &Executor{store: st, supervisor: s, Hooks: make(map[model.Operation]func(context.Context, *model.Job) error)}
	e.verifyOffline = e.inspectOffline
	e.resources = scheduler.ReadResources
	return e
}
func (e *Executor) SetReady(ready bool) { e.mu.Lock(); defer e.mu.Unlock(); e.ready = ready }
func (e *Executor) Submit(ctx context.Context, r model.JobRequest) (model.ID, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.ready {
		return "", errors.New("Harbor is recovering or shutting down")
	}
	if r.Trigger != "manual" && r.Trigger != "system" {
		return "", errors.New("invalid job trigger")
	}
	switch r.Kind {
	case "start", "stop", "pack", "meld", "roll", "chop":
	default:
		if e.Hooks[r.Kind] == nil {
			return "", errors.New("operation is not available")
		}
	}
	ship, err := e.store.GetShip(ctx, r.ShipID)
	if err != nil {
		return "", err
	}
	if ship.RecoveryBlocked && r.Kind != "backup" {
		return "", errors.New(ship.RecoveryBlockReason)
	}
	if ship.State == "externally_running" {
		return "", errors.New("stop the other runner before using Harbor")
	}
	pending, err := e.store.HasPendingJob(ctx, r.ShipID)
	if err != nil {
		return "", err
	}
	if pending {
		return "", errors.New("this ship already has an active or queued job")
	}
	j := model.Job{ID: model.NewID(), ShipID: r.ShipID, Kind: r.Kind, Trigger: r.Trigger, Phase: "queued", UploadID: r.UploadID}
	if err = e.store.CreateJob(ctx, j); err != nil {
		return "", err
	}
	e.emit(model.Event{Type: "job", ShipID: j.ShipID, JobID: j.ID, Data: j})
	return j.ID, nil
}
func (e *Executor) emit(v model.Event) {
	if e.OnChange != nil {
		e.OnChange(v)
	}
}
func (e *Executor) phase(ctx context.Context, j *model.Job, phase string) error {
	j.Phase = phase
	if err := e.store.UpdateJob(ctx, *j); err != nil {
		return err
	}
	e.emit(model.Event{Type: "job", ShipID: j.ShipID, JobID: j.ID, Data: j})
	return nil
}
func (e *Executor) RunOne(ctx context.Context) error {
	e.runMu.Lock()
	defer e.runMu.Unlock()
	e.mu.Lock()
	ready := e.ready
	e.mu.Unlock()
	if !ready {
		return nil
	}
	j, err := e.store.ClaimJob(ctx)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	j.StartedAt = time.Now().UTC()
	if err = e.phase(ctx, &j, "preflight"); err != nil {
		return err
	}
	if j.Kind == "import_archive" {
		hook := e.Hooks[j.Kind]
		if hook == nil {
			return e.finish(ctx, &j, "failed", errors.New("archive importer is unavailable"))
		}
		if err = e.phase(ctx, &j, "executing"); err != nil {
			return err
		}
		err = hook(ctx, &j)
		if err != nil {
			return e.finish(ctx, &j, "failed", err)
		}
		return e.finish(ctx, &j, "completed", nil)
	}
	ship, err := e.store.GetShip(ctx, j.ShipID)
	if err == nil && ship.RecoveryBlocked && j.Kind != "backup" {
		err = errors.New(ship.RecoveryBlockReason)
	}
	if err == nil && ship.State == "externally_running" {
		err = errors.New("pier has an external writer")
	}
	if err != nil {
		return e.finish(ctx, &j, "failed", err)
	}
	if j.Trigger == "scheduled" && !ship.DesiredRunning {
		return e.finish(ctx, &j, "skipped", errors.New("ship was intentionally stopped"))
	}
	if j.DependsOnJobID != "" {
		dep, de := e.store.GetJob(ctx, j.DependsOnJobID)
		if de != nil || dep.ShipID != j.ShipID || dep.Kind != "roll" || dep.Phase != "completed" {
			return e.finish(ctx, &j, "skipped", errors.New("the paired Roll did not complete successfully"))
		}
	}
	j.PriorRunning = ship.State == "running"
	if isMaintenance(j.Kind) {
		if err = e.checkResources(ctx, j, ship); err != nil {
			phase := "failed"
			if j.Trigger == "scheduled" {
				phase = "skipped"
			}
			return e.finish(ctx, &j, phase, err)
		}
	}
	if err = e.phase(ctx, &j, "executing"); err != nil {
		return err
	}
	if hook := e.Hooks[j.Kind]; hook != nil {
		err = hook(ctx, &j)
	} else {
		switch j.Kind {
		case "start":
			ship.DesiredRunning = true
			err = e.store.PutShip(ctx, ship)
			if err == nil {
				readyCtx, cancel := context.WithTimeout(ctx, 20*time.Minute)
				err = e.supervisor.Start(readyCtx, j.ID, ship.ID)
				cancel()
			}
		case "stop":
			ship.DesiredRunning = false
			err = e.store.PutShip(ctx, ship)
			if err == nil {
				err = e.supervisor.Stop(ctx, ship.ID)
			}
		case "pack", "meld", "roll", "chop":
			err = e.maintain(context.WithoutCancel(ctx), &j, ship)
		default:
			err = errors.New("unsupported durable job")
		}
	}
	if err != nil {
		return e.finish(ctx, &j, "failed", err)
	}
	return e.finish(ctx, &j, "completed", nil)
}
func (e *Executor) finish(ctx context.Context, j *model.Job, phase string, err error) error {
	j.FinishedAt = time.Now().UTC()
	if err != nil {
		j.Error = err.Error()
	}
	if update := e.phase(context.WithoutCancel(ctx), j, phase); update != nil {
		return update
	}
	if ship, se := e.store.GetShip(context.WithoutCancel(ctx), j.ShipID); se == nil {
		if e.supervisor.ValidatePier(ship, false) == nil {
			measure, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			if size, err := pier.MeasureBytes(measure, ship.PierPath); err == nil {
				e.store.RecordMeasurement(context.Background(), ship.ID, size, time.Now())
			}
			cancel()
		}
		if current, err := e.store.GetShip(context.Background(), ship.ID); err == nil {
			ship = current
		}
		e.emit(model.Event{Type: "ship", ShipID: ship.ID, Data: ship})
	}
	return nil
}
func (e *Executor) maintain(ctx context.Context, j *model.Job, ship model.Ship) error {
	args, err := vr.BuildArgs(ship, j.Kind)
	if err != nil {
		return err
	}
	return e.maintainWith(ctx, j, ship, args, nil, nil)
}
func isMaintenance(kind model.Operation) bool {
	return kind == "pack" || kind == "meld" || kind == "roll" || kind == "chop"
}
func (e *Executor) checkResources(ctx context.Context, j model.Job, ship model.Ship) error {
	r, err := e.resources(ctx, ship.PierPath)
	if err != nil {
		return errors.New("host resources could not be measured; utility was not started")
	}
	r.LoomExponent = ship.LoomExponent
	r.RuntimeVersion = vr.StableVersion
	if ship.RuntimeVersion != "" && ship.RuntimeVersion != "Vere "+vr.StableVersion {
		return scheduler.ErrUnvalidatedTarget
	}
	target, err := vr.NativePlatform()
	if err != nil {
		return err
	}
	return scheduler.Preflight(target, r, j.Kind, ship.State == "running", j.Trigger == "scheduled")
}
func (e *Executor) maintainWith(ctx context.Context, j *model.Job, ship model.Ship, args []string, before func(context.Context, model.Ship) error, after func(context.Context, *model.Ship) error) error {
	var err error
	if err = e.supervisor.ValidatePier(ship, false); err != nil {
		return err
	}
	if _, err = pier.Inspect(ship.PierPath); err != nil {
		return err
	}
	if j.PriorRunning {
		if err = e.phase(ctx, j, "stopping"); err != nil {
			return err
		}
		if err = e.supervisor.Stop(ctx, ship.ID); err != nil {
			return err
		}
	}
	if before != nil {
		if err = before(ctx, ship); err != nil {
			return err
		}
	}
	ship, err = e.store.GetShip(ctx, ship.ID)
	if err != nil {
		return err
	}
	if ship.RecoveryBlocked {
		return errors.New(ship.RecoveryBlockReason)
	}
	if err = e.supervisor.ValidatePier(ship, false); err != nil {
		return err
	}
	if isMaintenance(j.Kind) {
		if err = e.checkResources(ctx, *j, ship); err != nil {
			return err
		}
	}
	locked, err := pier.WriterLocked(ship.PierPath)
	if err != nil || locked {
		return errors.New("pier still has a writer; utility was not started")
	}
	release, err := pier.Guard(ship.PierPath)
	if err != nil {
		return errors.New("pier is managed by another Harbor process")
	}
	defer func() { release() }()
	ship.State = "maintaining"
	if err = e.store.PutShip(ctx, ship); err != nil {
		return err
	}
	if err = e.phase(ctx, j, "executing"); err != nil {
		return err
	}
	digest, err := vr.FileDigest(ship.RuntimePath)
	if err != nil {
		return err
	}
	p := model.ProcessRecord{LaunchID: model.NewID(), JobID: j.ID, ShipID: ship.ID, Kind: "utility", Operation: j.Kind, Phase: "launching", BinaryDigest: digest}
	if err = e.store.PutProcess(ctx, p); err != nil {
		return err
	}
	// Utilities are not tied to browser cancellation or killed on a context deadline.
	cmd := exec.Command(ship.RuntimePath, args...)
	log := e.supervisor.JobLogWriter(ship.ID, j.ID)
	cmd.Stdout = log
	cmd.Stderr = log
	if err = cmd.Start(); err != nil {
		p.Phase = "exited"
		exit := -1
		p.ExitCode = &exit
		e.store.PutProcess(context.WithoutCancel(ctx), p)
		return errors.New("utility could not start")
	}
	p.PID = cmd.Process.Pid
	p.StartIdentity, err = supervisor.StartIdentity(p.PID)
	p.Phase = "live"
	persistErr := e.store.PutProcess(context.WithoutCancel(ctx), p)
	testAfterUtilitySpawn()
	waitErr := cmd.Wait()
	exit := cmd.ProcessState.ExitCode()
	p.ExitCode = &exit
	p.Phase = "exited"
	if saveErr := e.store.PutProcess(context.WithoutCancel(ctx), p); saveErr != nil {
		persistErr = saveErr
	}
	testAfterUtilityExit()
	j.ExitCode = &exit
	if err != nil || persistErr != nil || waitErr != nil {
		ship.RecoveryBlocked = true
		ship.RecoveryBlockReason = "Utility outcome requires recovery verification. Harbor has not restarted the ship."
		ship.State = "failed"
		e.store.PutShip(context.WithoutCancel(ctx), ship)
		return errors.New(ship.RecoveryBlockReason)
	}
	ship.State = "stopped"
	if after != nil {
		if err = after(ctx, &ship); err != nil {
			return err
		}
	}
	if err = e.store.PutShip(ctx, ship); err != nil {
		return err
	}
	release()
	release = func() {} // drop guard before the restart obtains it
	e.mu.Lock()
	ready := e.ready
	e.mu.Unlock()
	if j.PriorRunning && ready {
		if err = e.phase(ctx, j, "restarting"); err != nil {
			return err
		}
		if err = e.supervisor.Start(ctx, j.ID, ship.ID); err != nil {
			// A successful utility exit does not establish a usable pier when
			// its restart fails. Preserve checkpoints and prevent startup retry.
			current, readErr := e.store.GetShip(context.WithoutCancel(ctx), ship.ID)
			if readErr != nil {
				return errors.Join(err, readErr)
			}
			current.RecoveryBlocked = true
			if current.RecoveryBlockReason == "" {
				current.RecoveryBlockReason = "Utility completed but its restart failed. Pier and checkpoints remain held for recovery verification."
			}
			return errors.Join(err, e.store.PutShip(context.WithoutCancel(ctx), current))
		}
	}
	return nil
}
func (e *Executor) Reconcile(ctx context.Context) error {
	jobs, err := e.store.ListUnfinishedJobs(ctx)
	if err != nil {
		return err
	}
	for _, j := range jobs {
		j.Phase = "interrupted"
		j.Error = "Harbor stopped before completion was recorded. Pier held for recovery verification."
		j.FinishedAt = time.Now().UTC()
		if err = e.store.InterruptAndHold(ctx, j); err != nil {
			return err
		}
	}
	return nil
}
func (e *Executor) Run(ctx context.Context) error {
	tick := time.NewTicker(300 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
			if err := e.RunOne(ctx); err != nil {
				return err
			}
		}
	}
}
