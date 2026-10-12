package jobs

import (
	"context"
	"harbor/internal/model"
	vr "harbor/internal/runtime"
	"harbor/internal/scheduler"
	"harbor/internal/store"
	"os"
	"strings"
	"testing"
	"time"
)

func queueScheduledMaintenance(t *testing.T, st *store.Store, ship model.ID) []model.Job {
	t.Helper()
	ctx := context.Background()
	at := time.Now().UTC().AddDate(0, 0, -1)
	runDay := time.Date(at.Year(), at.Month(), at.Day(), 4, 0, 0, 0, time.UTC)
	for _, row := range []struct {
		kind  model.Operation
		clock string
	}{{"roll", "03:05"}, {"chop", "03:10"}, {"meld", "03:15"}, {"pack", "03:00"}} {
		sc := model.Schedule{ID: model.ID(row.kind), ShipID: ship, Kind: row.kind, AnchorDate: runDay.AddDate(0, 0, -7).Format("2006-01-02"), IntervalDays: 7, LocalTime: row.clock, Timezone: "UTC", Enabled: true}
		var err error
		sc.NextDue, err = scheduler.NextOccurrence(sc, runDay.AddDate(0, 0, -1))
		if err != nil {
			t.Fatal(err)
		}
		if sc.Kind == "chop" {
			sc.AfterScheduleID = "roll"
		}
		if err = st.PutSchedule(ctx, sc); err != nil {
			t.Fatal(err)
		}
	}
	jobs, err := st.ClaimDueAndEnqueue(ctx, runDay, scheduler.NextOccurrence)
	if err != nil || len(jobs) != 4 {
		t.Fatal(jobs, err)
	}
	return jobs
}

func TestScheduledSequencePreservesConditionalMeld(t *testing.T) {
	target, err := vr.NativePlatform()
	if err != nil {
		t.Fatal(err)
	}
	if target.Arch != "arm64" {
		t.Skip("Meld admission requires a measured ARM64 profile")
	}
	for _, snapshot := range []uint64{300 << 20, 800 << 20} {
		t.Run(map[uint64]string{300 << 20: "below threshold", 800 << 20: "Meld admitted"}[snapshot], func(t *testing.T) {
			ctx := context.Background()
			ex, st, ship, _ := utilityMappingExecutor(t)
			// Keep the fixture offline while exercising the real queue, executor,
			// resource admission and helper utility processes.
			ship.DesiredRunning = true
			if err := st.PutShip(ctx, ship); err != nil {
				t.Fatal(err)
			}
			ex.resources = func(context.Context, string) (scheduler.Resources, error) {
				hardware := "Apple M2 Max"
				if target.OS == "linux" {
					hardware = "Raspberry Pi 4 Model B Rev 1.4"
				}
				return scheduler.Resources{AvailableMemory: 8 << 30, TotalMemory: 32 << 30, FreeDisk: 20 << 30, SnapshotBytes: snapshot, Hardware: hardware}, nil
			}
			jobs := queueScheduledMaintenance(t, st, ship.ID)
			ex.SetReady(true)
			for i, kind := range []model.Operation{"pack", "meld", "roll", "chop"} {
				if err = ex.RunOne(ctx); err != nil {
					t.Fatal(err)
				}
				got, err := st.GetJob(ctx, jobs[i].ID)
				if err != nil || got.Kind != kind {
					t.Fatal(got, err)
				}
				if kind == "meld" && snapshot < 512<<20 {
					if got.Phase != "skipped" || !strings.Contains(got.Error, "pressure threshold") {
						t.Fatalf("conditional Meld did not skip: %+v", got)
					}
					continue
				}
				if got.Phase != "completed" || got.ExitCode == nil || *got.ExitCode != 0 {
					t.Fatalf("sequence failed at %s: %+v", kind, got)
				}
			}
			if pending, err := st.HasPendingJob(ctx, ship.ID); err != nil || pending {
				t.Fatal("sequence left pending jobs", err)
			}
			id, err := ex.Submit(ctx, model.JobRequest{ShipID: ship.ID, Kind: "chop", Trigger: "manual"})
			if err != nil {
				t.Fatal("manual Chop was unavailable", err)
			}
			if err = ex.RunOne(ctx); err != nil {
				t.Fatal(err)
			}
			manual, err := st.GetJob(ctx, id)
			if err != nil || manual.Phase != "completed" || manual.DependsOnJobID != "" {
				t.Fatal("manual Chop inherited automatic sequencing", manual, err)
			}
		})
	}
}

func TestScheduledUtilityFailureKeepsFollowingStagesHeld(t *testing.T) {
	ctx := context.Background()
	ex, st, ship, _ := utilityMappingExecutor(t)
	ship.DesiredRunning = true
	if err := st.PutShip(ctx, ship); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ship.RuntimePath, []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	jobs := queueScheduledMaintenance(t, st, ship.ID)
	ex.SetReady(true)
	for _, j := range jobs {
		if err := ex.RunOne(ctx); err != nil {
			t.Fatal(err)
		}
		got, err := st.GetJob(ctx, j.ID)
		if err != nil || got.Phase != "failed" {
			t.Fatal("failure was hidden", got, err)
		}
		processes, err := st.ProcessesForJob(ctx, j.ID)
		if err != nil {
			t.Fatal(err)
		}
		if j.Kind == "pack" {
			if len(processes) != 1 || processes[0].ExitCode == nil || *processes[0].ExitCode == 0 {
				t.Fatal("failed Pack outcome was not preserved", processes)
			}
		} else if len(processes) != 0 {
			t.Fatal("following utility launched through the recovery hold", j.Kind, processes)
		}
	}
	held, err := st.GetShip(ctx, ship.ID)
	if err != nil || !held.RecoveryBlocked || held.State != "failed" {
		t.Fatal("maintenance failure released the pier", held, err)
	}
}
