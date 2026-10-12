package scheduler

import (
	"errors"
	"fmt"
	"harbor/internal/model"
	vr "harbor/internal/runtime"
)

type Resources struct {
	AvailableMemory uint64
	TotalMemory     uint64
	FreeDisk        uint64
	SnapshotBytes   uint64
	LoomExponent    int
	RuntimeVersion  string
	Hardware        string
}
type OperationProfile struct {
	Kind         model.Operation
	IntervalDays int
	LocalTime    string
	Enabled      bool
	AfterRoll    bool
}
type Profile struct {
	Name             string
	EnabledByDefault bool
	Certified        bool
	RetainedEpochs   int
	Operations       []OperationProfile
}

var ErrUnvalidatedTarget = errors.New("no measured maintenance profile for this hardware, runtime or loom")

// These are bounded native baseline profiles, not a claim of optimal cadence
// or certification of all ships on the same CPU architecture.
func ProfileFor(target vr.Platform, r Resources) (Profile, error) {
	if target.Arch != "arm64" || r.LoomExponent != 31 || r.RuntimeVersion != "4.6" {
		return Profile{}, ErrUnvalidatedTarget
	}
	measured := (target.OS == "darwin" && r.Hardware == "Apple M2 Max" && r.TotalMemory >= 32<<30) || (target.OS == "linux" && r.Hardware == "Raspberry Pi 4 Model B Rev 1.4" && r.TotalMemory >= 7<<30)
	if !measured {
		return Profile{}, ErrUnvalidatedTarget
	}
	return Profile{Name: "Weekly upkeep · 2 GiB loom", EnabledByDefault: true, Certified: true, RetainedEpochs: 2, Operations: []OperationProfile{{"pack", 7, "03:00", true, false}, {"meld", 28, "03:05", true, false}, {"roll", 7, "03:10", true, false}, {"chop", 7, "03:15", true, true}}}, nil
}
func Preflight(target vr.Platform, r Resources, op model.Operation, running, scheduled bool) error {
	if r.SnapshotBytes == 0 {
		return errors.New("snapshot size could not be established; maintenance was not started")
	}
	if r.SnapshotBytes > 1<<30 {
		return errors.New("snapshot exceeds the measured 1 GiB envelope; maintenance was not started")
	}
	if op == "meld" {
		if _, err := ProfileFor(target, r); err != nil {
			return err
		}
		if scheduled && r.SnapshotBytes < 512<<20 {
			return errors.New("Meld skipped: snapshot is below the 512 MiB pressure threshold")
		}
	}
	needMemory := map[model.Operation]uint64{"pack": 2 << 30, "meld": 4 << 30, "roll": 512 << 20, "chop": 256 << 20}[op]
	if needMemory == 0 {
		return errors.New("unknown maintenance operation")
	}
	if r.AvailableMemory < needMemory {
		return fmt.Errorf("%s needs %.2f GiB available RAM; %.2f GiB is available", op, float64(needMemory)/(1<<30), float64(r.AvailableMemory)/(1<<30))
	}
	needDisk := 2*r.SnapshotBytes + (256 << 20)
	if op == "chop" && !running {
		needDisk = 64 << 20
	}
	if r.FreeDisk < needDisk {
		return fmt.Errorf("%s needs %.2f GiB free on the pier volume for checkpoint/work space; %.2f GiB is free", op, float64(needDisk)/(1<<30), float64(r.FreeDisk)/(1<<30))
	}
	return nil
}
