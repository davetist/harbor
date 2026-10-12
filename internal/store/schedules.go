package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"harbor/internal/model"
	"sort"
	"time"
)

func (s *Store) ListSchedules(ctx context.Context, ship model.ID) ([]model.Schedule, error) {
	rows, e := s.db.QueryContext(ctx, "SELECT payload FROM schedules WHERE ship_id=? ORDER BY CASE json_extract(payload,'$.kind') WHEN 'pack' THEN 0 WHEN 'meld' THEN 1 WHEN 'roll' THEN 2 ELSE 3 END", ship)
	if e != nil {
		return nil, e
	}
	return decodeRows[model.Schedule](rows)
}
func validateSchedulePair(a, b model.Schedule) error {
	if a.Kind != "chop" || b.Kind != "roll" || a.ShipID != b.ShipID || a.AnchorDate != b.AnchorDate || a.IntervalDays != b.IntervalDays || a.Timezone != b.Timezone || a.LocalTime <= b.LocalTime {
		return errors.New("paired Chop requires an earlier Roll on the same ship, calendar cadence and timezone")
	}
	return nil
}
func (s *Store) PutSchedule(ctx context.Context, v model.Schedule) error {
	return s.putSchedule(ctx, v, nil)
}

// EditSchedule changes the shared cadence of a paired Roll/Chop together.
// Each operation retains its own clock time and enabled state.
func (s *Store) EditSchedule(ctx context.Context, v model.Schedule, next func(model.Schedule) (time.Time, error)) error {
	return s.putSchedule(ctx, v, next)
}
func (s *Store) putSchedule(ctx context.Context, v model.Schedule, next func(model.Schedule) (time.Time, error)) error {
	if v.ID == "" || v.IntervalDays < 1 || v.IntervalDays > 365 {
		return errors.New("invalid schedule interval or ID")
	}
	if _, e := time.Parse("2006-01-02", v.AnchorDate); e != nil {
		return e
	}
	if _, e := time.Parse("15:04", v.LocalTime); e != nil {
		return e
	}
	if _, e := time.LoadLocation(v.Timezone); e != nil {
		return e
	}
	switch v.Kind {
	case "pack", "meld", "roll", "chop":
	default:
		return errors.New("operation cannot be scheduled")
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	syncCadence := func(other *model.Schedule) error {
		if next == nil {
			return nil
		}
		other.AnchorDate, other.IntervalDays, other.Timezone = v.AnchorDate, v.IntervalDays, v.Timezone
		var err error
		other.NextDue, err = next(*other)
		return err
	}
	if v.AfterScheduleID != "" {
		other, e := decodeRow[model.Schedule](tx.QueryRowContext(ctx, "SELECT payload FROM schedules WHERE id=?", v.AfterScheduleID))
		if e != nil {
			return e
		}
		if e = syncCadence(&other); e != nil {
			return e
		}
		if e = validateSchedulePair(v, other); e != nil {
			return e
		}
		if next != nil {
			if e = saveSchedule(ctx, tx, other); e != nil {
				return e
			}
		}
	}
	rows, e := tx.QueryContext(ctx, "SELECT payload FROM schedules WHERE json_extract(payload,'$.afterScheduleId')=?", v.ID)
	if e != nil {
		return e
	}
	dependents, e := decodeRows[model.Schedule](rows)
	if e != nil {
		return e
	}
	for _, other := range dependents {
		if e = syncCadence(&other); e != nil {
			return e
		}
		if e = validateSchedulePair(other, v); e != nil {
			return e
		}
		if next != nil {
			if e = saveSchedule(ctx, tx, other); e != nil {
				return e
			}
		}
	}
	if existing, e := decodeRow[model.Schedule](tx.QueryRowContext(ctx, "SELECT payload FROM schedules WHERE id=?", v.ID)); e == nil {
		if existing.ShipID != v.ShipID || existing.Kind != v.Kind {
			return errors.New("schedule identity cannot change")
		}
		v.LastOccurrence = existing.LastOccurrence
		v.LastJobID = existing.LastJobID
		v.LastResult = existing.LastResult
	} else if !errors.Is(e, ErrNotFound) {
		return e
	}
	if e = saveSchedule(ctx, tx, v); e != nil {
		return e
	}
	return tx.Commit()
}
func saveSchedule(ctx context.Context, tx *sql.Tx, v model.Schedule) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, "INSERT INTO schedules(id,ship_id,next_due,enabled,payload) VALUES(?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET next_due=excluded.next_due,enabled=excluded.enabled,payload=excluded.payload", v.ID, v.ShipID, v.NextDue.UTC().Format(time.RFC3339Nano), v.Enabled, string(b))
	return e
}

func maintenanceOrder(kind model.Operation) int {
	switch kind {
	case "pack":
		return 0
	case "meld":
		return 1
	case "roll":
		return 2
	case "chop":
		return 3
	default:
		return 4
	}
}

