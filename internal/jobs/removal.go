package jobs

import (
	"context"
	"errors"
	"harbor/internal/model"
	"harbor/internal/pier"
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

func (e *Executor) DeleteShip(ctx context.Context, id model.ID, confirmedPath string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.ready {
		return errors.New("Harbor is recovering or shutting down")
	}
	ship, err := e.store.GetShip(ctx, id)
	if err != nil {
		return err
	}
	if confirmedPath != ship.PierPath {
		return errors.New("confirm the exact registered pier path before deleting")
	}
	release, err := pier.Guard(ship.PierPath)
	if err != nil {
		return errors.New("pier is in use; files were not deleted")
	}
	defer release()
	deletion, err := pier.PrepareDeletion(ship.PierPath, ship.PierDeletionIdentity)
	if err != nil {
		return err
	}
	defer deletion.Close()
	ship, err = e.store.BeginPierDeletion(ctx, id, confirmedPath, deletion.Identity())
	if err != nil {
		return err
	}
	e.emit(model.Event{Type: "ship", ShipID: id, Data: ship})
	// Once deletion is authorized and durable, disconnecting the browser cannot
	// turn a partially removed pier back into a startable ship. Shutdown waits on mu.
	if err = deletion.Remove(); err != nil {
		return errors.New("Pier deletion did not finish. The ship remains held; check storage access and confirm permanent deletion again to retry.")
	}
	if err = e.store.CompletePierDeletion(context.WithoutCancel(ctx), id, ship.PierDeletionIdentity); err != nil {
		return err
	}
	e.emit(model.Event{Type: "ship", ShipID: id, Data: map[string]bool{"removed": true}})
	return nil
}
