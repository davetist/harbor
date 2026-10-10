package store

import (
	"context"
	"encoding/json"
	"errors"
	"harbor/internal/model"
)

func (s *Store) CanRetryPrelaunch(ctx context.Context, j model.Job) bool {
	if j.Phase != "failed" {
		return false
	}
	switch j.Kind {
	case "boot_comet", "boot_keyfile", "import":
	default:
		return false
	}
	var n int
	if s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM processes WHERE ship_id=?", j.ShipID).Scan(&n) != nil || n != 0 {
		return false
	}
	if s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM jobs WHERE ship_id=? AND phase NOT IN ('completed','failed','skipped','interrupted')", j.ShipID).Scan(&n) != nil || n != 0 {
		return false
	}
	ship, err := s.GetShip(ctx, j.ShipID)
	return err == nil && !ship.RecoveryBlocked
}

// Only failures before any process launch may be retried. Reuse the durable
// registration while preserving failed attempts and their original diagnostics.
func (s *Store) RetryPrelaunch(ctx context.Context, id, upload model.ID) (model.Job, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Job{}, err
	}
	defer tx.Rollback()
	old, err := decodeRow[model.Job](tx.QueryRowContext(ctx, "SELECT payload FROM jobs WHERE id=?", id))
	if err != nil {
		return old, err
	}
	if old.Phase != "failed" {
		return old, errors.New("only failed pre-launch setup can be retried")
	}
	switch old.Kind {
	case "boot_comet", "boot_keyfile", "import":
	default:
		return old, errors.New("this operation cannot be retried")
	}
	var n int
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM processes WHERE ship_id=?", old.ShipID).Scan(&n); err != nil {
		return old, err
	}
	if n != 0 {
		return old, errors.New("a launch was recorded; inspect recovery instead of retrying")
	}
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM jobs WHERE ship_id=? AND phase NOT IN ('completed','failed','skipped','interrupted')", old.ShipID).Scan(&n); err != nil {
		return old, err
	}
	if n != 0 {
		return old, errors.New("setup is already queued or running")
	}
	ship, err := decodeRow[model.Ship](tx.QueryRowContext(ctx, "SELECT payload FROM ships WHERE id=?", old.ShipID))
	if err != nil {
		return old, err
	}
	if ship.RecoveryBlocked {
		return old, errors.New(ship.RecoveryBlockReason)
	}
	if old.Kind == "boot_keyfile" && upload == "" {
		return old, errors.New("select the keyfile again for this retry")
	}
	j := model.Job{ID: model.NewID(), ShipID: old.ShipID, Kind: old.Kind, Trigger: "manual", Phase: "queued", UploadID: upload}
	b, err := json.Marshal(j)
	if err != nil {
		return j, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO jobs(id,ship_id,kind,trigger,phase,payload,sequence) VALUES(?,?,?,?,?,?,COALESCE((SELECT MAX(sequence)+1 FROM jobs),1))`, j.ID, j.ShipID, j.Kind, j.Trigger, j.Phase, string(b)); err != nil {
		return j, err
	}
	return j, tx.Commit()
}