// Saved clock times are eligibility times. Earlier stages scheduled on the
// occurrence's maintenance day, or currently due after downtime, are claimed
// first. Calendar dates avoid normalizing a nonexistent local midnight.
func waitsForEarlierSchedule(schedules []model.Schedule, current model.Schedule, date string, now time.Time) (bool, error) {
	zone, err := time.LoadLocation(current.Timezone)
	if err != nil {
		return false, err
	}
	_, err = time.Parse("2006-01-02", date)
	if err != nil {
		return false, err
	}
	for _, earlier := range schedules {
		if earlier.ShipID == current.ShipID && earlier.Enabled && !earlier.NextDue.IsZero() && maintenanceOrder(earlier.Kind) < maintenanceOrder(current.Kind) {
			if earlier.NextDue.In(zone).Format("2006-01-02") <= date || !earlier.NextDue.After(now) {
				return true, nil
			}
		}
	}
	return false, nil
}

func (s *Store) ClaimDueAndEnqueue(ctx context.Context, now time.Time, next func(model.Schedule, time.Time) (time.Time, error)) ([]model.Job, error) {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	rows, e := tx.QueryContext(ctx, "SELECT payload FROM schedules WHERE enabled=1")
	if e != nil {
		return nil, e
	}
	schedules, e := decodeRows[model.Schedule](rows)
	if e != nil {
		return nil, e
	}
	sort.SliceStable(schedules, func(i, j int) bool { return maintenanceOrder(schedules[i].Kind) < maintenanceOrder(schedules[j].Kind) })
	out := make([]model.Job, 0)
	for i := range schedules {
		sc := schedules[i]
		if sc.NextDue.IsZero() || sc.NextDue.After(now) {
			continue
		}
		due := sc.NextDue
		var nextDue time.Time
		for count := 0; ; count++ {
			if count > 36600 {
				return nil, errors.New("schedule catch-up exceeds 100 years")
			}
			nextDue, e = next(sc, due)
			if e != nil {
				return nil, e
			}
			if !nextDue.After(due) {
				return nil, errors.New("schedule calculation did not advance")
			}
			if nextDue.After(now) {
				break
			}
			due = nextDue
		}
		var exists int
		if e = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM occurrences WHERE schedule_id=? AND scheduled_for=?", sc.ID, due.UTC().Format(time.RFC3339Nano)).Scan(&exists); e != nil {
			return nil, e
		}
		if exists != 0 {
			if s.testClaimHook != nil {
				if e = s.testClaimHook("occurrence"); e != nil {
					return nil, e
				}
			}
			sc.NextDue = nextDue
			if e = saveSchedule(ctx, tx, sc); e != nil {
				return nil, e
			}
			schedules[i] = sc
			continue
		}
		zone, e := time.LoadLocation(sc.Timezone)
		if e != nil {
			return nil, e
		}
		date := due.In(zone).Format("2006-01-02")
		waiting, e := waitsForEarlierSchedule(schedules, sc, date, now)
		if e != nil {
			return nil, e
		}
		if waiting {
			continue // Retain NextDue until the earlier stage becomes eligible.
		}
		occurrence := model.NewID()
		j := model.Job{ID: model.NewID(), ShipID: sc.ShipID, Kind: sc.Kind, Trigger: "scheduled", ScheduleID: sc.ID, OccurrenceID: occurrence, Phase: "queued"}
		if sc.AfterScheduleID != "" {
			paired, e := decodeRow[model.Schedule](tx.QueryRowContext(ctx, "SELECT payload FROM schedules WHERE id=?", sc.AfterScheduleID))
			if e != nil {
				return nil, e
			}
			if e = validateSchedulePair(sc, paired); e != nil {
				return nil, e
			}
			var dep string
			e = tx.QueryRowContext(ctx, "SELECT job_id FROM occurrences WHERE schedule_id=? AND local_date=? ORDER BY scheduled_for DESC LIMIT 1", sc.AfterScheduleID, date).Scan(&dep)
			if errors.Is(e, sql.ErrNoRows) {
				j.Phase = "skipped"
				j.Error = "Paired Roll occurrence was paused or coalesced away for this cycle."
				j.FinishedAt = now
			} else if e != nil {
				return nil, e
			} else {
				j.DependsOnJobID = model.ID(dep)
			}
		}
		b, e := json.Marshal(j)
		if e != nil {
			return nil, e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO jobs(id,ship_id,kind,trigger,phase,schedule_id,occurrence_id,payload,sequence) VALUES(?,?,?,?,?,?,?,?,COALESCE((SELECT MAX(sequence)+1 FROM jobs),1))`, j.ID, j.ShipID, j.Kind, j.Trigger, j.Phase, j.ScheduleID, j.OccurrenceID, string(b)); e != nil {
			return nil, e
		}
		if s.testClaimHook != nil {
			if e = s.testClaimHook("job"); e != nil {
				return nil, e
			}
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO occurrences(id,schedule_id,scheduled_for,local_date,job_id) VALUES(?,?,?,?,?)", occurrence, sc.ID, due.UTC().Format(time.RFC3339Nano), date, j.ID); e != nil {
			return nil, e
		}
		if s.testClaimHook != nil {
			if e = s.testClaimHook("occurrence"); e != nil {
				return nil, e
			}
		}
		sc.NextDue = nextDue
		sc.LastOccurrence = occurrence
		sc.LastJobID = j.ID
		if e = saveSchedule(ctx, tx, sc); e != nil {
			return nil, e
		}
		schedules[i] = sc
		if s.testClaimHook != nil {
			if e = s.testClaimHook("advance"); e != nil {
				return nil, e
			}
		}
		out = append(out, j)
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	return out, nil
}
