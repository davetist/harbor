package store

import (
	"context"
	"errors"
	"harbor/internal/model"
	"path/filepath"
	"testing"
	"time"
)

// Keep the former clock settings and insert Roll before compaction to exercise
// existing registrations, rather than relying on the new profile's row order.
func maintenanceSchedules(t *testing.T, s *Store, first time.Time) {
	t.Helper()
	ctx := context.Background()
	if err := s.PutShip(ctx, model.Ship{ID: "ship", PierPath: "/test/pier"}); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		kind         model.Operation
		minute, days int
	}{{"roll", 5, 7}, {"chop", 10, 7}, {"meld", 15, 28}, {"pack", 0, 7}} {
		sc := model.Schedule{ID: model.ID(row.kind), ShipID: "ship", Kind: row.kind, Enabled: true, AnchorDate: first.Format("2006-01-02"), IntervalDays: row.days, LocalTime: first.Add(time.Duration(row.minute) * time.Minute).Format("15:04"), Timezone: "UTC", NextDue: first.Add(time.Duration(row.minute) * time.Minute)}
		if sc.Kind == "chop" {
			sc.AfterScheduleID = "roll"
		}
		if err := s.PutSchedule(ctx, sc); err != nil {
			t.Fatal(err)
		}
	}
}

func nextMaintenanceDay(sc model.Schedule, after time.Time) (time.Time, error) {
	return after.AddDate(0, 0, sc.IntervalDays), nil
}

func claimMaintenanceOrder(t *testing.T, s *Store, now time.Time) {
	t.Helper()
	ctx := context.Background()
	for _, kind := range []model.Operation{"pack", "meld", "roll", "chop"} {
		j, err := s.claimJob(ctx, now)
		if err != nil || j.Kind != kind {
			t.Fatalf("wanted %s next, got %s: %v", kind, j.Kind, err)
		}
		if _, err = s.claimJob(ctx, now); !errors.Is(err, ErrNotFound) {
			t.Fatalf("another automatic operation became eligible while %s was active: %v", kind, err)
		}
		j.Phase = "completed"
		if err = s.UpdateJob(ctx, j); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMaintenanceCatchUpOrderSurvivesReopen(t *testing.T) {
	for _, firstDay := range []int{3, 10} {
		t.Run(time.Date(2026, 9, firstDay, 0, 0, 0, 0, time.UTC).Format("2006-01-02"), func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "db")
			s, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			maintenanceSchedules(t, s, time.Date(2026, 9, firstDay, 3, 0, 0, 0, time.UTC))
			now := time.Date(2026, 10, 8, 4, 0, 0, 0, time.UTC)
			jobs, err := s.ClaimDueAndEnqueue(ctx, now, nextMaintenanceDay)
			if err != nil || len(jobs) != 4 {
				t.Fatalf("catch-up: %v %v", jobs, err)
			}
			if err = s.Close(); err != nil {
				t.Fatal(err)
			}
			s, err = Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if again, err := s.ClaimDueAndEnqueue(ctx, now, nextMaintenanceDay); err != nil || len(again) != 0 {
				t.Fatalf("duplicate catch-up: %v %v", again, err)
			}
			claimMaintenanceOrder(t, s, now)
		})
	}
}

