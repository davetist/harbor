package jobs

import (
	"context"
	"errors"
	"harbor/internal/model"
	"harbor/internal/pier"
	"harbor/internal/store"
	"harbor/internal/supervisor"
	"path/filepath"
	"testing"
)

func TestInterruptedStartRecoveryRequiresClosedLaunchAndUnchangedInspection(t *testing.T) {
	for _, trial := range []struct {
		name            string
		phase           string
		processPhase    string
		noProcess       bool
		noIdentity      bool
		noDigest        bool
		stoppedIntent   bool
		duplicateLaunch bool
		unexpectedChild bool
		inspectionFails bool
		wantInspection  bool
		wantRelease     bool
	}{
		{name: "closed startup after reboot", wantInspection: true, wantRelease: true},
		{name: "inspection failure retains hold", inspectionFails: true, wantInspection: true},
		{name: "live process retains hold", processPhase: "live"},
		{name: "uncertain launch retains hold", processPhase: "launching"},
		{name: "missing process evidence", noProcess: true},
		{name: "missing process identity", noIdentity: true},
		{name: "missing binary identity", noDigest: true},
		{name: "stopped intent remains held", stoppedIntent: true},
		{name: "failed start is not interrupted", phase: "failed"},
		{name: "multiple launches remain held", duplicateLaunch: true},
		{name: "unexpected utility remains held", unexpectedChild: true},
	} {
		t.Run(trial.name, func(t *testing.T) {
			ctx := context.Background()
			st, err := store.Open(filepath.Join(t.TempDir(), "harbor.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { st.Close() })
			require := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			ship := model.Ship{ID: "held", PierPath: t.TempDir(), State: "failed",
				RecoveryBlocked: true, RecoveryBlockReason: "Interrupted startup.",
				DesiredRunning: !trial.stoppedIntent}
			require(st.PutShip(ctx, ship))
			ship, err = st.GetShip(ctx, ship.ID)
			require(err)
			job := model.Job{ID: "start", ShipID: ship.ID, Kind: "start", Trigger: "manual", Phase: "queued"}
			require(st.CreateJob(ctx, job))
			job.Phase = "interrupted"
			if trial.phase != "" {
				job.Phase = trial.phase
			}
			require(st.UpdateJob(ctx, job))
			process := model.ProcessRecord{LaunchID: "ship", JobID: job.ID, ShipID: ship.ID,
				Kind: "ship", Operation: "start", Phase: "exited", PID: 1234,
				StartIdentity: "recorded process identity", BinaryDigest: "recorded binary digest"}
			if trial.processPhase != "" {
				process.Phase = trial.processPhase
			}
			if trial.noIdentity {
				process.StartIdentity = ""
			}
			if trial.noDigest {
				process.BinaryDigest = ""
			}
			if !trial.noProcess {
				require(st.PutProcess(ctx, process))
			}
			if trial.duplicateLaunch || trial.unexpectedChild {
				process.LaunchID = "another"
				if trial.unexpectedChild {
					process.Kind, process.Operation = "utility", "pack"
				}
				require(st.PutProcess(ctx, process))
			}
			ex := New(st, supervisor.New(st, nil))
			inspections := 0
			ex.verifyOffline = func(_ context.Context, current model.Ship, currentJob model.ID) error {
				inspections++
				if locked, err := pier.WriterLocked(current.PierPath); err != nil || !locked {
					t.Fatal("original pier is not protected during inspection", err)
				}
				if current.PierPath != ship.PierPath || currentJob != job.ID {
					t.Fatal("inspection did not use the current pier and interrupted Start")
				}
				if trial.inspectionFails {
					return errors.New("offline inspection rejected the copy")
				}
				return nil
			}
			err = ex.VerifyAndReleaseHold(ctx, ship.ID)
			got, lookup := st.GetShip(ctx, ship.ID)
			require(lookup)
			if (inspections == 1) != trial.wantInspection || inspections > 1 {
				t.Fatalf("unexpected inspection count: %d", inspections)
			}
			if trial.wantRelease {
				if err != nil || got.RecoveryBlocked || got.State != "stopped" || got.DesiredRunning {
					t.Fatalf("verified Start did not release to stopped: held=%v state=%s err=%v", got.RecoveryBlocked, got.State, err)
				}
			} else if err == nil || got != ship {
				t.Fatalf("failed recovery changed the held ship: err=%v state=%+v", err, got)
			}
			pending, err := st.HasPendingJob(ctx, ship.ID)
			require(err)
			if pending {
				t.Fatal("verification must not queue a networked start")
			}
		})
	}
}

func TestInterruptedReadinessRetryUsesItsEarlierClosedLaunch(t *testing.T) {
	for _, trial := range []struct {
		name            string
		previousPhase   string
		previousError   string
		interveningKind model.Operation
		repeatedRetry   bool
		noLaunch        bool
		uncertainLaunch bool
		noIdentity      bool
		noDigest        bool
		invalidPID      bool
		wrongKind       bool
		wrongOperation  bool
		duplicateLaunch bool
		inspectionFails bool
		wantInspection  bool
		wantRelease     bool
	}{
		{name: "interrupted readiness retry", wantInspection: true, wantRelease: true},
		{name: "repeated readiness retries", repeatedRetry: true, wantInspection: true, wantRelease: true},
		{name: "inspection failure retains hold", inspectionFails: true, wantInspection: true},
		{name: "missing earlier launch", noLaunch: true},
		{name: "uncertain earlier launch", uncertainLaunch: true},
		{name: "earlier launch without identity", noIdentity: true},
		{name: "earlier launch without digest", noDigest: true},
		{name: "earlier launch without PID", invalidPID: true},
		{name: "earlier utility is not a ship launch", wrongKind: true},
		{name: "earlier non-start operation remains held", wrongOperation: true},
		{name: "multiple earlier launches remain held", duplicateLaunch: true},
		{name: "different failure is not a readiness retry", previousError: "runtime exited during readiness"},
		{name: "completed start does not prove a readiness retry", previousPhase: "completed"},
		{name: "intervening maintenance remains held", interveningKind: "pack"},
		{name: "intervening stop remains held", interveningKind: "stop"},
	} {
		t.Run(trial.name, func(t *testing.T) {
			ctx := context.Background()
			st, err := store.Open(filepath.Join(t.TempDir(), "harbor.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { st.Close() })
			require := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			ship := model.Ship{ID: "retry", PierPath: t.TempDir(), State: "failed", DesiredRunning: true,
				RecoveryBlocked: true, RecoveryBlockReason: "Interrupted startup."}
			require(st.PutShip(ctx, ship))
			ship, err = st.GetShip(ctx, ship.ID)
			require(err)
			addJob := func(id model.ID, kind model.Operation, phase, message string) {
				job := model.Job{ID: id, ShipID: ship.ID, Kind: kind, Trigger: "system", Phase: "queued"}
				require(st.CreateJob(ctx, job))
				job.Phase, job.Error = phase, message
				require(st.UpdateJob(ctx, job))
			}
			phase, message := "failed", "ship readiness timed out; process remains tracked"
			if trial.previousPhase != "" {
				phase = trial.previousPhase
			}
			if trial.previousError != "" {
				message = trial.previousError
			}
			addJob("original", "start", phase, message)
			if !trial.noLaunch {
				processPhase := "exited"
				if trial.uncertainLaunch {
					processPhase = "launching"
				}
				process := model.ProcessRecord{LaunchID: "launch", JobID: "original", ShipID: ship.ID,
					Kind: "ship", Operation: "start", Phase: processPhase, PID: 1234,
					StartIdentity: "recorded process identity", BinaryDigest: "recorded binary digest"}
				if trial.noIdentity {
					process.StartIdentity = ""
				}
				if trial.noDigest {
					process.BinaryDigest = ""
				}
				if trial.invalidPID {
					process.PID = 0
				}
				if trial.wrongKind {
					process.Kind = "utility"
				}
				if trial.wrongOperation {
					process.Operation = "pack"
				}
				require(st.PutProcess(ctx, process))
				if trial.duplicateLaunch {
					process.LaunchID = "second"
					require(st.PutProcess(ctx, process))
				}
			}
			if trial.repeatedRetry {
				addJob("earlier-retry", "start", "failed", message)
			}
			if trial.interveningKind != "" {
				addJob("intervening", trial.interveningKind, "completed", "")
			}
			addJob("interrupted-retry", "start", "interrupted", ship.RecoveryBlockReason)
			ex := New(st, supervisor.New(st, nil))
			inspections := 0
			ex.verifyOffline = func(_ context.Context, current model.Ship, job model.ID) error {
				inspections++
				if job != "interrupted-retry" {
					t.Fatal("inspection must belong to the interrupted retry")
				}
				if locked, err := pier.WriterLocked(current.PierPath); err != nil || !locked {
					t.Fatal("original is not protected", err)
				}
				if trial.inspectionFails {
					return errors.New("inspection failed")
				}
				return nil
			}
			err = ex.VerifyAndReleaseHold(ctx, ship.ID)
			got, lookup := st.GetShip(ctx, ship.ID)
			require(lookup)
			if (inspections == 1) != trial.wantInspection || inspections > 1 {
				t.Fatalf("unexpected inspection count: %d", inspections)
			}
			if trial.wantRelease {
				if err != nil || got.RecoveryBlocked || got.State != "stopped" || got.DesiredRunning {
					t.Fatalf("retry was not recovered: %v, %+v", err, got)
				}
			} else if err == nil || got != ship {
				t.Fatalf("unverified retry changed the hold: %v", err)
			}
			pending, err := st.HasPendingJob(ctx, ship.ID)
			require(err)
			if pending {
				t.Fatal("verification queued a start")
			}
		})
	}
}
