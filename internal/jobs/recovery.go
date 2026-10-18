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
	var records []model.ProcessRecord
	latestIndex := -1
	for i := range jobs {
		if jobs[i].Phase == "interrupted" || jobs[i].Phase == "failed" {
			records, err = e.store.ProcessesForJob(ctx, jobs[i].ID)
			if err != nil {
				return err
			}
			// Jobs rejected by this existing hold never launched a utility.
			// They must not hide the operation whose outcome needs verification.
			if isMaintenance(jobs[i].Kind) && jobs[i].Phase == "failed" && jobs[i].ExitCode == nil && len(records) == 0 && ship.RecoveryBlockReason != "" && jobs[i].Error == ship.RecoveryBlockReason {
				continue
			}
			latest = &jobs[i]
			latestIndex = i
			break
		}
	}
	if latest == nil {
		return errors.New("no recorded operation can establish this pier's outcome")
	}
	confirmed := false
	switch latest.Kind {
	case "start":
		// A host restart may interrupt readiness after a registered ship was
		// launched. Require a conclusively closed launch below, then inspect
		// the current storage without booting or replacing it from an archive.
		if latest.Phase != "interrupted" || !ship.DesiredRunning {
			return errors.New("the interrupted Start did not retain running intent; hold remains")
		}
		if onlyVerificationRecords(records) {
			// A readiness retry reuses an owned process. Follow only the
			// consecutive failed readiness checks to its original launch.
			for _, previous := range jobs[latestIndex+1:] {
				if previous.Kind != "start" || previous.Phase != "failed" || previous.Error != "ship readiness timed out; process remains tracked" {
					break
				}
				prior, lookup := e.store.ProcessesForJob(ctx, previous.ID)
				if lookup != nil {
					return lookup
				}
				if onlyVerificationRecords(prior) {
					continue
				}
				launch, valid := singleClosedStartLaunch(prior)
				if !valid {
					return errors.New("the earlier readiness launch was not conclusively closed; hold remains")
				}
				records = []model.ProcessRecord{launch}
				break
			}
		}
	case "stop":
		// Stop does not run an offline utility. PriorRunning is recorded only
		// after admission checks; saved stopped intent proves Stop progressed
		// past that point. The original ship process is checked above, and the
		// current pier must still pass offline inspection before release.
		if latest.Phase != "interrupted" || !latest.PriorRunning || ship.DesiredRunning {
			return errors.New("the interrupted Stop did not record a running ship and stopped intent; hold remains")
		}
		confirmed = true
	case "pack", "meld", "roll", "chop":
	default:
		return errors.New("this interrupted operation requires recovery from a pier archive without rolling back live network history")
	}
	for _, p := range records {
		if p.Operation == "verify" {
			continue
		}
		if latest.Kind == "start" {
			if p.Kind != "ship" || p.Operation != "start" || p.Phase != "exited" || p.PID <= 1 || p.StartIdentity == "" || p.BinaryDigest == "" || confirmed {
				return errors.New("a single closed ship launch was not conclusively recorded; hold remains")
			}
			confirmed = true
			continue
		}
		if latest.Kind == "stop" {
			return errors.New("an unexpected process was recorded for Stop; hold remains")
		}
		// Maintenance can launch a ship after the utility exits. A closed
		// restart is not another utility outcome; offline verification follows.
		if p.Kind == "ship" && p.Operation == "start" && p.Phase == "exited" {
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
		return errors.New("missing process outcome; hold remains")
	}
	release, err := pier.Guard(ship.PierPath)
	if err != nil {
		return errors.New("pier is in use")
	}
	defer release()
	unblock, err := pier.ExcludeWriter(ship.PierPath)
	if err != nil {
		return err
	}
	defer unblock()
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

func onlyVerificationRecords(records []model.ProcessRecord) bool {
	for _, process := range records {
		if process.Operation != "verify" {
			return false
		}
	}
	return true
}

func singleClosedStartLaunch(records []model.ProcessRecord) (model.ProcessRecord, bool) {
	var launch model.ProcessRecord
	found := false
	for _, process := range records {
		if process.Operation == "verify" {
			continue
		}
		if found || process.Kind != "ship" || process.Operation != "start" || process.Phase != "exited" || process.PID <= 1 || process.StartIdentity == "" || process.BinaryDigest == "" {
			return model.ProcessRecord{}, false
		}
		launch, found = process, true
	}
	return launch, found
}

func (e *Executor) inspectOffline(ctx context.Context, ship model.Ship, job model.ID) error {
	// VerifyAndReleaseHold keeps both source locks through the final store write.
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
	mapArgs, err := vr.LMDBMapArgs(ctx, ship.PierPath, false)
	if err != nil {
		return err
	}
	digest, err := vr.FileDigest(ship.RuntimePath)
	if err != nil {
		return err
	}
	inspection, err := e.copyForInspection(ctx, ship.PierPath)
	if err != nil {
		return errors.New("could not prepare a private inspection copy; check free disk space and storage access; hold remains")
	}
	defer inspection.close()
	p := model.ProcessRecord{LaunchID: model.NewID(), JobID: job, ShipID: ship.ID, Kind: "utility", Operation: "verify", Phase: "launching", BinaryDigest: digest}
	if err = e.store.PutProcess(ctx, p); err != nil {
		return err
	}
	// Info may migrate or fall back during load. Run it only on the private copy
	// and reject changed storage, even on exit zero. It never boots a ship server.
	args := append([]string{"info"}, mapArgs...)
	args = append(args, "--loom", strconv.Itoa(ship.LoomExponent), inspection.path)
	cmd := exec.Command(ship.RuntimePath, args...)
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
	if err = inspection.unchanged(ctx); err != nil {
		return err
	}
	return inspection.close()
}