func TestMaintenanceDispatchOrdersPreviouslyQueuedJobs(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	maintenanceSchedules(t, s, time.Date(2026, 10, 8, 3, 0, 0, 0, time.UTC))
	if _, err = s.ClaimDueAndEnqueue(ctx, time.Date(2026, 10, 8, 4, 0, 0, 0, time.UTC), nextMaintenanceDay); err != nil {
		t.Fatal(err)
	}
	// Represent a queue saved by an older controller with retention first.
	if _, err = s.db.Exec("UPDATE jobs SET sequence=CASE kind WHEN 'roll' THEN -4 WHEN 'chop' THEN -3 WHEN 'meld' THEN -2 ELSE -1 END"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	claimMaintenanceOrder(t, s, time.Date(2026, 10, 8, 4, 0, 0, 0, time.UTC))
}

func TestMaintenanceWaitsForLaterCompactionClock(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	maintenanceSchedules(t, s, time.Date(2026, 10, 8, 3, 0, 0, 0, time.UTC))
	now := time.Date(2026, 10, 8, 3, 6, 0, 0, time.UTC)
	jobs, err := s.ClaimDueAndEnqueue(ctx, now, nextMaintenanceDay)
	if err != nil || len(jobs) != 1 || jobs[0].Kind != "pack" {
		t.Fatalf("Roll overtook today's later Meld: %v %v", jobs, err)
	}
	pack, err := s.claimJob(ctx, now)
	if err != nil || pack.Kind != "pack" {
		t.Fatal(pack, err)
	}
	pack.Phase = "completed"
	s.UpdateJob(ctx, pack)
	if jobs, err = s.ClaimDueAndEnqueue(ctx, now.Add(5*time.Minute), nextMaintenanceDay); err != nil || len(jobs) != 0 {
		t.Fatalf("retention overtook Meld: %v %v", jobs, err)
	}
	if jobs, err = s.ClaimDueAndEnqueue(ctx, now.Add(9*time.Minute), nextMaintenanceDay); err != nil || len(jobs) != 3 {
		t.Fatalf("Meld/retention did not become eligible: %v %v", jobs, err)
	}
	for _, kind := range []model.Operation{"meld", "roll", "chop"} {
		j, err := s.claimJob(ctx, now.Add(9*time.Minute))
		if err != nil || j.Kind != kind {
			t.Fatal(kind, j, err)
		}
		j.Phase = "completed"
		s.UpdateJob(ctx, j)
	}
}

func TestManualChopRemainsIndependentOfAutomaticCompaction(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	maintenanceSchedules(t, s, time.Date(2026, 10, 8, 3, 0, 0, 0, time.UTC))
	if _, err = s.ClaimDueAndEnqueue(ctx, time.Date(2026, 10, 8, 4, 0, 0, 0, time.UTC), nextMaintenanceDay); err != nil {
		t.Fatal(err)
	}
	// A manual Chop keeps its ordinary queue position, without an automatic
	// compaction dependency. The executor still serializes actual processes.
	j := model.Job{ID: "manual-chop", ShipID: "ship", Kind: "chop", Trigger: "manual", Phase: "queued"}
	if err = s.CreateJob(ctx, j); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec("UPDATE jobs SET sequence=-1 WHERE id='manual-chop'"); err != nil {
		t.Fatal(err)
	}
	got, err := s.ClaimJob(ctx)
	if err != nil || got.ID != j.ID || got.DependsOnJobID != "" {
		t.Fatalf("manual Chop acquired automatic dependencies: %v %v", got, err)
	}
}

func TestPausedOrNotDueMeldDoesNotDelayRetention(t *testing.T) {
	for _, paused := range []bool{false, true} {
		t.Run(map[bool]string{false: "not due", true: "paused"}[paused], func(t *testing.T) {
			ctx := context.Background()
			s, err := Open(filepath.Join(t.TempDir(), "db"))
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			maintenanceSchedules(t, s, time.Date(2026, 10, 8, 3, 0, 0, 0, time.UTC))
			rows, err := s.ListSchedules(ctx, "ship")
			if err != nil {
				t.Fatal(err)
			}
			for _, sc := range rows {
				if sc.Kind == "meld" {
					sc.Enabled = !paused
					if !paused {
						sc.NextDue = sc.NextDue.AddDate(0, 0, 28)
					}
					if err = s.PutSchedule(ctx, sc); err != nil {
						t.Fatal(err)
					}
				}
			}
			if jobs, err := s.ClaimDueAndEnqueue(ctx, time.Date(2026, 10, 8, 3, 11, 0, 0, time.UTC), nextMaintenanceDay); err != nil || len(jobs) != 3 {
				t.Fatalf("optional Meld blocked retention: %v %v", jobs, err)
			}
			for _, kind := range []model.Operation{"pack", "roll", "chop"} {
				j, err := s.claimJob(ctx, time.Date(2026, 10, 8, 3, 11, 0, 0, time.UTC))
				if err != nil || j.Kind != kind {
					t.Fatal(kind, j, err)
				}
				j.Phase = "completed"
				if err = s.UpdateJob(ctx, j); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestQueuedRollWaitsForUnclaimedCompaction(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	maintenanceSchedules(t, s, time.Date(2026, 10, 8, 3, 0, 0, 0, time.UTC))
	rows, err := s.ListSchedules(ctx, "ship")
	if err != nil {
		t.Fatal(err)
	}
	var meld model.Schedule
	for _, sc := range rows {
		if sc.Kind == "meld" {
			meld = sc
		}
	}
	meld.Enabled = false
	if err = s.PutSchedule(ctx, meld); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 8, 3, 11, 0, 0, time.UTC)
	if _, err = s.ClaimDueAndEnqueue(ctx, now, nextMaintenanceDay); err != nil {
		t.Fatal(err)
	}
	pack, err := s.claimJob(ctx, now)
	if err != nil || pack.Kind != "pack" {
		t.Fatal(pack, err)
	}
	pack.Phase = "completed"
	if err = s.UpdateJob(ctx, pack); err != nil {
		t.Fatal(err)
	}
	// Re-enabling a compaction schedule must also guard retention that is
	// already queued, including a queue restored before the scheduler ticks.
	meld.Enabled = true
	if err = s.PutSchedule(ctx, meld); err != nil {
		t.Fatal(err)
	}
	if j, err := s.claimJob(ctx, now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("queued retention bypassed unclaimed Meld: %v %v", j, err)
	}
	if _, err = s.ClaimDueAndEnqueue(ctx, now.Add(4*time.Minute), nextMaintenanceDay); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []model.Operation{"meld", "roll", "chop"} {
		j, err := s.claimJob(ctx, now.Add(4*time.Minute))
		if err != nil || j.Kind != kind {
			t.Fatal(kind, j, err)
		}
		j.Phase = "completed"
		if err = s.UpdateJob(ctx, j); err != nil {
			t.Fatal(err)
		}
	}
}

func TestOldQueuedRollWaitsForCurrentlyDueCompaction(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	first := time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC)
	maintenanceSchedules(t, s, first)
	if _, err = s.ClaimDueAndEnqueue(ctx, first.Add(time.Hour), nextMaintenanceDay); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []model.Operation{"pack", "meld"} {
		j, err := s.claimJob(ctx, first.Add(time.Hour))
		if err != nil || j.Kind != kind {
			t.Fatal(kind, j, err)
		}
		j.Phase = "completed"
		if err = s.UpdateJob(ctx, j); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := first.AddDate(0, 0, 7).Add(time.Hour)
	// The executor may poll before the scheduler's first startup tick.
	if j, err := s.claimJob(ctx, now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("old retention ran before currently due Pack was claimed: %v %v", j, err)
	}
	if _, err = s.ClaimDueAndEnqueue(ctx, now, nextMaintenanceDay); err != nil {
		t.Fatal(err)
	}
	j, err := s.claimJob(ctx, now)
	if err != nil || j.Kind != "pack" {
		t.Fatalf("current compaction did not run first: %v %v", j, err)
	}
	j.Phase = "completed"
	if err = s.UpdateJob(ctx, j); err != nil {
		t.Fatal(err)
	}
	j, err = s.claimJob(ctx, now)
	if err != nil || j.Kind != "roll" {
		t.Fatalf("old Roll did not resume after compaction: %v %v", j, err)
	}
}

func TestMissingLocalMidnightDoesNotBypassLaterPack(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.PutShip(ctx, model.Ship{ID: "ship", PierPath: "/test/pier"}); err != nil {
		t.Fatal(err)
	}
	// Santiago skips midnight on this day. Its 23:30 Pack must still precede
	// the morning Roll; constructing a normalized midnight loses that hour.
	pack := model.Schedule{ID: "pack", ShipID: "ship", Kind: "pack", Enabled: true, AnchorDate: "2026-09-06", IntervalDays: 7, LocalTime: "23:30", Timezone: "America/Santiago", NextDue: time.Date(2026, 9, 7, 2, 30, 0, 0, time.UTC)}
	roll := pack
	roll.ID, roll.Kind, roll.LocalTime = "roll", "roll", "03:00"
	roll.NextDue = time.Date(2026, 9, 6, 6, 0, 0, 0, time.UTC)
	for _, sc := range []model.Schedule{roll, pack} {
		if err = s.PutSchedule(ctx, sc); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2026, 9, 6, 7, 0, 0, 0, time.UTC)
	if jobs, err := s.ClaimDueAndEnqueue(ctx, now, nextMaintenanceDay); err != nil || len(jobs) != 0 {
		t.Fatalf("Roll bypassed its later same-day Pack: %v %v", jobs, err)
	}
}
