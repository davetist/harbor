package jobs

import (
	"context"
	"harbor/internal/model"
	"harbor/internal/pier"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStartReadinessTimeoutCanRecheckOwnedProcess(t *testing.T) {
	ctx := context.Background()
	ex, st, ship, mode := runningMaintenanceExecutor(t)
	if err := ex.supervisor.Stop(ctx, ship.ID); err != nil {
		t.Fatal(err)
	}
	*mode = "unready"
	ex.readinessTimeout = 500 * time.Millisecond
	run := func() model.Job {
		t.Helper()
		id, err := ex.Submit(ctx, model.JobRequest{ShipID: ship.ID, Kind: "start", Trigger: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		if err := ex.RunOne(ctx); err != nil {
			t.Fatal(err)
		}
		job, err := st.GetJob(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return job
	}
	first := run()
	current, err := st.GetShip(ctx, ship.ID)
	if err != nil || first.Phase != "failed" || first.FinishedAt.IsZero() || current.State != "failed" || current.Error == "" || !current.DesiredRunning || current.RecoveryBlocked {
		t.Fatalf("readiness timeout did not settle the failed start: job=%s state=%s desired=%v hold=%v err=%v", first.Phase, current.State, current.DesiredRunning, current.RecoveryBlocked, err)
	}
	if pending, err := st.HasPendingJob(ctx, ship.ID); err != nil || pending {
		t.Fatal("failed start retained an active job", err)
	}
	records, err := st.ProcessesForJob(ctx, first.ID)
	if err != nil || len(records) != 1 || records[0].Phase != "live" || records[0].StartIdentity == "" {
		t.Fatal("failed readiness lost the owned writer", records, err)
	}
	if release, err := pier.Guard(ship.PierPath); err == nil {
		release()
		t.Fatal("failed readiness released a live writer's guard")
	}
	port := current.WebPort
	// A second unsuccessful readiness check must retain the very same process.
	second := run()
	if second.Phase != "failed" {
		t.Fatal("unready process was accepted", second.Phase)
	}
	if err := os.WriteFile(filepath.Join(ship.PierPath, "finish-readiness"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	third := run()
	current, err = st.GetShip(ctx, ship.ID)
	if err != nil || third.Phase != "completed" || current.State != "running" || current.Error != "" || !current.DesiredRunning || current.RecoveryBlocked || current.WebPort != port {
		t.Fatal("ready process was not recovered by Start", third.Phase, current.State, err)
	}
	open, err := st.ListOpenProcesses(ctx)
	if err != nil || len(open) != 1 || open[0] != records[0] {
		t.Fatal("readiness retry replaced or reassigned the owned process", open, err)
	}
	for _, retry := range []model.Job{second, third} {
		if own, err := st.ProcessesForJob(ctx, retry.ID); err != nil || len(own) != 0 {
			t.Fatal("readiness retry recorded a new launch", own, err)
		}
	}
	if err := ex.supervisor.Start(ctx, "duplicate", ship.ID); err == nil {
		t.Fatal("running duplicate Start was accepted")
	}
}
