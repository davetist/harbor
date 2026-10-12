package onboarding

import (
	"context"
	"errors"
	"harbor/internal/config"
	"harbor/internal/jobs"
	"harbor/internal/model"
	"harbor/internal/pier"
	vr "harbor/internal/runtime"
	"harbor/internal/scheduler"
	"harbor/internal/store"
	"harbor/internal/supervisor"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type Service struct {
	Config        config.Config
	Store         *store.Store
	Executor      *jobs.Executor
	Supervisor    *supervisor.Supervisor
	Uploads       Uploads
	ReadResources func(context.Context, string) (scheduler.Resources, error)
}
type Request struct {
	Mode          string   `json:"mode"`
	FolderName    string   `json:"folderName"`
	Name          string   `json:"name"`
	PierPath      string   `json:"pierPath"`
	LoomExponent  int      `json:"loomExponent"`
	UploadID      model.ID `json:"uploadId,omitempty"`
	ImportArchive bool     `json:"importArchive,omitempty"`
	LoomConfirmed bool     `json:"loomConfirmed,omitempty"`
}
type CometRequest = Request
type KeyfileRequest = Request
type RegisterRequest = Request

var localLabel = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`)
var identity = regexp.MustCompile(`^~?(?:[a-z]{3}|[a-z]{6}(?:-[a-z]{6})*)$`)

func (s *Service) CreateComet(ctx context.Context, r CometRequest) (model.ID, error) {
	r.Mode = "comet"
	return s.create(ctx, r)
}
func (s *Service) BootKeyfile(ctx context.Context, r KeyfileRequest) (model.ID, error) {
	r.Mode = "keyfile"
	return s.create(ctx, r)
}
func (s *Service) RegisterPier(ctx context.Context, r RegisterRequest) (model.ID, error) {
	r.Mode = "existing"
	return s.create(ctx, r)
}
func (s *Service) create(ctx context.Context, r Request) (model.ID, error) {
	if r.ImportArchive {
		return s.queueArchive(ctx, r)
	}
	if r.Mode == "existing" {
		if !r.LoomConfirmed {
			return "", errors.New("confirm the existing pier’s original loom size; Harbor will not guess or reduce it")
		}
		if r.LoomExponent < 30 || r.LoomExponent > 32 {
			return "", errors.New("existing loom requirement is not supported")
		}
	} else if r.LoomExponent != 31 {
		return "", errors.New("new ships currently require the validated 2 GiB setting")
	}
	kind := model.Operation("boot_comet")
	label := r.FolderName
	name := "Comet"
	path := r.PierPath
	newDestination := r.Mode != "existing"
	switch r.Mode {
	case "comet":
		if !localLabel.MatchString(label) || label == "." || label == ".." {
			return "", errors.New("choose a simple folder name")
		}
	case "keyfile":
		if r.Name == "" || !identity.MatchString(r.Name) {
			return "", errors.New("confirm the intended Urbit identity")
		}
		if _, e := s.Uploads.KeyPath(r.UploadID); e != nil {
			return "", e
		}
		name = "~" + strings.TrimPrefix(r.Name, "~")
		label = strings.TrimPrefix(name, "~")
		kind = "boot_keyfile"
	case "existing":
		kind = "import"
		name = filepath.Base(path)
	default:
		return "", errors.New("unknown onboarding mode")
	}
	if newDestination {
		if path == "" {
			path = s.Config.AllowedRoots[0]
		}
		path = filepath.Join(path, label)
	}
	canonical, e := pier.Canonical(path, newDestination)
	if e != nil {
		return "", e
	}
	if !newDestination {
		if _, e = pier.Inspect(canonical); e != nil {
			return "", e
		}
		if locked, e := pier.WriterLocked(canonical); e != nil || locked {
			return "", errors.New("stop the other runner before registering this pier")
		}
	}
	ship := model.Ship{ID: model.NewID(), Name: name, Nickname: label, PierPath: canonical, LoomExponent: r.LoomExponent, HTTPBindAddress: s.Config.DefaultShipHTTPBindAddress, State: "stopped"}
	job := model.Job{ID: model.NewID(), ShipID: ship.ID, Kind: kind, Trigger: "manual", Phase: "queued", UploadID: r.UploadID}
	if e = s.Store.RegisterQueued(ctx, ship, job, s.initialSchedules(ship)...); e != nil {
		return "", errors.New("pier is already registered or metadata could not be saved")
	}
	return job.ID, nil
}
func (s *Service) Wire() {
	s.Executor.Hooks["import_archive"] = s.executeArchive
	for _, kind := range []model.Operation{"boot_comet", "boot_keyfile", "import"} {
		s.Executor.Hooks[kind] = s.execute
	}
}
func (s *Service) execute(ctx context.Context, j *model.Job) error {
	ship, e := s.Store.GetShip(ctx, j.ShipID)
	if e != nil {
		return e
	}
	if j.UploadID != "" {
		defer s.Uploads.Remove(j.UploadID)
	}
	if err := pier.VerifyRegistered(ship.PierPath, j.Kind != "import"); err != nil {
		return err
	}
	platform, e := vr.NativePlatform()
	if e != nil {
		return e
	}
	runtime, e := (vr.Resolver{Dir: filepath.Join(s.Config.DataDir, "runtimes")}).Resolve(ctx, platform, vr.StableVersion)
	if e != nil {
		return e
	}
	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	_, e = vr.ProbeCapabilities(probeCtx, runtime.Path)
	cancel()
	if e != nil {
		return e
	}
	ship.RuntimePath = runtime.Path
	ship.RuntimeVersion = "Vere " + vr.StableVersion
	ship.DesiredRunning = true
	if e = s.Store.PutShip(ctx, ship); e != nil {
		return e
	}
	args, e := vr.BuildArgs(ship, "start")
	if e != nil {
		return e
	}
	args = append([]string{"--no-dock"}, args...)
	if j.Kind != "import" {
		if _, e = os.Lstat(ship.PierPath); !os.IsNotExist(e) {
			return errors.New("boot destination now exists; refusing to overwrite")
		}
		args = args[:len(args)-1]
		if j.Kind == "boot_keyfile" {
			key, e := s.Uploads.KeyPath(j.UploadID)
			if e != nil {
				return e
			}
			args = append(args, "--name", strings.TrimPrefix(ship.Name, "~"), "--key-file", key)
		}
		args = append(args, "--pier", ship.PierPath)
	}
	ready, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()
	if j.Kind == "import" {
		e = s.Supervisor.Start(ready, j.ID, ship.ID)
	} else {
		e = s.Supervisor.LaunchNewShip(ready, j.ID, ship, args)
	}
	if e != nil {
		ship, _ = s.Store.GetShip(context.WithoutCancel(ctx), ship.ID)
		ship.DesiredRunning = false
		ship.Error = e.Error()
		s.Store.PutShip(context.WithoutCancel(ctx), ship)
		return e
	}
	return nil
}

func (s *Service) initialSchedules(ship model.Ship) []model.Schedule {
	zone, _ := time.LoadLocation(s.Config.Timezone)
	read := s.ReadResources
	if read == nil {
		read = scheduler.ReadResources
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resources, resourceErr := read(ctx, ship.PierPath)
	resources.LoomExponent = ship.LoomExponent
	resources.RuntimeVersion = vr.StableVersion
	target, _ := vr.NativePlatform()
	profile, profileErr := scheduler.ProfileFor(target, resources)
	operations := []scheduler.OperationProfile{{Kind: "pack", IntervalDays: 7, LocalTime: "03:00"}, {Kind: "meld", IntervalDays: 28, LocalTime: "03:05"}, {Kind: "roll", IntervalDays: 7, LocalTime: "03:10"}, {Kind: "chop", IntervalDays: 7, LocalTime: "03:15", AfterRoll: true}}
	if resourceErr == nil && profileErr == nil {
		operations = profile.Operations
	}
	rows := make([]model.Schedule, 0, 4)
	var roll model.ID
	for _, operation := range operations {
		kind := operation.Kind
		row := model.Schedule{ID: model.NewID(), ShipID: ship.ID, Kind: kind, AnchorDate: time.Now().In(zone).Format("2006-01-02"), IntervalDays: operation.IntervalDays, LocalTime: operation.LocalTime, Timezone: s.Config.Timezone, Enabled: operation.Enabled, LastResult: "Never run"}
		if !operation.Enabled {
			row.LastResult = "No measured default profile for this configuration"
		}
		if kind == "roll" {
			roll = row.ID
		}
		if kind == "chop" {
			row.AfterScheduleID = roll
		}
		row.NextDue, _ = scheduler.NextOccurrence(row, time.Now())
		rows = append(rows, row)
	}
	return rows
}
