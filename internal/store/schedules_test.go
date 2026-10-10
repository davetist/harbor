package store

import (
	"context"
	"errors"
	"harbor/internal/model"
	"path/filepath"
	"testing"
	"time"
)

func nextDay(s model.Schedule, after time.Time) (time.Time, error) {
	return after.Add(24 * time.Hour), nil
}
func TestDueClaimAndQueueSurviveReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "db")
	s, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	s.PutShip(ctx, model.Ship{ID: "ship", PierPath: "/test/pier"})
	now := time.Date(2026, 10, 8, 4, 0, 0, 0, time.UTC)
	schedule := model.Schedule{ID: "pack", ShipID: "ship", Kind: "pack", Enabled: true, NextDue: now.Add(-72 * time.Hour), IntervalDays: 1, AnchorDate: "2026-01-01", LocalTime: "04:00", Timezone: "UTC"}
	if e = s.PutSchedule(ctx, schedule); e != nil {
		t.Fatal(e)
	}
	jobs, e := s.ClaimDueAndEnqueue(ctx, now, nextDay)
	if e != nil || len(jobs) != 1 {
		t.Fatalf("%v %v", jobs, e)
	}
	if jobs[0].Trigger != "scheduled" || jobs[0].ScheduleID != schedule.ID || jobs[0].OccurrenceID == "" {
		t.Fatal("lost occurrence metadata")
	}
	s.Close()
	s, e = Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	again, e := s.ClaimDueAndEnqueue(ctx, now, nextDay)
	if e != nil || len(again) != 0 {
		t.Fatalf("duplicate occurrence: %v %v", again, e)
	}
	job, e := s.ClaimJob(ctx)
	if e != nil || job.ID != jobs[0].ID {
		t.Fatalf("lost durable queue: %v %v", job, e)
	}
}
func TestPairedChopReferencesSameCycleRoll(t *testing.T) {
	ctx := context.Background()
	s, e := Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	s.PutShip(ctx, model.Ship{ID: "ship", PierPath: "/test/pier"})
	now := time.Date(2026, 10, 8, 4, 0, 0, 0, time.UTC)
	roll := model.Schedule{ID: "roll", ShipID: "ship", Kind: "roll", Enabled: true, NextDue: now.Add(-2 * time.Hour), IntervalDays: 1, AnchorDate: "2026-01-01", LocalTime: "02:00", Timezone: "UTC"}
	chop := roll
	chop.ID = "chop"
	chop.Kind = "chop"
	chop.LocalTime = "03:00"
	chop.NextDue = now.Add(-time.Hour)
	chop.AfterScheduleID = roll.ID
	s.PutSchedule(ctx, roll)
	s.PutSchedule(ctx, chop)
	jobs, e := s.ClaimDueAndEnqueue(ctx, now, nextDay)
	if e != nil || len(jobs) != 2 {
		t.Fatalf("%v %v", jobs, e)
	}
	if jobs[1].DependsOnJobID != jobs[0].ID {
		t.Fatalf("wrong dependency: %v", jobs)
	}
}

func TestDueTransactionRollsBackEveryFailureWindow(t *testing.T) {
	for _, stage := range []string{"job", "occurrence", "advance"} {
		t.Run(stage, func(t *testing.T) {
			ctx := context.Background()
			p := filepath.Join(t.TempDir(), "db")
			s, e := Open(p)
			if e != nil {
				t.Fatal(e)
			}
			s.PutShip(ctx, model.Ship{ID: "ship", PierPath: "/test/pier"})
			now := time.Date(2026, 10, 8, 4, 0, 0, 0, time.UTC)
			s.PutSchedule(ctx, model.Schedule{ID: "pack", ShipID: "ship", Kind: "pack", Enabled: true, NextDue: now, AnchorDate: "2026-01-01", IntervalDays: 1, LocalTime: "04:00", Timezone: "UTC"})
			s.testClaimHook = func(at string) error {
				if at == stage {
					return errors.New("injected transaction interruption")
				}
				return nil
			}
			if _, e = s.ClaimDueAndEnqueue(ctx, now, nextDay); e == nil {
				t.Fatal("failure hook not reached")
			}
			s.Close()
			s, e = Open(p)
			if e != nil {
				t.Fatal(e)
			}
			defer s.Close()
			jobs, e := s.ClaimDueAndEnqueue(ctx, now, nextDay)
			if e != nil || len(jobs) != 1 {
				t.Fatalf("partial commit survived: %v %v", jobs, e)
			}
		})
	}
}
func TestTwoWorkersClaimOnce(t *testing.T) {
	ctx := context.Background()
	s, e := Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	s.PutShip(ctx, model.Ship{ID: "ship", PierPath: "/test/pier"})
	now := time.Date(2026, 10, 8, 4, 0, 0, 0, time.UTC)
	s.PutSchedule(ctx, model.Schedule{ID: "pack", ShipID: "ship", Kind: "pack", Enabled: true, NextDue: now, AnchorDate: "2026-01-01", IntervalDays: 1, LocalTime: "04:00", Timezone: "UTC"})
	results := make(chan int, 2)
	errs := make(chan error, 2)
	for range 2 {
		go func() { jobs, e := s.ClaimDueAndEnqueue(ctx, now, nextDay); results <- len(jobs); errs <- e }()
	}
	if a, b := <-results, <-results; a+b != 1 {
		t.Fatalf("claimed %d %d", a, b)
	}
	for range 2 {
		if e = <-errs; e != nil {
			t.Fatal(e)
		}
	}
}
