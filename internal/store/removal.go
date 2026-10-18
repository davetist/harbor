package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"harbor/internal/model"
	"harbor/internal/storage"
)

// RemoveShip unregisters an idle ship and its management records in one commit.
// It never opens, changes or deletes the pier, runtime, uploads or export files.
func (s *Store) RemoveShip(ctx context.Context, id model.ID) error {
	return s.removeShip(ctx, id, "")
}

// CompletePierDeletion releases the registration only after the guarded folder
// has been deleted. A failure retains the durable hold for an explicit retry.
func (s *Store) CompletePierDeletion(ctx context.Context, id model.ID, identity string) error {
	if identity == "" {
		return errors.New("missing pier deletion identity")
	}
	return s.removeShip(ctx, id, identity)
}

func removableShip(ctx context.Context, tx *sql.Tx, id model.ID, deleting bool) (model.Ship, error) {
	ship, err := decodeRow[model.Ship](tx.QueryRowContext(ctx, "SELECT payload FROM ships WHERE id=?", id))
	if err != nil {
		return ship, err
	}
	if ship.PierDeletionIdentity != "" && !deleting {
		return ship, errors.New("Pier deletion is pending. Confirm permanent deletion again to finish removal.")
	}
	if ship.RecoveryBlocked && !(deleting && ship.PierDeletionIdentity != "") {
		return ship, errors.New("Resolve this ship's recovery hold before removing it from Harbor.")
	}
	if ship.DesiredRunning || (ship.State != "stopped" && ship.State != "failed") {
		return ship, errors.New("Stop this ship before removing it from Harbor.")
	}
	var busy bool
	if err = tx.QueryRowContext(ctx, `SELECT
		EXISTS(SELECT 1 FROM jobs WHERE ship_id=? AND phase NOT IN ('completed','failed','skipped','interrupted'))
		OR EXISTS(SELECT 1 FROM processes WHERE ship_id=? AND phase!='exited')
		OR EXISTS(SELECT 1 FROM pending_registrations WHERE id=?)`, id, id, id).Scan(&busy); err != nil {
		return ship, err
	}
	if busy {
		return ship, errors.New("This ship has active or queued work. Wait for it to finish before removing it.")
	}
	return ship, nil
}

func (s *Store) removeShip(ctx context.Context, id model.ID, identity string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	ship, err := removableShip(ctx, tx, id, identity != "")
	if err != nil {
		return err
	}
	if identity != "" && ship.PierDeletionIdentity != identity {
		return errors.New("pier deletion identity changed")
	}
	// Jobs and occurrences reference each other with deferred constraints.
	// Delete their dependents before schedules and the registration itself.
	for _, query := range []string{
		"DELETE FROM job_log_chunks WHERE job_id IN (SELECT id FROM jobs WHERE ship_id=?)",
		"DELETE FROM occurrences WHERE schedule_id IN (SELECT id FROM schedules WHERE ship_id=?)",
		"DELETE FROM jobs WHERE ship_id=?",
		"DELETE FROM processes WHERE ship_id=?",
		"DELETE FROM schedules WHERE ship_id=?",
		"DELETE FROM backups WHERE ship_id=?",
		"DELETE FROM ships WHERE id=?",
	} {
		if _, err = tx.ExecContext(ctx, query, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// BeginPierDeletion commits stopped intent and a hold before any files change.
// Pausing schedules in the same transaction prevents new scheduled admission.
func (s *Store) BeginPierDeletion(ctx context.Context, id model.ID, path, identity string) (model.Ship, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Ship{}, err
	}
	defer tx.Rollback()
	ship, err := removableShip(ctx, tx, id, true)
	if err != nil {
		return ship, err
	}
	if ship.PierPath != path || identity == "" || (ship.PierDeletionIdentity != "" && ship.PierDeletionIdentity != identity) {
		return ship, errors.New("pier deletion confirmation no longer matches the registered folder")
	}
	rows, err := tx.QueryContext(ctx, `SELECT pier_path,1 FROM ships WHERE id!=?
		UNION ALL SELECT json_extract(payload,'$.runtimePath'),0 FROM ships
		UNION ALL SELECT archive_path,0 FROM backups
		UNION ALL SELECT json_extract(payload,'$.pierPath'),1 FROM pending_registrations`, id)
	if err != nil {
		return ship, err
	}
	for rows.Next() {
		var protected sql.NullString
		var bothDirections bool
		if err = rows.Scan(&protected, &bothDirections); err != nil {
			break
		}
		if !protected.Valid || protected.String == "" {
			continue
		}
		var overlap bool
		overlap, err = storage.ContainsPath(path, protected.String)
		if err == nil && !overlap && bothDirections {
			overlap, err = storage.ContainsPath(protected.String, path)
		}
		if err != nil {
			break
		}
		if overlap {
			err = errors.New("pier folder overlaps another registered pier, runtime, pending import or exported archive; move it before deleting")
			break
		}
	}
	rowErr := rows.Err()
	rows.Close()
	if err != nil || rowErr != nil {
		return ship, errors.Join(err, rowErr)
	}
	ship.PierDeletionIdentity = identity
	ship.RecoveryBlocked = true
	ship.RecoveryBlockReason = "Pier deletion is pending. Confirm permanent deletion again to finish removal. This ship cannot be started."
	ship.DesiredRunning = false
	ship.State = "failed"
	b, err := json.Marshal(ship)
	if err != nil {
		return ship, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE ships SET payload=? WHERE id=?", string(b), id); err != nil {
		return ship, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE schedules SET enabled=0,payload=json_set(payload,'$.enabled',json('false')) WHERE ship_id=?", id); err != nil {
		return ship, err
	}
	return ship, tx.Commit()
}

func rejectDeletingOverlap(ctx context.Context, tx *sql.Tx, path string) error {
	rows, err := tx.QueryContext(ctx, "SELECT pier_path FROM ships WHERE COALESCE(json_extract(payload,'$.pierDeletionIdentity'),'')!=''")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var deleting string
		if err = rows.Scan(&deleting); err != nil {
			return err
		}
		inside, err := storage.ContainsPath(deleting, path)
		if err != nil {
			return err
		}
		if !inside {
			inside, err = storage.ContainsPath(path, deleting)
		}
		if err != nil {
			return err
		}
		if inside {
			return errors.New("pier overlaps a folder pending permanent deletion")
		}
	}
	return rows.Err()
}
