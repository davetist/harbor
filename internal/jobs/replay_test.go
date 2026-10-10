package jobs

import (
	"context"
	"harbor/internal/model"
	"harbor/internal/store"
	"harbor/internal/supervisor"
	"os"
	"path/filepath"
	"testing"
)

func TestReplayRequiresVerifiedBackup(t *testing.T) {
	ctx := context.Background()
	st, e := store.Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer st.Close()
	st.PutShip(ctx, model.Ship{ID: "ship", PierPath: t.TempDir(), LoomExponent: 31, State: "stopped"})
	ex := New(st, supervisor.New(st, nil))
	ex.EnableReplay()
	ex.SetReady(true)
	id, e := ex.Submit(ctx, model.JobRequest{ShipID: "ship", Kind: "replay", Trigger: "manual"})
	if e != nil {
		t.Fatal(e)
	}
	if e = ex.RunOne(ctx); e != nil {
		t.Fatal(e)
	}
	j, e := st.GetJob(ctx, id)
	if e != nil || j.Phase != "failed" || j.Error != "a verified full-pier export is required" {
		t.Fatalf("replay bypassed backup: %+v %v", j, e)
	}
	p, _ := st.ListOpenProcesses(ctx)
	if len(p) != 0 {
		t.Fatal("replay launched without backup")
	}
}

func TestUpdateFailurePreservesUsableRuntime(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	st, e := store.Open(filepath.Join(root, "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer st.Close()
	runtime, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	ship := model.Ship{ID: "ship", PierPath: t.TempDir(), RuntimePath: runtime, LoomExponent: 31, State: "running"}
	st.PutShip(ctx, ship)
	blocked := filepath.Join(root, "blocked")
	os.WriteFile(blocked, []byte("not a directory"), 0600)
	ex := New(st, supervisor.New(st, nil))
	ex.EnableRuntimeUpdates(filepath.Join(blocked, "runtime"))
	ex.SetReady(true)
	id, e := ex.Submit(ctx, model.JobRequest{ShipID: ship.ID, Kind: "update", Trigger: "manual"})
	if e != nil {
		t.Fatal(e)
	}
	ex.RunOne(ctx)
	j, _ := st.GetJob(ctx, id)
	if j.Phase != "failed" {
		t.Fatal("expected preflight failure")
	}
	got, _ := st.GetShip(ctx, ship.ID)
	if got.RuntimePath != runtime || got.State != "running" {
		t.Fatalf("failed download disturbed working runtime: %+v", got)
	}
}
