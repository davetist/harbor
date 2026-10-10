package store

import (
	"context"
	"errors"
	"harbor/internal/model"
	"path/filepath"
	"testing"
)

func TestShipSettingsSurviveReopen(t *testing.T) {
	ctx := context.Background()
	p := filepath.Join(t.TempDir(), "harbor.sqlite")
	s, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	ship := model.Ship{ID: "ship-a", Name: "~sampel-palnet", PierPath: "/test/pier", RuntimePath: "/test/vere", LoomExponent: 32, HTTPBindAddress: "127.0.0.1", WebPort: 8080, RecoveryBlocked: true, RecoveryBlockReason: "uncertain utility", State: "failed"}
	if err = s.PutShip(ctx, ship); err != nil {
		t.Fatal(err)
	}
	proc := model.ProcessRecord{LaunchID: "launch-a", JobID: "job-a", ShipID: ship.ID, Kind: "utility", Phase: "live", PID: 123, StartIdentity: "test-start", BinaryDigest: "sha"}
	if err = s.PutProcess(ctx, proc); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, err := s.GetShip(ctx, ship.ID)
	if err != nil || got != ship {
		t.Fatalf("lost settings: %+v %v", got, err)
	}
	processes, err := s.ListOpenProcesses(ctx)
	if err != nil || len(processes) != 1 || processes[0].StartIdentity != "test-start" {
		t.Fatalf("lost process: %+v %v", processes, err)
	}
}
func TestDuplicateCanonicalPierRejected(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	a := model.Ship{ID: "a", PierPath: "/test/pier"}
	if err = s.PutShip(ctx, a); err != nil {
		t.Fatal(err)
	}
	a.ID = "b"
	if err = s.PutShip(ctx, a); err == nil {
		t.Fatal("registered second writer for same pier")
	}
}
func TestJobTriggerAndDependencySurviveReopen(t *testing.T) {
	p := filepath.Join(t.TempDir(), "db")
	s, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	j := model.Job{ID: "job-a", ShipID: "a", Kind: "pack", Trigger: "manual", Phase: "queued", DependsOnJobID: "roll-123"}
	if err = s.CreateJob(ctx, j); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, err := s.GetJob(ctx, j.ID)
	if err != nil || got.Trigger != "manual" || got.DependsOnJobID != "roll-123" {
		t.Fatalf("lost metadata: %+v %v", got, err)
	}
	first, err := s.ClaimJob(ctx)
	if err != nil || first.ID != j.ID {
		t.Fatalf("claim: %+v %v", first, err)
	}
	if _, err = s.ClaimJob(ctx); !errors.Is(err, ErrNotFound) {
		t.Fatalf("claimed twice: %v", err)
	}
	if err = s.CreateJob(ctx, model.Job{ID: "schedule-direct", Trigger: "scheduled"}); err == nil {
		t.Fatal("scheduled job bypassed occurrence transaction")
	}
}

func TestDependentJobWaitsForExactPrerequisite(t *testing.T) {
	ctx := context.Background()
	s, e := Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	s.CreateJob(ctx, model.Job{ID: "chop", ShipID: "a", Kind: "chop", Trigger: "manual", Phase: "queued", DependsOnJobID: "roll"})
	s.CreateJob(ctx, model.Job{ID: "roll", ShipID: "a", Kind: "roll", Trigger: "manual", Phase: "queued"})
	j, e := s.ClaimJob(ctx)
	if e != nil || j.ID != "roll" {
		t.Fatalf("dependent ran early: %s %v", j.ID, e)
	}
	if _, e = s.ClaimJob(ctx); !errors.Is(e, ErrNotFound) {
		t.Fatal("dependent ran during prerequisite")
	}
	j.Phase = "failed"
	s.UpdateJob(ctx, j)
	j, e = s.ClaimJob(ctx)
	if e != nil || j.ID != "chop" {
		t.Fatal("failed prerequisite did not become eligible for explicit skip")
	}
}

func TestInterruptedHoldTransactionRollsBack(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ship := model.Ship{ID: "s", PierPath: t.TempDir(), DesiredRunning: true}
	s.PutShip(ctx, ship)
	j := model.Job{ID: "j", ShipID: ship.ID, Kind: "pack", Trigger: "manual", Phase: "queued"}
	s.CreateJob(ctx, j)
	j.Phase = "executing"
	s.UpdateJob(ctx, j)
	s.testRecoveryHook = func() error { return errors.New("second crash") }
	j.Phase = "interrupted"
	j.Error = "uncertain"
	if err = s.InterruptAndHold(ctx, j); err == nil {
		t.Fatal("injected failure ignored")
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, _ := s.GetJob(ctx, j.ID)
	if got.Phase != "executing" {
		t.Fatalf("interruption escaped transaction: %s", got.Phase)
	}
	if err = s.InterruptAndHold(ctx, j); err != nil {
		t.Fatal(err)
	}
	held, _ := s.GetShip(ctx, ship.ID)
	got, _ = s.GetJob(ctx, j.ID)
	if !held.RecoveryBlocked || got.Phase != "interrupted" {
		t.Fatal("job and hold were not committed together")
	}
}
