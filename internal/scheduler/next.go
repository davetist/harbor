package scheduler

import (
	"errors"
	"fmt"
	"harbor/internal/model"
	"time"
)

func Validate(s model.Schedule) error {
	if s.IntervalDays < 1 || s.IntervalDays > 365 {
		return errors.New("interval must be 1–365 calendar days")
	}
	if _, e := time.Parse("2006-01-02", s.AnchorDate); e != nil {
		return errors.New("invalid schedule anchor date")
	}
	if _, e := time.Parse("15:04", s.LocalTime); e != nil {
		return errors.New("time must be HH:mm")
	}
	if _, e := time.LoadLocation(s.Timezone); e != nil {
		return errors.New("invalid IANA timezone")
	}
	switch s.Kind {
	case "pack", "meld", "roll", "chop":
	default:
		return errors.New("operation cannot be scheduled")
	}
	return nil
}
func ordinal(t time.Time) int64 {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Unix() / 86400
}
func localInstant(day time.Time, hour, minute int, zone *time.Location) (time.Time, error) {
	y, m, d := day.Date()
	targetDate := day.Format("2006-01-02")
	center := time.Date(y, m, d, 12, 0, 0, 0, time.UTC)
	var best time.Time
	bestClock := 24*60 + 1
	// Enumerate UTC minutes to disambiguate folds and choose the next valid
	// local minute in a gap. Fixed-duration advancement is never used for days.
	for at := center.Add(-24 * time.Hour); at.Before(center.Add(24 * time.Hour)); at = at.Add(time.Minute) {
		local := at.In(zone)
		if local.Format("2006-01-02") != targetDate {
			continue
		}
		clock := local.Hour()*60 + local.Minute()
		if clock < hour*60+minute {
			continue
		}
		if clock < bestClock {
			best = at
			bestClock = clock
		}
		if clock == hour*60+minute {
			return at, nil
		}
	}
	if !best.IsZero() {
		return best, nil
	}
	return time.Time{}, fmt.Errorf("local date %s has no valid scheduled instant", targetDate)
}
func NextOccurrence(s model.Schedule, after time.Time) (time.Time, error) {
	if e := Validate(s); e != nil {
		return time.Time{}, e
	}
	zone, _ := time.LoadLocation(s.Timezone)
	anchor, _ := time.Parse("2006-01-02", s.AnchorDate)
	clock, _ := time.Parse("15:04", s.LocalTime)
	days := ordinal(after.In(zone)) - ordinal(anchor)
	cycle := int64(0)
	if days > 0 {
		cycle = days / int64(s.IntervalDays)
	}
	for attempt := 0; attempt < 4; attempt++ {
		date := anchor.AddDate(0, 0, int(cycle)*s.IntervalDays)
		candidate, e := localInstant(date, clock.Hour(), clock.Minute(), zone)
		if e == nil && candidate.After(after) {
			return candidate.UTC(), nil
		}
		cycle++
	}
	return time.Time{}, errors.New("unable to calculate next schedule occurrence")
}
