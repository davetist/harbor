package jobs

import (
	"context"
	"fmt"
	"harbor/internal/model"
	"harbor/internal/pier"
	vr "harbor/internal/runtime"
	"harbor/internal/scheduler"
	"harbor/internal/store"
	"harbor/internal/supervisor"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestMaintenanceRuntimeHelper(t *testing.T) {
	mode := os.Getenv("HARBOR_MAINTENANCE_RUNTIME_HELPER")
	if mode == "" {
		return
	}
	port := ""
	for i, arg := range os.Args {
		if arg == "--http-port" && i+1 < len(os.Args) {
			port = os.Args[i+1]
		}
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:"+port)
	if err != nil {
		os.Exit(2)
	}
	if err = os.WriteFile(filepath.Join(os.Args[len(os.Args)-1], ".http.ports"), []byte(port+" insecure loopback\n"), 0600); err != nil {
		os.Exit(2)
	}
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM)
	go http.Serve(listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			if _, err := os.Stat(filepath.Join(os.Args[len(os.Args)-1], "finish-readiness")); mode == "unready" && err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			fmt.Fprint(w, `"~zod"`)
			return
		}
		fmt.Fprint(w, "ready")
	}))
	<-ch
	listener.Close()
	os.Exit(0)
}

func runningMaintenanceExecutor(t *testing.T) (*Executor, *store.Store, model.Ship, *string) {
	t.Helper()
	ex, st, ship, _ := utilityMappingExecutor(t)
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	mode := "ready"
	sup := supervisor.New(st, func(_ string, args ...string) *exec.Cmd {
		cmd := exec.Command(binary, append([]string{"-test.run=^TestMaintenanceRuntimeHelper$", "--"}, args...)...)
		cmd.Env = append(os.Environ(), "HARBOR_MAINTENANCE_RUNTIME_HELPER="+mode)
		return cmd
	})
	ex.supervisor = sup
	ex.SetReady(true)
	ship.HTTPBindAddress = "127.0.0.1"
	ship.DesiredRunning = true
	if err := st.PutShip(context.Background(), ship); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := sup.Stop(ctx, ship.ID); err != nil {
			t.Error("fixture shutdown failed", err)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := sup.Start(ctx, "initial-start", ship.ID); err != nil {
		t.Fatal(err)
	}
	ship, err = st.GetShip(ctx, ship.ID)
	if err != nil {
		t.Fatal(err)
	}
	return ex, st, ship, &mode
}

func TestScheduledMaintenanceRestartsAndReleasesQueue(t *testing.T) {
	target, err := vr.NativePlatform()
	if err != nil {
		t.Fatal(err)
	}
	if target.Arch != "arm64" {
		t.Skip("the complete cycle includes Meld's measured ARM64 admission")
	}
	ctx := context.Background()
	ex, st, ship, _ := runningMaintenanceExecutor(t)
	ex.resources = func(context.Context, string) (scheduler.Resources, error) {
		hardware := "Apple M2 Max"
		if target.OS == "linux" {
			hardware = "Raspberry Pi 4 Model B Rev 1.4"
		}
		return scheduler.Resources{AvailableMemory: 8 << 30, TotalMemory: 32 << 30, FreeDisk: 20 << 30, SnapshotBytes: 800 << 20, Hardware: hardware}, nil
	}
	jobs := queueScheduledMaintenance(t, st, ship.ID)
	for i, kind := range []model.Operation{"pack", "meld", "roll", "chop"} {
		if err := ex.RunOne(ctx); err != nil {
			t.Fatal(err)
		}
		job, err := st.GetJob(ctx, jobs[i].ID)
		if err != nil || job.Kind != kind || job.Trigger != "scheduled" || job.Phase != "completed" || !job.PriorRunning || job.ExitCode == nil || *job.ExitCode != 0 {
			t.Fatalf("scheduled %s did not complete its running-ship cycle: %+v, %v", kind, job, err)
		}
		current, err := st.GetShip(ctx, ship.ID)
		if err != nil || current.State != "running" || !current.DesiredRunning || current.RecoveryBlocked {
			t.Fatalf("scheduled %s did not restore the ship: %+v, %v", kind, current, err)
		}
		records, err := st.ProcessesForJob(ctx, job.ID)
		if err != nil || len(records) != 2 || records[0].Kind != "utility" || records[0].Phase != "exited" || records[0].ExitCode == nil || *records[0].ExitCode != 0 || records[1].Kind != "ship" || records[1].Phase != "live" {
			t.Fatal("utility exit and restarted writer were not recorded", records, err)
		}
	}
	if pending, err := st.HasPendingJob(ctx, ship.ID); err != nil || pending {
		t.Fatal("completed maintenance left pending work", err)
	}
	for _, kind := range []model.Operation{"stop", "start"} {
		id, err := ex.Submit(ctx, model.JobRequest{ShipID: ship.ID, Kind: kind, Trigger: "manual"})
		if err != nil {
			t.Fatal("maintenance blocked the next lifecycle action", kind, err)
		}
		if err := ex.RunOne(ctx); err != nil {
			t.Fatal(err)
		}
		job, err := st.GetJob(ctx, id)
		if err != nil || job.Phase != "completed" {
			t.Fatal("post-maintenance lifecycle action did not complete", job, err)
		}
	}
}

func TestScheduledMaintenanceReadinessFailureSettlesJobAndRetainsWriter(t *testing.T) {
	ctx := context.Background()
	ex, st, ship, mode := runningMaintenanceExecutor(t)
	*mode = "unready"
	ex.readinessTimeout = 500 * time.Millisecond
	at := time.Now().UTC().Add(-time.Minute)
	sc := model.Schedule{ID: "pack", ShipID: ship.ID, Kind: "pack", AnchorDate: at.Format("2006-01-02"), IntervalDays: 1, LocalTime: at.Format("15:04"), Timezone: "UTC", Enabled: true, NextDue: at}
	if err := st.PutSchedule(ctx, sc); err != nil {
		t.Fatal(err)
	}
	if err := (scheduler.Worker{Store: st}).Tick(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	jobs, err := st.JobsForShip(ctx, ship.ID)
	if err != nil || len(jobs) != 1 {
		t.Fatal(jobs, err)
	}
	done := make(chan error, 1)
	go func() { done <- ex.RunOne(ctx) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		stop, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := ex.supervisor.Stop(stop, ship.ID)
		cancel()
		if err != nil {
			t.Fatal("fixture could not stop its unready writer", err)
		}
		<-done
		t.Fatal("maintenance restart never settled at its readiness deadline")
	}
	job, err := st.GetJob(ctx, jobs[0].ID)
	if err != nil || job.Phase != "failed" || job.ExitCode == nil || *job.ExitCode != 0 || job.FinishedAt.IsZero() {
		t.Fatal("successful utility / failed restart was not settled", job, err)
	}
	if pending, err := st.HasPendingJob(ctx, ship.ID); err != nil || pending {
		t.Fatal("readiness failure retained the active job", err)
	}
	current, err := st.GetShip(ctx, ship.ID)
	if err != nil || current.State != "failed" || !current.RecoveryBlocked || !current.DesiredRunning {
		t.Fatal("readiness failure remained starting or lost recovery protection", current.State, current.RecoveryBlocked, err)
	}
	records, err := st.ProcessesForJob(ctx, job.ID)
	if err != nil || len(records) != 2 || records[0].Phase != "exited" || records[1].Phase != "live" || records[1].PID <= 0 || records[1].StartIdentity == "" {
		t.Fatal("readiness timeout lost the utility outcome or live writer", records, err)
	}
	if release, err := pier.Guard(ship.PierPath); err == nil {
		release()
		t.Fatal("readiness timeout released a live writer's guard")
	}
	if err := ex.supervisor.Stop(ctx, ship.ID); err != nil {
		t.Fatal(err)
	}
	afterExit, err := st.GetShip(ctx, ship.ID)
	if err != nil || !afterExit.RecoveryBlocked || afterExit.State != "failed" || afterExit.Error != current.Error || afterExit.RecoveryBlockReason != current.RecoveryBlockReason {
		t.Fatal("process exit overwrote the failed restart's recovery state", afterExit.State, afterExit.RecoveryBlocked, err)
	}
	for _, next := range queueScheduledMaintenance(t, st, ship.ID) {
		if err := ex.RunOne(ctx); err != nil {
			t.Fatal(err)
		}
		job, err := st.GetJob(ctx, next.ID)
		if err != nil || job.Phase != "failed" {
			t.Fatal("following maintenance did not settle behind the recovery hold", job, err)
		}
		records, err := st.ProcessesForJob(ctx, job.ID)
		if err != nil || len(records) != 0 {
			t.Fatal("following maintenance launched through the recovery hold", records, err)
		}
	}
	if pending, err := st.HasPendingJob(ctx, ship.ID); err != nil || pending {
		t.Fatal("held following maintenance remained active", err)
	}
}
