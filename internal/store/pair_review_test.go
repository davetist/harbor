package store

import (
	"context"
	"harbor/internal/model"
	"path/filepath"
	"testing"
	"time"
)

func TestReviewPairedCadenceEditable(t *testing.T) {
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	st.PutShip(ctx, model.Ship{ID: "review", PierPath: t.TempDir()})
	roll := model.Schedule{ID: "r", ShipID: "review", Kind: "roll", IntervalDays: 7, AnchorDate: "2026-10-01", LocalTime: "02:00", Timezone: "UTC"}
	chop := roll
	chop.ID = "c"
	chop.Kind = "chop"
	chop.LocalTime = "03:00"
	chop.AfterScheduleID = "r"
	if err = st.PutSchedule(ctx, roll); err != nil {
		t.Fatal(err)
	}
	if err = st.PutSchedule(ctx, chop); err != nil {
		t.Fatal(err)
	}
	roll.IntervalDays = 14
	chop.IntervalDays = 14
	next := func(v model.Schedule) (time.Time, error) { return time.Date(2026, 10, 15, 2, 0, 0, 0, time.UTC), nil }
	if err = st.EditSchedule(ctx, roll, next); err != nil {
		t.Fatal(err)
	}
	rows, _ := st.ListSchedules(ctx, "review")
	for _, v := range rows {
		if v.IntervalDays != 14 {
			t.Fatal("paired cadence was not updated atomically")
		}
	}
	chop.IntervalDays = 21
	if err = st.EditSchedule(ctx, chop, next); err != nil {
		t.Fatal(err)
	}
	rows, _ = st.ListSchedules(ctx, "review")
	for _, v := range rows {
		if v.IntervalDays != 21 {
			t.Fatal("reverse paired edit failed")
		}
	}
}
