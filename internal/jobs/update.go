package jobs

import (
	"context"
	"harbor/internal/model"
	vr "harbor/internal/runtime"
)

func (e *Executor) EnableRuntimeUpdates(dir string) {
	e.Hooks["update"] = func(ctx context.Context, j *model.Job) error {
		ship, err := e.store.GetShip(ctx, j.ShipID)
		if err != nil {
			return err
		}
		platform, err := vr.NativePlatform()
		if err != nil {
			return err
		}
		selected, err := (vr.Resolver{Dir: dir}).Resolve(ctx, platform, vr.StableVersion)
		if err != nil {
			return err
		}
		current, err := vr.FileDigest(ship.RuntimePath)
		if err != nil {
			return err
		}
		if current == selected.Digest {
			if err = e.store.SetRuntimeVersion(ctx, ship.ID, "Vere "+vr.StableVersion); err != nil {
				return err
			}
			j.Result = "Already using the verified stable Vere " + vr.StableVersion + " runtime."
			return nil
		}
		args, err := vr.BuildArgs(ship, "update")
		if err != nil {
			return err
		}
		// Vere prep saves an up-to-date checkpoint for the version transition.
		// Keep the old runtime and native checkpoints; a full-pier migration
		// export is neither a prerequisite nor part of this update operation.
		return e.maintainWith(context.WithoutCancel(ctx), j, ship, args, nil, func(ctx context.Context, ship *model.Ship) error {
			ship.RuntimePath = selected.Path
			ship.RuntimeVersion = "Vere " + vr.StableVersion
			return nil
		})
	}
}
