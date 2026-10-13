package jobs

import (
	"context"
	"errors"
	"harbor/internal/model"
)

func (e *Executor) InterruptShip(ctx context.Context, id model.ID) error {
	// Match manual admission and shutdown: no new manual lifecycle request can
	// enter while the supervisor verifies readiness, pending work and ownership.
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.ready || e.supervisor == nil {
		return errors.New("Harbor is recovering or shutting down")
	}
	return e.supervisor.Interrupt(ctx, id)
}
