package jobs

import (
	"context"
	"errors"
	"harbor/internal/model"
	"harbor/internal/store"
	"harbor/internal/supervisor"
	"path/filepath"
	"testing"
)

func TestRecoveryUsesRecordedUtilityAfterBlockedJobsAndShipRestart(t *testing.T) {
	for _, trial := range []struct {
		name                                         string
		restart, blocked, uncertain, inspectionFails bool
	}{
		{name: "closed ship restart", restart: true},
		{name: "later jobs rejected by hold", blocked: true},
		{name: "closed restart and later blocked jobs", restart: true, blocked: true},
		{name: "offline inspection failure keeps hold", restart: true, blocked: true, inspectionFails: true},
		{name: "unknown newer utility keeps hold", uncertain: true},
	} {
		t.Run(trial.name, func(t *testing.T) {
			ctx := context.Background()
			st, err := store.Open(filepath.Join(t.TempDir(), "db"))
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()
			require := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			reason := "Harbor stopped before completion was recorded. Pier held for recovery verification."
			ship := model.Ship{ID: "held", PierPath: t.TempDir(), State: "failed", RecoveryBlocked: true, RecoveryBlockReason: reason, DesiredRunning: true}
			require(st.PutShip(ctx, ship))
			pack := model.Job{ID: "pack", ShipID: ship.ID, Kind: "pack", Trigger: "manual", Phase: "queued"}
			require(st.CreateJob(ctx, pack))
			pack.Phase, pack.Error = "interrupted", reason
			require(st.UpdateJob(ctx, pack))
			exit := 0
			require(st.PutProcess(ctx, model.ProcessRecord{LaunchID: "utility", JobID: pack.ID, ShipID: ship.ID, Kind: "utility", Operation: "pack", Phase: "exited", PID: 1234, StartIdentity: "recorded utility", ExitCode: &exit}))
			if trial.restart {
				require(st.PutProcess(ctx, model.ProcessRecord{LaunchID: "restart", JobID: pack.ID, ShipID: ship.ID, Kind: "ship", Operation: "start", Phase: "exited", PID: 1235, StartIdentity: "recorded restart"}))
			}
			if trial.blocked {
				for _, kind := range []model.Operation{"roll", "chop", "meld"} {
					job := model.Job{ID: model.ID(kind), ShipID: ship.ID, Kind: kind, Trigger: "manual", Phase: "queued"}
					require(st.CreateJob(ctx, job))
					job.Phase, job.Error = "failed", reason
					require(st.UpdateJob(ctx, job))
				}
			}
			if trial.uncertain {
				job := model.Job{ID: "uncertain", ShipID: ship.ID, Kind: "meld", Trigger: "manual", Phase: "queued"}
				require(st.CreateJob(ctx, job))
				job.Phase, job.Error = "interrupted", reason
				require(st.UpdateJob(ctx, job))
			}
			ex := New(st, supervisor.New(st, nil))
			inspections := 0
			ex.verifyOffline = func(_ context.Context, _ model.Ship, job model.ID) error {
				inspections++
				if job != pack.ID {
					t.Fatalf("inspected %s instead of Pack", job)
				}
				if trial.inspectionFails {
					return errors.New("offline verification failed")
				}
				return nil
			}
			err = ex.VerifyAndReleaseHold(ctx, ship.ID)
			got, lookup := st.GetShip(ctx, ship.ID)
			require(lookup)
			if trial.uncertain || trial.inspectionFails {
				if err == nil || !got.RecoveryBlocked {
					t.Fatal("uncertain or invalid pier was released")
				}
				if trial.uncertain && inspections != 0 {
					t.Fatal("uncertain utility reached offline inspection")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if inspections != 1 || got.RecoveryBlocked || got.State != "stopped" || got.DesiredRunning {
					t.Fatalf("incorrect verified state: %+v; inspections=%d", got, inspections)
				}
			}
		})
	}
}
