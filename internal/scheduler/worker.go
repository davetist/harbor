package scheduler

import (
	"context"
	"harbor/internal/store"
	"time"
)

type Worker struct{ Store *store.Store }

func (w Worker) Tick(ctx context.Context, now time.Time) error {
	_, e := w.Store.ClaimDueAndEnqueue(ctx, now, NextOccurrence)
	return e
}
func (w Worker) Run(ctx context.Context) error {
	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case now := <-tick.C:
			if e := w.Tick(ctx, now); e != nil {
				return e
			}
		}
	}
}
