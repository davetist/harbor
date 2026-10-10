package jobs

import (
	"context"
	"harbor/internal/model"
	"harbor/internal/store"
	"harbor/internal/supervisor"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCompletedUtilityHelper(t *testing.T) {
	if os.Getenv("HARBOR_COMPLETED_UTILITY_HELPER") == "1" {
		os.Exit(0)
	}
}

func TestFailedRestartAfterSuccessfulUtilityPersistsHold(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	db := filepath.Join(root, "db")
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p := filepath.Join(root, "pier")
	for _, dir := range []string{".urb/chk", ".urb/log"} {
		if err := os.MkdirAll(filepath.Join(p, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(p, ".urb/chk/image.bin"), []byte("snapshot"), 0600); err != nil {
		t.Fatal(err)
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	ship := model.Ship{ID: "ship", PierPath: p, RuntimePath: binary, State: "running", DesiredRunning: true, LoomExponent: 31, HTTPBindAddress: "127.0.0.1", WebPort: port}
	if err := st.PutShip(ctx, ship); err != nil {
		t.Fatal(err)
	}
	ship, err = st.GetShip(ctx, ship.ID)
	if err != nil {
		t.Fatal(err)
	}
	j := model.Job{ID: "replay", ShipID: ship.ID, Kind: "replay", Trigger: "manual", Phase: "queued", PriorRunning: true}
	if err := st.CreateJob(ctx, j); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HARBOR_COMPLETED_UTILITY_HELPER", "1")
	sup := supervisor.New(st, func(string, ...string) *exec.Cmd { return exec.Command("/bin/sh", "-c", "sleep 0.5; exit 1") })
	ex := New(st, sup)
	ex.SetReady(true)
	err = ex.maintainWith(ctx, &j, ship, []string{"-test.run=^TestCompletedUtilityHelper$"}, nil, nil)
	if err == nil {
		t.Fatal("failed restart reported success")
	}
	if j.ExitCode == nil || *j.ExitCode != 0 {
		t.Fatal("utility did not complete", j.ExitCode)
	}
	if err := ex.finish(ctx, &j, "failed", err); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetShip(ctx, ship.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.RecoveryBlocked || got.RecoveryBlockReason == "" {
		t.Fatal("failed restart lost recovery protection", got)
	}
	if _, err := ex.Submit(ctx, model.JobRequest{ShipID: ship.ID, Kind: "start", Trigger: "manual"}); err == nil {
		t.Fatal("start bypassed failed-restart hold")
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err = reopened.GetShip(ctx, ship.ID)
	if err != nil || !got.RecoveryBlocked {
		t.Fatal("hold did not survive restart", err)
	}
}
