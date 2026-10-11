package jobs

import (
	"context"
	"errors"
	"harbor/internal/model"
)

func (e *Executor) RemoveShip(ctx context.Context, id model.ID) error {
	// Serialize removal with manual submissions; the store transaction checks
	// scheduled work and live processes before releasing the registration.
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.ready {
		return errors.New("Harbor is recovering or shutting down")
	}
	if err := e.store.RemoveShip(ctx, id); err != nil {
		return err
	}
	e.emit(model.Event{Type: "ship", ShipID: id, Data: map[string]bool{"removed": true}})
	return nil
}
