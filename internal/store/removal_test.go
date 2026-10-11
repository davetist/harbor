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
