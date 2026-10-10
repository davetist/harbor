package store

import (
	"context"
	"harbor/internal/model"
	"path/filepath"
	"testing"
)

func TestRetryPrelaunchDoesNotRegisterAnotherShipOrRetryUncertainWork(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ship := model.Ship{ID: "s", PierPath: filepath.Join(t.TempDir(), "new")}
	s.PutShip(ctx, ship)
	j := model.Job{ID: "first", ShipID: ship.ID, Kind: "boot_comet", Trigger: "manual", Phase: "queued"}
	s.CreateJob(ctx, j)
	j.Phase = "failed"
	s.UpdateJob(ctx, j)
	retried, err := s.RetryPrelaunch(ctx, j.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if retried.ShipID != ship.ID {
		t.Fatal("changed registration")
	}
	if _, err = s.RetryPrelaunch(ctx, j.ID, ""); err == nil {
		t.Fatal("duplicate pending retry")
	}
	retried.Phase = "failed"
	s.UpdateJob(ctx, retried)
	s.PutProcess(ctx, model.ProcessRecord{LaunchID: "intent", ShipID: ship.ID, JobID: retried.ID, Phase: "launching"})
	if _, err = s.RetryPrelaunch(ctx, retried.ID, ""); err == nil {
		t.Fatal("retried uncertain launch")
	}
}
