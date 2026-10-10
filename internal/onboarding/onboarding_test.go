package onboarding

import (
	"bytes"
	"context"
	"harbor/internal/config"
	"harbor/internal/model"
	"harbor/internal/scheduler"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestStagedKeyIsPrivateAndRemoved(t *testing.T) {
	u := Uploads{Dir: t.TempDir()}
	id, e := u.StageKey(bytes.NewBufferString("0w123abc"))
	if e != nil {
		t.Fatal(e)
	}
	p, e := u.KeyPath(id)
	if e != nil {
		t.Fatal(e)
	}
	st, e := os.Stat(p)
	if e != nil || st.Mode().Perm() != 0600 {
		t.Fatalf("unsafe key mode %v", e)
	}
	if e = u.Remove(id); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(p); !os.IsNotExist(e) {
		t.Fatal("staged key retained")
	}
	if _, e = u.KeyPath("../../secrets"); e == nil {
		t.Fatal("upload path traversal")
	}
}
func TestKeyLimitsAndFormat(t *testing.T) {
	u := Uploads{Dir: t.TempDir()}
	for _, key := range []string{"", "a password", string(bytes.Repeat([]byte("x"), 65537))} {
		if _, e := u.StageKey(bytes.NewBufferString(key)); e == nil {
			t.Fatal("invalid key accepted")
		}
	}
	files, _ := filepath.Glob(filepath.Join(u.Dir, "*"))
	if len(files) > 0 {
		t.Fatal("invalid staged bytes retained")
	}
}

func TestMeasuredInitialSchedulesEnabledAndPaired(t *testing.T) {
	if runtime.GOARCH != "arm64" {
		t.Skip("native measured profile is ARM64")
	}
	hardware := "Apple M2 Max"
	total := uint64(32 << 30)
	if runtime.GOOS == "linux" {
		hardware = "Raspberry Pi 4 Model B Rev 1.4"
		total = 8 << 30
	}
	s := Service{Config: config.Config{Timezone: "UTC"}, ReadResources: func(context.Context, string) (scheduler.Resources, error) {
		return scheduler.Resources{Hardware: hardware, TotalMemory: total, AvailableMemory: 8 << 30, FreeDisk: 20 << 30}, nil
	}}
	rows := s.initialSchedules(model.Ship{ID: "new", PierPath: "/new", LoomExponent: 31})
	if len(rows) != 4 {
		t.Fatal("missing defaults")
	}
	var roll model.Schedule
	for _, row := range rows {
		if !row.Enabled || row.LastResult != "Never run" {
			t.Fatal("new defaults paused", row)
		}
		if row.Kind == "roll" {
			roll = row
		}
	}
	for _, row := range rows {
		if row.Kind == "chop" && (row.AfterScheduleID != roll.ID || row.IntervalDays != roll.IntervalDays) {
			t.Fatal("unpaired retention")
		}
	}
	rows = s.initialSchedules(model.Ship{ID: "other", PierPath: "/new", LoomExponent: 32})
	for _, row := range rows {
		if row.Enabled {
			t.Fatal("unmeasured loom enabled")
		}
	}
}
