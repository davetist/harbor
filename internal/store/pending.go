package store

import (
	"context"
	"encoding/json"
	"errors"
	"harbor/internal/model"
)

func (s *Store) QueuePendingImport(ctx context.Context, ship model.Ship, j model.Job) error {
	if j.Kind != "import_archive" || j.ShipID != ship.ID || j.Trigger != "manual" || j.Phase != "queued" {
		return errors.New("invalid pending import")
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
	if _, e = tx.ExecContext(ctx, "INSERT INTO pending_registrations(id,job_id,payload) VALUES(?,?,?)", ship.ID, j.ID, string(b)); e != nil {
		return e
	}
	b, e = json.Marshal(j)
	if e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO jobs(id,ship_id,kind,trigger,phase,payload,sequence) VALUES(?,?,?,?,?,?,COALESCE((SELECT MAX(sequence)+1 FROM jobs),1))`, j.ID, j.ShipID, j.Kind, j.Trigger, j.Phase, string(b)); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) GetPendingImport(ctx context.Context, id model.ID) (model.Ship, error) {
	return decodeRow[model.Ship](s.db.QueryRowContext(ctx, "SELECT payload FROM pending_registrations WHERE id=?", id))
}
func (s *Store) RemovePendingImport(ctx context.Context, id model.ID) error {
	_, e := s.db.ExecContext(ctx, "DELETE FROM pending_registrations WHERE id=?", id)
	return e
}
