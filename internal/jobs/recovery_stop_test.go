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

func TestInterruptedStopRecoveryVerifiesCurrentPierAndKeepsItStopped(t *testing.T) {
	for _, trial := range []struct {
		name            string
		phase           string
		processPhase    string
		desiredRunning  bool
		wasNotRunning   bool
		unexpectedChild bool
		inspectionFails bool
		blockedJob      bool
		wantInspection  bool
		wantRelease     bool
	}{
		{name: "closed running ship", wantInspection: true, wantRelease: true},
		{name: "later blocked maintenance", blockedJob: true, wantInspection: true, wantRelease: true},
		{name: "offline failure retains hold", inspectionFails: true, wantInspection: true},
		{name: "live process retains hold", processPhase: "live"},
		{name: "uncertain launch retains hold", processPhase: "launching"},
		{name: "stop intent not persisted", desiredRunning: true},
		{name: "stop did not reach a running ship", wasNotRunning: true},
		{name: "failed stop is not an interrupted stop", phase: "failed"},
		{name: "unexpected utility retains hold", unexpectedChild: true},
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
			reason := "Harbor stopped before completion was recorded. Pier held for recovery verification."
			ship := model.Ship{ID: "held", PierPath: t.TempDir(), State: "failed", RecoveryBlocked: true,
				RecoveryBlockReason: reason, DesiredRunning: trial.desiredRunning}
			require(st.PutShip(ctx, ship))
			ship, err = st.GetShip(ctx, ship.ID)
			require(err)
			putJob := func(job model.Job) {
				phase := job.Phase
				job.Phase = "queued"
				require(st.CreateJob(ctx, job))
				job.Phase = phase
				require(st.UpdateJob(ctx, job))
			}
			boot := model.Job{ID: "boot", ShipID: ship.ID, Kind: "boot_keyfile", Trigger: "manual", Phase: "completed"}
			putJob(boot)
			phase := "exited"
			if trial.processPhase != "" {
				phase = trial.processPhase
			}
			// A stopped ship's process belongs to its original launch job. A
			// controller restart can confirm it is gone without knowing its exit code.
			require(st.PutProcess(ctx, model.ProcessRecord{LaunchID: "ship", JobID: boot.ID,
				ShipID: ship.ID, Kind: "ship", Operation: "start", Phase: phase,
				PID: 1234, StartIdentity: "recorded process identity"}))
			stop := model.Job{ID: "stop", ShipID: ship.ID, Kind: "stop", Trigger: "manual",
				Phase: "interrupted", PriorRunning: !trial.wasNotRunning, Error: reason}
			if trial.phase != "" {
				stop.Phase = trial.phase
			}
			putJob(stop)
			if trial.unexpectedChild {
				exit := 0
				require(st.PutProcess(ctx, model.ProcessRecord{LaunchID: "unexpected", JobID: stop.ID,
					ShipID: ship.ID, Kind: "utility", Operation: "pack", Phase: "exited",
					PID: 1235, StartIdentity: "unexpected utility", ExitCode: &exit}))
			}
			if trial.blockedJob {
				putJob(model.Job{ID: "blocked", ShipID: ship.ID, Kind: "pack",
					Trigger: "manual", Phase: "failed", Error: reason})
			}
			ex := New(st, supervisor.New(st, nil))
			inspections := 0
			ex.verifyOffline = func(_ context.Context, current model.Ship, job model.ID) error {
				inspections++
				if locked, err := pier.WriterLocked(current.PierPath); err != nil || !locked {
					t.Fatal("original pier was not protected against writers during inspection", err)
				}
				if current.PierPath != ship.PierPath || job != stop.ID {
					t.Fatal("verification did not use the current pier and interrupted Stop")
				}
				if trial.inspectionFails {
					return errors.New("offline inspection failed")
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
				if err != nil || got.RecoveryBlocked || got.State != "stopped" || got.DesiredRunning || got.Error != "" || got.RecoveryBlockReason != "" {
					t.Fatalf("interrupted Stop did not recover: state=%s held=%v desired=%v err=%v", got.State, got.RecoveryBlocked, got.DesiredRunning, err)
				}
			} else if err == nil || got != ship {
				t.Fatalf("failed recovery altered the held ship: err=%v state=%+v", err, got)
			}
			pending, err := st.HasPendingJob(ctx, ship.ID)
			require(err)
			if pending {
				t.Fatal("verification queued a ship start")
			}
		})
	}
}
