package scheduler

import (
	"errors"
	vr "harbor/internal/runtime"
	"testing"
)

func measuredResources(os string) Resources {
	hardware := "Apple M2 Max"
	total := uint64(32 << 30)
	if os == "linux" {
		hardware = "Raspberry Pi 4 Model B Rev 1.4"
		total = uint64(7995120 * 1024)
	}
	return Resources{AvailableMemory: 8 << 30, TotalMemory: total, FreeDisk: 20 << 30, SnapshotBytes: 800 << 20, LoomExponent: 31, RuntimeVersion: "4.6", Hardware: hardware}
}
func TestDefaultMaintenanceEnabled(t *testing.T) {
	for _, os := range []string{"darwin", "linux"} {
		target, _ := vr.DetectPlatform(os, "arm64")
		p, err := ProfileFor(target, measuredResources(os))
		if err != nil || !p.Certified || !p.EnabledByDefault {
			t.Fatal("measured profile disabled", p, err)
		}
		want := map[string]int{"pack": 7, "meld": 28, "roll": 7, "chop": 7}
		for _, op := range p.Operations {
			if op.IntervalDays != want[string(op.Kind)] || !op.Enabled {
				t.Fatal("unexpected default", op)
			}
		}
	}
}
func TestUnvalidatedTargetCannotClaimCertifiedDefaults(t *testing.T) {
	for _, pair := range [][2]string{{"darwin", "amd64"}, {"linux", "amd64"}} {
		target, _ := vr.DetectPlatform(pair[0], pair[1])
		p, err := ProfileFor(target, measuredResources(pair[0]))
		if !errors.Is(err, ErrUnvalidatedTarget) || p.Certified || p.EnabledByDefault {
			t.Fatal("unmeasured Intel profile claimed", p, err)
		}
	}
	target, _ := vr.DetectPlatform("linux", "arm64")
	r := measuredResources("linux")
	r.Hardware = "unknown ARM host"
	if _, err := ProfileFor(target, r); !errors.Is(err, ErrUnvalidatedTarget) {
		t.Fatal("unknown hardware claimed")
	}
	r = measuredResources("linux")
	r.LoomExponent = 32
	if _, err := ProfileFor(target, r); !errors.Is(err, ErrUnvalidatedTarget) {
		t.Fatal("unmeasured loom claimed")
	}
}
func TestProfileRetentionMatchesDocumentedEpochPolicy(t *testing.T) {
	target, _ := vr.DetectPlatform("linux", "arm64")
	p, err := ProfileFor(target, measuredResources("linux"))
	if err != nil {
		t.Fatal(err)
	}
	if p.RetainedEpochs != 2 {
		t.Fatal("retention is not Vere's two epochs")
	}
	var roll, chop OperationProfile
	for _, v := range p.Operations {
		if v.Kind == "roll" {
			roll = v
		}
		if v.Kind == "chop" {
			chop = v
		}
	}
	if chop.IntervalDays != roll.IntervalDays || chop.LocalTime <= roll.LocalTime || !chop.AfterRoll {
		t.Fatal("retention pair invalid")
	}
}
func TestPreconditionsCoverPressureMemoryDiskAndScope(t *testing.T) {
	target, _ := vr.DetectPlatform("linux", "arm64")
	r := measuredResources("linux")
	if err := Preflight(target, r, "meld", false, true); err != nil {
		t.Fatal(err)
	}
	r.AvailableMemory = 1 << 30
	if err := Preflight(target, r, "meld", false, true); err == nil {
		t.Fatal("memory shortage accepted")
	}
	r = measuredResources("linux")
	r.FreeDisk = 100 << 20
	if err := Preflight(target, r, "pack", true, true); err == nil {
		t.Fatal("checkpoint space accepted")
	}
	r = measuredResources("linux")
	r.SnapshotBytes = 300 << 20
	if err := Preflight(target, r, "meld", true, true); err == nil {
		t.Fatal("scheduled Meld ran with no pressure")
	}
	if err := Preflight(target, r, "meld", true, false); err != nil {
		t.Fatal("explicit manual Meld unnecessarily skipped", err)
	}
	r.SnapshotBytes = 2 << 30
	if err := Preflight(target, r, "meld", true, false); err == nil {
		t.Fatal("unmeasured state size accepted")
	}
}
