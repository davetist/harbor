package store

import (
	"context"
	"harbor/internal/model"
	"path/filepath"
	"testing"
	"time"
)

func TestIdleRunningActionSerializesScheduledAdmission(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ship := model.Ship{ID: "ship", PierPath: "/test/pier", State: "running"}
	if err := st.PutShip(ctx, ship); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := st.PutSchedule(ctx, model.Schedule{ID: "pack", ShipID: ship.ID, Kind: "pack", Enabled: true, NextDue: now, IntervalDays: 7, AnchorDate: "2026-01-01", LocalTime: "03:00", Timezone: "UTC"}); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	actionDone := make(chan error, 1)
	go func() {
		actionDone <- st.WithIdleRunningShip(ctx, ship.ID, func(model.Ship) error {
			close(entered)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	<-entered
	scheduled := make(chan error, 1)
	go func() {
		queued, err := st.ClaimDueAndEnqueue(ctx, now, func(model.Schedule, time.Time) (time.Time, error) { return now.Add(7 * 24 * time.Hour), nil })
		if err == nil && len(queued) != 1 {
			t.Error("due maintenance was lost")
		}
		scheduled <- err
	}()
	for st.db.Stats().WaitCount == 0 && ctx.Err() == nil {
		time.Sleep(time.Millisecond)
	}
	select {
	case <-scheduled:
		t.Fatal("scheduled work passed admission during the guarded action")
	default:
	}
	close(release)
	if err := <-actionDone; err != nil {
		t.Fatal(err)
	}
	if err := <-scheduled; err != nil {
		t.Fatal(err)
	}
	if pending, err := st.HasPendingJob(ctx, ship.ID); err != nil || !pending {
		t.Fatal("scheduled work was not admitted after the action", err)
	}
}

func TestIdleRunningActionRefusesHoldsStatesAndPendingWork(t *testing.T) {
	for _, mode := range []string{"hold", "stopping", "pending"} {
		t.Run(mode, func(t *testing.T) {
			st, err := Open(filepath.Join(t.TempDir(), "db"))
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()
			ctx := context.Background()
			ship := model.Ship{ID: "ship", PierPath: "/test/pier", State: "running", RecoveryBlocked: mode == "hold"}
			if mode == "stopping" {
				ship.State = "stopping"
			}
			if err := st.PutShip(ctx, ship); err != nil {
				t.Fatal(err)
			}
			if mode == "pending" {
				if err := st.CreateJob(ctx, model.Job{ID: "job", ShipID: ship.ID, Kind: "stop", Trigger: "manual", Phase: "queued"}); err != nil {
					t.Fatal(err)
				}
			}
			called := false
			err = st.WithIdleRunningShip(ctx, ship.ID, func(model.Ship) error { called = true; return nil })
			if err == nil || called {
				t.Fatal("unsafe action reached its callback")
			}
		})
	}
}
