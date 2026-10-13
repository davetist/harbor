package store

import (
	"context"
	"errors"
	"harbor/internal/model"
)

// WithIdleRunningShip reserves the store's sole connection through the final
// guarded action. Scheduled admission and state updates cannot pass the checks
// before fn returns. The callback must not call back into Store.
func (s *Store) WithIdleRunningShip(ctx context.Context, id model.ID, fn func(model.Ship) error) error {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	ship, err := decodeRow[model.Ship](conn.QueryRowContext(ctx, "SELECT payload FROM ships WHERE id=?", id))
	if err != nil {
		return err
	}
	if ship.RecoveryBlocked {
		return errors.New("Resolve this ship's recovery hold before interrupting.")
	}
	if ship.State != "running" {
		return errors.New("Only a running ship can be interrupted.")
	}
	var pending bool
	if err = conn.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM jobs WHERE ship_id=? AND phase NOT IN ('completed','failed','skipped','interrupted'))", id).Scan(&pending); err != nil {
		return err
	}
	if pending {
		return errors.New("Wait for this ship's active or queued operation to finish.")
	}
	return fn(ship)
}
