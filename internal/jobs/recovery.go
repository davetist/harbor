package jobs

import (
	"context"
	"errors"
	"harbor/internal/model"
	"harbor/internal/pier"
	vr "harbor/internal/runtime"
	"harbor/internal/supervisor"
	"io"
	"os/exec"
	"strconv"
)

func (e *Executor) VerifyAndReleaseHold(ctx context.Context, id model.ID) error {
	e.runMu.Lock()
	defer e.runMu.Unlock()
	ship, err := e.store.GetShip(ctx, id)
	if err != nil {
		return err
	}
	if !ship.RecoveryBlocked {
		return nil
	}
	if err = e.supervisor.ValidatePier(ship, false); err != nil {
		return err
	}
	locked, err := pier.WriterLocked(ship.PierPath)
	if err != nil || locked {
		return errors.New("a writer is still present; recovery hold remains")
	}
	open, err := e.store.ListOpenProcesses(ctx)
	if err != nil {
		return err
	}
	for _, p := range open {
		if p.ShipID == id {
			return errors.New("a process launch or outcome is still uncertain; recover from a verified pier archive in a new folder without rolling back live network history")
		}
	}
	jobs, err := e.store.JobsForShip(ctx, id)
	if err != nil {
		return err
	}
	var latest *model.Job
	for i := range jobs {
		if jobs[i].Phase == "interrupted" || jobs[i].Phase == "failed" {
			latest = &jobs[i]
			break
		}
	}
	if latest == nil {
		return errors.New("no recorded operation can establish this pier's outcome")
	}
	switch latest.Kind {
	case "pack", "meld", "roll", "chop":
	default:
		return errors.New("this interrupted operation requires recovery from a pier archive without rolling back live network history")
	}
	records, err := e.store.ProcessesForJob(ctx, latest.ID)
	if err != nil {
		return err
	}
	confirmed := false
	for _, p := range records {
		if p.Operation == "verify" {
			continue
		}
		if p.Kind != "utility" || p.Operation != latest.Kind || p.Phase != "exited" || p.ExitCode == nil || *p.ExitCode != 0 || p.PID <= 1 || p.StartIdentity == "" {
			return errors.New("utility success was not conclusively recorded; hold remains")
		}
		if confirmed {
			return errors.New("multiple utility launches require recovery from a pier archive")
		}
		confirmed = true
	}
	if !confirmed {
		return errors.New("missing utility outcome; hold remains")
	}
	if err = e.verifyOffline(context.WithoutCancel(ctx), ship, latest.ID); err != nil {
		return err
	}
	ship.RecoveryBlocked = false
	ship.RecoveryBlockReason = ""
	ship.Error = ""
	ship.State = "stopped"
	ship.DesiredRunning = false
	if err = e.store.PutShip(context.WithoutCancel(ctx), ship); err != nil {
		return err
	}
	e.emit(model.Event{Type: "ship", ShipID: ship.ID, Data: ship})
	return nil
}
func (e *Executor) inspectOffline(ctx context.Context, ship model.Ship, job model.ID) error {
	if err := e.supervisor.ValidatePier(ship, false); err != nil {
		return err
	}
	if _, err := pier.Inspect(ship.PierPath); err != nil {
		return err
	}
	platform, err := vr.NativePlatform()
	if err != nil {
		return err
	}
	if err = vr.ValidateBinary(ship.RuntimePath, platform); err != nil {
		return err
	}
	release, err := pier.Guard(ship.PierPath)
	if err != nil {
		return errors.New("pier is in use")
	}
	defer release()
	if locked, err := pier.WriterLocked(ship.PierPath); err != nil || locked {
		return errors.New("pier still has a writer")
	}
	digest, err := vr.FileDigest(ship.RuntimePath)
	if err != nil {
		return err
	}
	p := model.ProcessRecord{LaunchID: model.NewID(), JobID: job, ShipID: ship.ID, Kind: "utility", Operation: "verify", Phase: "launching", BinaryDigest: digest}
	if err = e.store.PutProcess(ctx, p); err != nil {
		return err
	}
	// Vere info loads the offline snapshot/event store and exits. It never boots
	// an HTTP server or sends an Arvo event; all diagnostic output is discarded.
	cmd := exec.Command(ship.RuntimePath, "info", "--loom", strconv.Itoa(ship.LoomExponent), ship.PierPath)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err = cmd.Start(); err != nil {
		exit := -1
		p.Phase = "exited"
		p.ExitCode = &exit
		e.store.PutProcess(ctx, p)
		return errors.New("offline inspection could not start")
	}
	p.PID = cmd.Process.Pid
	p.StartIdentity, err = supervisor.StartIdentity(p.PID)
	p.Phase = "live"
	persist := e.store.PutProcess(ctx, p)
	wait := cmd.Wait()
	exit := cmd.ProcessState.ExitCode()
	p.Phase = "exited"
	p.ExitCode = &exit
	save := e.store.PutProcess(ctx, p)
	if err != nil || persist != nil || wait != nil || save != nil {
		return errors.New("offline snapshot/event verification did not succeed; hold remains")
	}
	return nil
}
