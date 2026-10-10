package jobs

import (
	"context"
	"errors"
	"harbor/internal/backup"
	"harbor/internal/model"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func (e *Executor) EnableBackups(dir string) {
	e.Hooks["backup"] = func(ctx context.Context, j *model.Job) error {
		ship, err := e.store.GetShip(ctx, j.ShipID)
		if err != nil {
			return err
		}
		// An exported pier can move to another host only while the source stays
		// stopped. Persist that intent before stopping or writing any archive so
		// startup recovery and scheduled maintenance cannot resume the source.
		ship.DesiredRunning = false
		if err = e.store.PutShip(context.WithoutCancel(ctx), ship); err != nil {
			return err
		}
		if j.PriorRunning {
			if err = e.phase(ctx, j, "stopping"); err != nil {
				return err
			}
			if err = e.supervisor.Stop(ctx, ship.ID); err != nil {
				return err
			}
		}
		if err = e.takeFullBackup(context.WithoutCancel(ctx), j, ship, dir); err != nil {
			return err
		}
		j.Result = "Pier exported. Source ship remains stopped. Keep it stopped when moving to another device."
		return nil
	}
}
func (e *Executor) VerifiedBackup(ctx context.Context, id model.ID) (model.Backup, error) {
	rows, err := e.store.ListBackups(ctx, id)
	if err != nil {
		return model.Backup{}, err
	}
	for _, b := range rows {
		if b.Verified {
			if _, err = backup.Validate(ctx, b.ArchivePath); err == nil {
				return b, nil
			}
		}
	}
	return model.Backup{}, errors.New("a verified full-pier export is required")
}

func (e *Executor) takeFullBackup(ctx context.Context, j *model.Job, ship model.Ship, dir string) error {
	var err error
	if err = e.supervisor.ValidatePier(ship, false); err != nil {
		return err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	id := model.NewID()
	destination := filepath.Join(dir, string(id)+".tar.gz")
	metadata := backup.Manifest{ShipIdentifier: ship.Name, Platform: runtime.GOOS + "/" + runtime.GOARCH, VereVersion: strings.TrimPrefix(ship.RuntimeVersion, "Vere "), LoomExponent: ship.LoomExponent}
	if err = e.phase(context.WithoutCancel(ctx), j, "executing"); err != nil {
		return err
	}
	if _, err = backup.ExportWithMetadata(context.WithoutCancel(ctx), ship.PierPath, destination, metadata); err != nil {
		return err
	}
	if _, err = backup.Validate(context.WithoutCancel(ctx), destination); err != nil {
		return err
	}
	if err = e.store.PutBackup(context.WithoutCancel(ctx), model.Backup{ID: id, ShipID: ship.ID, ArchivePath: destination, CreatedAt: time.Now().UTC(), Verified: true}); err != nil {
		return err
	}

	return nil
}
