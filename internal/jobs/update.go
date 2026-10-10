package jobs

import (
	"context"
	"harbor/internal/model"
	vr "harbor/internal/runtime"
	"path/filepath"
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
		b, err := e.VerifiedBackup(ctx, ship.ID)
		if err != nil {
			return err
		}
		args, err := vr.BuildArgs(ship, "update")
		if err != nil {
			return err
		}
		return e.maintainWith(context.WithoutCancel(ctx), j, ship, args, func(ctx context.Context, ship model.Ship) error {
			return e.takeFullBackup(ctx, j, ship, filepath.Dir(b.ArchivePath))
		}, func(ctx context.Context, ship *model.Ship) error {
			ship.RuntimePath = selected.Path
			ship.RuntimeVersion = "Vere " + vr.StableVersion
			return nil
		})
	}
}
