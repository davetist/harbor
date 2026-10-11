package store

import (
	"context"
	"errors"
	"harbor/internal/model"
)

// RemoveShip unregisters an idle ship and its management records in one commit.
// It never opens, changes or deletes the pier, runtime, uploads or export files.
func (s *Store) RemoveShip(ctx context.Context, id model.ID) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	ship, err := decodeRow[model.Ship](tx.QueryRowContext(ctx, "SELECT payload FROM ships WHERE id=?", id))
	if err != nil {
		return err
	}
	if ship.RecoveryBlocked {
		return errors.New("Resolve this ship's recovery hold before removing it from Harbor.")
	}
	if ship.DesiredRunning || (ship.State != "stopped" && ship.State != "failed") {
		return errors.New("Stop this ship before removing it from Harbor.")
	}
	var busy bool
	if err = tx.QueryRowContext(ctx, `SELECT
		EXISTS(SELECT 1 FROM jobs WHERE ship_id=? AND phase NOT IN ('completed','failed','skipped','interrupted'))
		OR EXISTS(SELECT 1 FROM processes WHERE ship_id=? AND phase!='exited')
		OR EXISTS(SELECT 1 FROM pending_registrations WHERE id=?)`, id, id, id).Scan(&busy); err != nil {
		return err
	}
	if busy {
		return errors.New("This ship has active or queued work. Wait for it to finish before removing it.")
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
