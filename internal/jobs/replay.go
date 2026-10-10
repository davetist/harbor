package jobs

import (
	"context"
	"harbor/internal/model"
	vr "harbor/internal/runtime"
	"path/filepath"
)

func (e *Executor) EnableReplay() {
	e.Hooks["replay"] = func(ctx context.Context, j *model.Job) error {
		b, err := e.VerifiedBackup(ctx, j.ShipID)
		if err != nil {
			return err
		}
		ship, err := e.store.GetShip(ctx, j.ShipID)
		if err != nil {
			return err
		}
		args, err := vr.BuildArgs(ship, "replay")
		if err != nil {
			return err
		}
		return e.maintainWith(context.WithoutCancel(ctx), j, ship, args, func(ctx context.Context, ship model.Ship) error {
			return e.takeFullBackup(ctx, j, ship, filepath.Dir(b.ArchivePath))
		}, nil)
	}
}
