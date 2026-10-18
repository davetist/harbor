package store

import (
	"context"
	"harbor/internal/model"
	"path/filepath"
	"testing"
	"time"
)

func TestRemovalRollsBackManagementRecordsOnFailure(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ship := model.Ship{ID: "comet", PierPath: t.TempDir(), State: "stopped"}
	if err = s.PutShip(ctx, ship); err != nil {
		t.Fatal(err)
	}
	if err = s.PutSchedule(ctx, model.Schedule{ID: "pack", ShipID: ship.ID, Kind: "pack", IntervalDays: 7, AnchorDate: "2026-01-01", LocalTime: "03:00", Timezone: "UTC", NextDue: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`CREATE TRIGGER refuse_removal BEFORE DELETE ON ships BEGIN SELECT RAISE(ABORT, 'injected failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err = s.RemoveShip(ctx, ship.ID); err == nil {
		t.Fatal("injected failure was ignored")
	}
	if _, err = s.GetShip(ctx, ship.ID); err != nil {
		t.Fatalf("registration was lost: %v", err)
	}
	rows, err := s.ListSchedules(ctx, ship.ID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("settings were lost: %v %v", rows, err)
	}
}

func TestPendingDeletionRefusesOverlappingRegistrations(t *testing.T) {
	for _, registration := range []string{"put", "queued", "pending"} {
		t.Run(registration, func(t *testing.T) {
			ctx := context.Background()
			s, err := Open(filepath.Join(t.TempDir(), "db"))
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			ship := model.Ship{ID: "deleting", PierPath: t.TempDir(), State: "stopped"}
			if err = s.PutShip(ctx, ship); err != nil {
				t.Fatal(err)
			}
			ship, _ = s.GetShip(ctx, ship.ID)
			if _, err = s.BeginPierDeletion(ctx, ship.ID, ship.PierPath, "original-folder"); err != nil {
				t.Fatal(err)
			}
			for _, overlap := range []bool{true, false} {
				path := filepath.Join(ship.PierPath, "nested")
				if !overlap {
					path = filepath.Join(filepath.Dir(ship.PierPath), "sibling")
				}
				other := model.Ship{ID: model.NewID(), PierPath: path, State: "stopped"}
				job := model.Job{ID: model.NewID(), ShipID: other.ID, Kind: "import", Trigger: "manual", Phase: "queued"}
				switch registration {
				case "put":
					err = s.PutShip(ctx, other)
				case "queued":
					err = s.RegisterQueued(ctx, other, job)
				case "pending":
					job.Kind = "import_archive"
					err = s.QueuePendingImport(ctx, other, job)
				}
				if (err != nil) != overlap {
					t.Fatalf("overlap=%v, registration=%s: %v", overlap, registration, err)
				}
			}
		})
	}
}

func TestDeletionHoldKeepsSchedulesPausedAndSurvivesRemovalFailure(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ship := model.Ship{ID: "comet", PierPath: t.TempDir(), State: "stopped"}
	if err = s.PutShip(ctx, ship); err != nil {
		t.Fatal(err)
	}
	ship, _ = s.GetShip(ctx, ship.ID)
	schedule := model.Schedule{ID: "pack", ShipID: ship.ID, Kind: "pack", Enabled: true, IntervalDays: 7, AnchorDate: "2026-01-01", LocalTime: "03:00", Timezone: "UTC", NextDue: time.Now()}
	if err = s.PutSchedule(ctx, schedule); err != nil {
		t.Fatal(err)
	}
	if _, err = s.BeginPierDeletion(ctx, ship.ID, ship.PierPath, "original"); err != nil {
		t.Fatal(err)
	}
	rows, err := s.ListSchedules(ctx, ship.ID)
	if err != nil || len(rows) != 1 || rows[0].Enabled {
		t.Fatalf("schedule was not paused: %+v %v", rows, err)
	}
	if err = s.PutSchedule(ctx, schedule); err == nil {
		t.Fatal("maintenance was re-enabled during deletion")
	}
	if _, err = s.db.Exec(`CREATE TRIGGER refuse_delete BEFORE DELETE ON ships BEGIN SELECT RAISE(ABORT,'injected'); END`); err != nil {
		t.Fatal(err)
	}
	if err = s.CompletePierDeletion(ctx, ship.ID, "original"); err == nil {
		t.Fatal("ignored failed removal")
	}
	held, err := s.GetShip(ctx, ship.ID)
	if err != nil || !held.RecoveryBlocked || held.PierDeletionIdentity != "original" {
		t.Fatalf("lost hold: %+v %v", held, err)
	}
}
