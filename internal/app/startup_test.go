package app

import (
	"context"
	"harbor/internal/jobs"
	"harbor/internal/model"
	"harbor/internal/store"
	"harbor/internal/supervisor"
	"path/filepath"
	"testing"
)

func TestRecoveryPrecedesAutostartAndLeavesOtherShipsEligible(t *testing.T) {
	ctx := context.Background()
	st, e := store.Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer st.Close()
	for _, id := range []model.ID{"uncertain", "ordinary"} {
		st.PutShip(ctx, model.Ship{ID: id, PierPath: t.TempDir(), DesiredRunning: true, State: "stopped"})
	}
	j := model.Job{ID: "pack", ShipID: "uncertain", Kind: "pack", Trigger: "manual", Phase: "queued"}
	st.CreateJob(ctx, j)
	j.Phase = "executing"
	st.UpdateJob(ctx, j)
	sup := supervisor.New(st, nil)
	ex := jobs.New(st, sup)
	if e = RecoverAndStart(ctx, st, sup, ex); e != nil {
		t.Fatal(e)
	}
	held, _ := st.GetShip(ctx, "uncertain")
	if !held.RecoveryBlocked {
		t.Fatal("uncertain pier not held before autostart")
	}
	queued, e := st.ClaimJob(ctx)
	if e != nil || queued.ShipID != "ordinary" || queued.Kind != "start" || queued.Trigger != "system" {
		t.Fatalf("eligible ship did not recover: %+v %v", queued, e)
	}
	if _, e = ex.Submit(ctx, model.JobRequest{ShipID: "uncertain", Kind: "start", Trigger: "manual"}); e == nil {
		t.Fatal("manual start bypassed recovered hold")
	}
}
