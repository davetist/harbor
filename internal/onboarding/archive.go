package onboarding

import (
	"context"
	"errors"
	"harbor/internal/backup"
	"harbor/internal/model"
	"harbor/internal/pier"
	vr "harbor/internal/runtime"
	"path/filepath"
	"time"
)

func (s *Service) queueArchive(ctx context.Context, r Request) (model.ID, error) {
	if _, e := s.Uploads.ArchivePath(r.UploadID); e != nil {
		return "", e
	}
	parent := r.PierPath
	if parent == "" {
		parent = s.Config.AllowedRoots[0]
	}
	parent, e := pier.Canonical(parent, s.Config.AllowedRoots, false)
	if e != nil {
		return "", e
	}
	label := r.FolderName
	if label == "" {
		label = "import-" + string(r.UploadID)[:8]
	}
	if !localLabel.MatchString(label) || label == "." || label == ".." {
		return "", errors.New("invalid import folder name")
	}
	destination, e := pier.Canonical(filepath.Join(parent, label), s.Config.AllowedRoots, true)
	if e != nil {
		return "", e
	}
	ship := model.Ship{ID: model.NewID(), Name: "Import", PierPath: destination, HTTPBindAddress: s.Config.DefaultShipHTTPBindAddress, State: "stopped"}
	j := model.Job{ID: model.NewID(), ShipID: ship.ID, Kind: "import_archive", Trigger: "manual", Phase: "queued", UploadID: r.UploadID}
	if e = s.Store.QueuePendingImport(ctx, ship, j); e != nil {
		return "", e
	}
	return j.ID, nil
}
func (s *Service) executeArchive(ctx context.Context, j *model.Job) error {
	ship, e := s.Store.GetPendingImport(ctx, j.ShipID)
	if e != nil {
		return e
	}
	if canonical, err := pier.Canonical(ship.PierPath, s.Config.AllowedRoots, true); err != nil || canonical != ship.PierPath {
		return errors.New("import destination changed or is outside permitted folders")
	}
	archive, e := s.Uploads.ArchivePath(j.UploadID)
	if e != nil {
		return e
	}
	manifest, e := backup.Validate(ctx, archive)
	if e != nil {
		return e
	}
	if manifest.LoomExponent < 30 || manifest.LoomExponent > 32 {
		return errors.New("archive loom requirement is missing or not supported; it has not been reduced")
	}
	platform, e := vr.NativePlatform()
	if e != nil {
		return e
	}
	runtime, e := (vr.Resolver{Dir: filepath.Join(s.Config.DataDir, "runtimes")}).Resolve(ctx, platform, vr.StableVersion)
	if e != nil {
		return e
	}
	if _, e = backup.Restore(ctx, archive, ship.PierPath); e != nil {
		return e
	}
	if _, e = pier.Inspect(ship.PierPath); e != nil {
		return e
	}
	// Only a complete verified destination becomes a registered ship. Original
	// uploaded archive bytes are retained independently of extraction/boot.
	ship.Name = manifest.ShipIdentifier
	ship.LoomExponent = manifest.LoomExponent
	ship.RuntimePath = runtime.Path
	ship.RuntimeVersion = "Vere " + vr.StableVersion
	ship.DesiredRunning = true
	ship.WebPort, e = s.availablePort(ctx)
	if e != nil {
		return e
	}
	if e = s.Store.PutShip(ctx, ship); e != nil {
		return e
	}
	if e = s.Store.RemovePendingImport(ctx, ship.ID); e != nil {
		return e
	}
	for _, row := range s.initialSchedules(ship) {
		if e = s.Store.PutSchedule(ctx, row); e != nil {
			return e
		}
	}
	ready, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()
	return s.Supervisor.Start(ready, j.ID, ship.ID)
}
