package jobs

import (
	"context"
	"harbor/internal/model"
	"harbor/internal/scheduler"
	"harbor/internal/store"
	"harbor/internal/supervisor"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUncertainOperationIsInterruptedNotRetried(t *testing.T) {
	ctx := context.Background()
	st, e := store.Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer st.Close()
	ship := model.Ship{ID: "a", PierPath: t.TempDir(), DesiredRunning: true}
	st.PutShip(ctx, ship)
	j := model.Job{ID: "utility-a", ShipID: "a", Kind: "pack", Trigger: "manual", Phase: "queued"}
	st.CreateJob(ctx, j)
	j.Phase = "executing"
	st.UpdateJob(ctx, j)
	executor := New(st, supervisor.New(st, nil))
	if e = executor.Reconcile(ctx); e != nil {
		t.Fatal(e)
	}
	got, _ := st.GetJob(ctx, j.ID)
	if got.Phase != "interrupted" {
		t.Fatal(got.Phase)
	}
	ship, _ = st.GetShip(ctx, ship.ID)
	if !ship.RecoveryBlocked {
		t.Fatal("uncertain utility not held")
	}
	executor.SetReady(true)
	if _, e = executor.Submit(ctx, model.JobRequest{ShipID: ship.ID, Kind: "start", Trigger: "manual"}); e == nil {
		t.Fatal("start bypassed hold")
	}
}
func TestUnsupportedJobsRejectedBeforeEnqueue(t *testing.T) {
	ctx := context.Background()
	st, e := store.Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer st.Close()
	st.PutShip(ctx, model.Ship{ID: "a", PierPath: t.TempDir()})
	ex := New(st, supervisor.New(st, nil))
	ex.SetReady(true)
	for _, v := range []model.JobRequest{{ShipID: "a", Kind: "pack", Trigger: "scheduled"}, {ShipID: "a", Kind: "unknown", Trigger: "manual"}} {
		if _, e = ex.Submit(ctx, v); e == nil {
			t.Fatal("invalid job enqueued")
		}
	}
}

func TestConfirmedCompletionReleasesHoldWithoutStarting(t *testing.T) {
	ctx := context.Background()
	st, e := store.Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer st.Close()
	ship := model.Ship{ID: "held", PierPath: t.TempDir(), State: "failed", RecoveryBlocked: true, RecoveryBlockReason: "interrupted pack", DesiredRunning: true}
	st.PutShip(ctx, ship)
	j := model.Job{ID: "pack", ShipID: ship.ID, Kind: "pack", Trigger: "manual", Phase: "queued"}
	st.CreateJob(ctx, j)
	j.Phase = "interrupted"
	st.UpdateJob(ctx, j)
	exit := 0
	st.PutProcess(ctx, model.ProcessRecord{LaunchID: "p", JobID: j.ID, ShipID: ship.ID, Kind: "utility", Operation: "pack", Phase: "exited", PID: 1234, StartIdentity: "recorded child", ExitCode: &exit})
	ex := New(st, supervisor.New(st, nil))
	ex.verifyOffline = func(context.Context, model.Ship, model.ID) error { return nil }
	if e = ex.VerifyAndReleaseHold(ctx, ship.ID); e != nil {
		t.Fatal(e)
	}
	got, _ := st.GetShip(ctx, ship.ID)
	if got.RecoveryBlocked || got.State != "stopped" || got.DesiredRunning {
		t.Fatalf("not verified: %+v", got)
	}
	if pending, _ := st.HasPendingJob(ctx, ship.ID); pending {
		t.Fatal("verification started or enqueued a ship")
	}
}
func TestUnknownCompletionKeepsHold(t *testing.T) {
	ctx := context.Background()
	st, e := store.Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer st.Close()
	ship := model.Ship{ID: "held", PierPath: t.TempDir(), State: "failed", RecoveryBlocked: true}
	st.PutShip(ctx, ship)
	j := model.Job{ID: "pack", ShipID: ship.ID, Kind: "pack", Trigger: "manual", Phase: "queued"}
	st.CreateJob(ctx, j)
	j.Phase = "interrupted"
	st.UpdateJob(ctx, j)
	st.PutProcess(ctx, model.ProcessRecord{LaunchID: "p", JobID: j.ID, ShipID: ship.ID, Kind: "utility", Operation: "pack", Phase: "live", PID: 1234})
	ex := New(st, supervisor.New(st, nil))
	ex.verifyOffline = func(context.Context, model.Ship, model.ID) error {
		t.Fatal("inspection should not run with unknown utility outcome")
		return nil
	}
	if e = ex.VerifyAndReleaseHold(ctx, ship.ID); e == nil {
		t.Fatal("unknown completion released")
	}
	got, _ := st.GetShip(ctx, ship.ID)
	if !got.RecoveryBlocked {
		t.Fatal("hold cleared")
	}
}

func TestUnsupportedResourcesSkipBeforeStop(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".urb/chk"), 0700)
	os.MkdirAll(filepath.Join(dir, ".urb/log"), 0700)
	os.WriteFile(filepath.Join(dir, ".urb/chk/image.bin"), []byte("snapshot"), 0600)
	ship := model.Ship{ID: "running", PierPath: dir, State: "running", DesiredRunning: true, LoomExponent: 31, RuntimeVersion: "Vere 4.6", RuntimePath: "/missing/runtime"}
	st.PutShip(ctx, ship)
	sc := model.Schedule{ID: "pack-schedule", ShipID: ship.ID, Kind: "pack", IntervalDays: 7, AnchorDate: "2026-10-08", LocalTime: "03:00", Timezone: "UTC", Enabled: true, NextDue: time.Now().Add(-time.Minute)}
	st.PutSchedule(ctx, sc)
	st.ClaimDueAndEnqueue(ctx, time.Now(), scheduler.NextOccurrence)
	ex := New(st, supervisor.New(st, nil))
	ex.resources = func(context.Context, string) (scheduler.Resources, error) {
		return scheduler.Resources{AvailableMemory: 1, FreeDisk: 20 << 30, SnapshotBytes: 500 << 20}, nil
	}
	ex.SetReady(true)
	if err = ex.RunOne(ctx); err != nil {
		t.Fatal(err)
	}
	got, _ := st.GetShip(ctx, ship.ID)
	jobs, _ := st.JobsForShip(ctx, ship.ID)
	if got.State != "running" || !got.DesiredRunning || len(jobs) != 1 || jobs[0].Phase != "skipped" || !strings.Contains(jobs[0].Error, "RAM") {
		t.Fatal("resource skip altered running ship", got, jobs)
	}
	processes, _ := st.ListOpenProcesses(ctx)
	if len(processes) != 0 {
		t.Fatal("spawned before resource check")
	}
}
