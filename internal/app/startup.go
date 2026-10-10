package app

import (
	"context"
	"harbor/internal/jobs"
	"harbor/internal/model"
	"harbor/internal/store"
	"harbor/internal/supervisor"
)

func RecoverAndStart(ctx context.Context, st *store.Store, s *supervisor.Supervisor, e *jobs.Executor) error {
	e.SetReady(false)
	if err := s.Observe(ctx); err != nil {
		return err
	}
	if err := e.Reconcile(ctx); err != nil {
		return err
	}
	ships, err := st.ListShips(ctx)
	if err != nil {
		return err
	}
	for _, ship := range ships {
		if ship.DesiredRunning && !ship.RecoveryBlocked && ship.State != "externally_running" {
			pending, err := st.HasPendingJob(ctx, ship.ID)
			if err != nil {
				return err
			}
			if !pending {
				if err = st.CreateJob(ctx, model.Job{ID: model.NewID(), ShipID: ship.ID, Kind: "start", Trigger: "system", Phase: "queued"}); err != nil {
					return err
				}
			}
		}
	}
	e.SetReady(true)
	return nil
}
