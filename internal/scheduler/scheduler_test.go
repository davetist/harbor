package scheduler

import (
	"harbor/internal/model"
	"testing"
	"time"
)

func TestCalendarDaysPreserveLocalTime(t *testing.T) {
	s := model.Schedule{AnchorDate: "2026-10-01", IntervalDays: 1, Kind: "pack", LocalTime: "04:00", Timezone: "Europe/Madrid"}
	a := time.Date(2026, 10, 24, 3, 0, 0, 0, time.UTC)
	next, e := NextOccurrence(s, a)
	if e != nil {
		t.Fatal(e)
	}
	zone, _ := time.LoadLocation(s.Timezone)
	if next.In(zone).Hour() != 4 || next.In(zone).Day() != 25 {
		t.Fatal(next)
	}
}
func TestDSTGapUsesNextValidInstant(t *testing.T) {
	s := model.Schedule{AnchorDate: "2026-01-01", IntervalDays: 1, Kind: "pack", LocalTime: "02:30", Timezone: "America/New_York"}
	next, e := NextOccurrence(s, time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC))
	if e != nil {
		t.Fatal(e)
	}
	want := time.Date(2026, 3, 8, 7, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("%s != %s", next, want)
	}
}
func TestDSTFoldRunsOnce(t *testing.T) {
	s := model.Schedule{AnchorDate: "2026-01-01", IntervalDays: 1, Kind: "pack", LocalTime: "01:30", Timezone: "America/New_York"}
	first, e := NextOccurrence(s, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC))
	if e != nil {
		t.Fatal(e)
	}
	if !first.Equal(time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC)) {
		t.Fatal(first)
	}
	second, e := NextOccurrence(s, first)
	if e != nil {
		t.Fatal(e)
	}
	if second.Day() != 2 {
		t.Fatal("fold dispatched twice", second)
	}
}
func TestAnchorIntervalAndInvalidInput(t *testing.T) {
	s := model.Schedule{AnchorDate: "2026-10-01", IntervalDays: 7, Kind: "pack", LocalTime: "03:00", Timezone: "UTC"}
	next, e := NextOccurrence(s, time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC))
	if e != nil || next.Day() != 8 {
		t.Fatalf("%s %v", next, e)
	}
	s.IntervalDays = 0
	if _, e = NextOccurrence(s, time.Now()); e == nil {
		t.Fatal("zero interval accepted")
	}
}
