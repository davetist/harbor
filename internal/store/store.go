package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"harbor/internal/model"
	_ "modernc.org/sqlite"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	db               *sql.DB
	testClaimHook    func(string) error
	testRecoveryHook func() error
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	f.Close()
	if err = os.Chmod(path, 0600); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", (&url.URL{Scheme: "file", Path: path}).String()+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	var version int
	if err = db.QueryRow("SELECT version FROM schema_version").Scan(&version); err != nil || version != 1 {
		db.Close()
		return nil, fmt.Errorf("unsupported database schema: %d", version)
	}
	return &Store{db: db}, nil
}
func (s *Store) Close() error { return s.db.Close() }
func (s *Store) RecordMeasurement(ctx context.Context, id model.ID, size int64, at time.Time) error {
	_, err := s.db.ExecContext(ctx, "UPDATE ships SET payload=json_set(payload,'$.pierBytes',?,'$.measuredAt',?) WHERE id=?", size, at.UTC().Format(time.RFC3339Nano), id)
	return err
}
func (s *Store) SetRuntimeVersion(ctx context.Context, id model.ID, version string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE ships SET payload=json_set(payload,'$.runtimeVersion',?) WHERE id=?", version, id)
	return err
}
func (s *Store) PutShip(ctx context.Context, v model.Ship) error {
	if v.ID == "" || v.PierPath == "" {
		return errors.New("ship ID and pier path required")
	}
	v.PierPath = filepath.Clean(v.PierPath)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	existing, lookup := decodeRow[model.Ship](tx.QueryRowContext(ctx, "SELECT payload FROM ships WHERE id=?", v.ID))
	if lookup == nil {
		if existing.PierPath != v.PierPath {
			return errors.New("registered pier path cannot change")
		}
	} else if !errors.Is(lookup, ErrNotFound) {
		return lookup
	} else if real, err := filepath.EvalSymlinks(v.PierPath); err == nil {
		v.PierPath = real
	}
	if errors.Is(lookup, ErrNotFound) {
		if err = rejectDeletingOverlap(ctx, tx, v.PierPath); err != nil {
			return err
		}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO ships(id,pier_path,payload) VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET payload=excluded.payload WHERE ships.pier_path=excluded.pier_path", v.ID, v.PierPath, string(b))
	if err != nil {
		return err
	}
	return tx.Commit()
}

// An interrupted outcome and its hold must survive the same commit, including
// a second controller crash during startup reconciliation.
func (s *Store) InterruptAndHold(ctx context.Context, j model.Job) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	b, err := json.Marshal(j)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE jobs SET phase=?,payload=? WHERE id=?", j.Phase, string(b), j.ID); err != nil {
		return err
	}
	if s.testRecoveryHook != nil {
		if err = s.testRecoveryHook(); err != nil {
			return err
		}
	}
	ship, err := decodeRow[model.Ship](tx.QueryRowContext(ctx, "SELECT payload FROM ships WHERE id=?", j.ShipID))
	if errors.Is(err, ErrNotFound) && j.Kind == "import_archive" {
		return tx.Commit()
	}
	if err != nil {
		return err
	}
	ship.RecoveryBlocked = true
	ship.RecoveryBlockReason = j.Error
	ship.State = "failed"
	b, err = json.Marshal(ship)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE ships SET payload=? WHERE id=?", string(b), ship.ID); err != nil {
		return err
	}
	return tx.Commit()
}
func decodeRow[T any](row *sql.Row) (T, error) {
	var v T
	var b string
	err := row.Scan(&b)
	if errors.Is(err, sql.ErrNoRows) {
		return v, ErrNotFound
	}
	if err != nil {
		return v, err
	}
	return v, json.Unmarshal([]byte(b), &v)
}
func decodeRows[T any](rows *sql.Rows) ([]T, error) {
	defer rows.Close()
	out := make([]T, 0)
	for rows.Next() {
		var b string
		var v T
		if err := rows.Scan(&b); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(b), &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) GetShip(ctx context.Context, id model.ID) (model.Ship, error) {
	return decodeRow[model.Ship](s.db.QueryRowContext(ctx, "SELECT payload FROM ships WHERE id=?", id))
}
func (s *Store) ListShips(ctx context.Context) ([]model.Ship, error) {
	r, e := s.db.QueryContext(ctx, "SELECT payload FROM ships ORDER BY rowid")
	if e != nil {
		return nil, e
	}
	return decodeRows[model.Ship](r)
}
func (s *Store) CreateJob(ctx context.Context, j model.Job) error {
	if j.Trigger != "manual" && j.Trigger != "system" {
		return errors.New("scheduled jobs require occurrence transaction")
	}
	if j.ScheduleID != "" || j.OccurrenceID != "" {
		return errors.New("occurrence metadata is reserved for scheduler")
	}
	if j.Phase != "queued" || j.ID == "" {
		return errors.New("new job must be queued with an ID")
	}
	b, e := json.Marshal(j)
	if e != nil {
		return e
	}
	_, e = s.db.ExecContext(ctx, `INSERT INTO jobs(id,ship_id,kind,trigger,phase,payload,sequence) VALUES(?,?,?,?,?,?,COALESCE((SELECT MAX(sequence)+1 FROM jobs),1))`, j.ID, j.ShipID, j.Kind, j.Trigger, j.Phase, string(b))
	return e
}
func (s *Store) GetJob(ctx context.Context, id model.ID) (model.Job, error) {
	return decodeRow[model.Job](s.db.QueryRowContext(ctx, "SELECT payload FROM jobs WHERE id=?", id))
}
func (s *Store) ClaimJob(ctx context.Context) (model.Job, error) {
	return s.claimJob(ctx, time.Now())
}

func (s *Store) claimJob(ctx context.Context, now time.Time) (model.Job, error) {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return model.Job{}, e
	}
	defer tx.Rollback()
	rows, e := tx.QueryContext(ctx, `SELECT j.payload FROM jobs j
		LEFT JOIN jobs dep ON dep.id=json_extract(j.payload,'$.dependsOnJobId')
		WHERE j.phase='queued' AND (dep.id IS NULL OR dep.phase IN ('completed','failed','skipped','interrupted'))
		AND (j.trigger!='scheduled' OR NOT EXISTS (
			SELECT 1 FROM jobs earlier WHERE earlier.ship_id=j.ship_id AND earlier.trigger='scheduled'
			AND earlier.phase NOT IN ('completed','failed','skipped','interrupted')
			AND CASE earlier.kind WHEN 'pack' THEN 0 WHEN 'meld' THEN 1 WHEN 'roll' THEN 2 WHEN 'chop' THEN 3 ELSE 4 END
			  < CASE j.kind WHEN 'pack' THEN 0 WHEN 'meld' THEN 1 WHEN 'roll' THEN 2 WHEN 'chop' THEN 3 ELSE 4 END
		)) ORDER BY j.sequence`)
	if e != nil {
		return model.Job{}, e
	}
	candidates, e := decodeRows[model.Job](rows)
	if e != nil {
		return model.Job{}, e
	}
	var j model.Job
	for _, candidate := range candidates {
		if candidate.Trigger == "scheduled" {
			current, err := decodeRow[model.Schedule](tx.QueryRowContext(ctx, "SELECT payload FROM schedules WHERE id=?", candidate.ScheduleID))
			if err != nil {
				return model.Job{}, err
			}
			var date string
			if err = tx.QueryRowContext(ctx, "SELECT local_date FROM occurrences WHERE id=?", candidate.OccurrenceID).Scan(&date); err != nil {
				return model.Job{}, err
			}
			rows, err := tx.QueryContext(ctx, "SELECT payload FROM schedules WHERE ship_id=? AND enabled=1", candidate.ShipID)
			if err != nil {
				return model.Job{}, err
			}
			schedules, err := decodeRows[model.Schedule](rows)
			if err != nil {
				return model.Job{}, err
			}
			waiting, err := waitsForEarlierSchedule(schedules, current, date, now)
			if err != nil {
				return model.Job{}, err
			}
			if waiting {
				continue
			}
		}
		j = candidate
		break
	}
	if j.ID == "" {
		return j, ErrNotFound
	}
	j.Phase = "preflight"
	b, e := json.Marshal(j)
	if e != nil {
		return j, e
	}
	r, e := tx.ExecContext(ctx, "UPDATE jobs SET phase=?,payload=? WHERE id=? AND phase='queued'", j.Phase, string(b), j.ID)
	if e != nil {
		return j, e
	}
	n, _ := r.RowsAffected()
	if n != 1 {
		return j, ErrNotFound
	}
	return j, tx.Commit()
}
func (s *Store) UpdateJob(ctx context.Context, j model.Job) error {
	b, e := json.Marshal(j)
	if e != nil {
		return e
	}
	r, e := s.db.ExecContext(ctx, "UPDATE jobs SET phase=?,payload=? WHERE id=?", j.Phase, string(b), j.ID)
	if e != nil {
		return e
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
func (s *Store) ListUnfinishedJobs(ctx context.Context) ([]model.Job, error) {
	r, e := s.db.QueryContext(ctx, "SELECT payload FROM jobs WHERE phase NOT IN ('queued','completed','failed','skipped','interrupted') ORDER BY sequence")
	if e != nil {
		return nil, e
	}
	return decodeRows[model.Job](r)
}
func (s *Store) PutProcess(ctx context.Context, p model.ProcessRecord) error {
	b, e := json.Marshal(p)
	if e != nil {
		return e
	}
	_, e = s.db.ExecContext(ctx, "INSERT INTO processes(id,ship_id,phase,payload) VALUES(?,?,?,?) ON CONFLICT(id) DO UPDATE SET phase=excluded.phase,payload=excluded.payload", p.LaunchID, p.ShipID, p.Phase, string(b))
	return e
}
func (s *Store) ListOpenProcesses(ctx context.Context) ([]model.ProcessRecord, error) {
	r, e := s.db.QueryContext(ctx, "SELECT payload FROM processes WHERE phase!='exited'")
	if e != nil {
		return nil, e
	}
	return decodeRows[model.ProcessRecord](r)
}

func (s *Store) HasPendingJob(ctx context.Context, id model.ID) (bool, error) {
	var n int
	e := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM jobs WHERE ship_id=? AND phase NOT IN ('completed','failed','skipped','interrupted')", id).Scan(&n)
	return n > 0, e
}
func (s *Store) JobsForShip(ctx context.Context, id model.ID) ([]model.Job, error) {
	r, e := s.db.QueryContext(ctx, "SELECT payload FROM jobs WHERE ship_id=? ORDER BY sequence DESC LIMIT 100", id)
	if e != nil {
		return nil, e
	}
	return decodeRows[model.Job](r)
}

func (s *Store) RegisterQueued(ctx context.Context, ship model.Ship, j model.Job, schedules ...model.Schedule) error {
	if j.ShipID != ship.ID || j.Trigger != "manual" || j.Phase != "queued" {
		return errors.New("invalid registration job")
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if e = rejectDeletingOverlap(ctx, tx, ship.PierPath); e != nil {
		return e
	}
	b, e := json.Marshal(ship)
	if e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "INSERT INTO ships(id,pier_path,payload) VALUES(?,?,?)", ship.ID, ship.PierPath, string(b)); e != nil {
		return e
	}
	b, e = json.Marshal(j)
	if e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO jobs(id,ship_id,kind,trigger,phase,payload,sequence) VALUES(?,?,?,?,?,?,COALESCE((SELECT MAX(sequence)+1 FROM jobs),1))`, j.ID, j.ShipID, j.Kind, j.Trigger, j.Phase, string(b)); e != nil {
		return e
	}
	for _, sc := range schedules {
		if sc.ShipID != ship.ID {
			return errors.New("schedule belongs to another ship")
		}
		if e = saveSchedule(ctx, tx, sc); e != nil {
			return e
		}
	}
	return tx.Commit()
}

func (s *Store) ProcessesForJob(ctx context.Context, id model.ID) ([]model.ProcessRecord, error) {
	rows, e := s.db.QueryContext(ctx, "SELECT payload FROM processes WHERE json_extract(payload,'$.jobId')=?", id)
	if e != nil {
		return nil, e
	}
	return decodeRows[model.ProcessRecord](rows)
}
