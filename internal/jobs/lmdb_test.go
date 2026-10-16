package jobs

import (
	"context"
	"encoding/json"
	"harbor/internal/model"
	"harbor/internal/pier"
	vr "harbor/internal/runtime"
	"harbor/internal/scheduler"
	"harbor/internal/store"
	"harbor/internal/supervisor"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestUtilityMappingHelper(t *testing.T) {
	if os.Getenv("HARBOR_UTILITY_MAPPING_HELPER") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			args, err := json.Marshal(os.Args[i+1:])
			if err != nil || os.WriteFile(os.Getenv("HARBOR_UTILITY_MAPPING_ARGS"), args, 0600) != nil {
				os.Exit(2)
			}
			os.Exit(0)
		}
	}
	os.Exit(2)
}

func utilityMappingExecutor(t *testing.T) (*Executor, *store.Store, model.Ship, string) {
	t.Helper()
	root := t.TempDir()
	st, err := store.Open(filepath.Join(root, "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	p := filepath.Join(root, "pier")
	for _, dir := range []string{".urb/chk", ".urb/log"} {
		if err := os.MkdirAll(filepath.Join(p, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(p, ".urb/chk/image.bin"), []byte("snapshot"), 0600); err != nil {
		t.Fatal(err)
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	runtimePath := filepath.Join(root, "utility")
	// Keep the executor's actual subprocess path while routing only Vere itself
	// to this test binary. The helper records the arguments received by the child.
	wrapper := "#!/bin/sh\nexec '" + strings.ReplaceAll(binary, "'", "'\"'\"'") + "' -test.run='^TestUtilityMappingHelper$' -- \"$@\"\n"
	if err := os.WriteFile(runtimePath, []byte(wrapper), 0700); err != nil {
		t.Fatal(err)
	}
	ship := model.Ship{ID: "ship", PierPath: p, RuntimePath: runtimePath, State: "stopped", LoomExponent: 31, RuntimeVersion: "Vere 4.6"}
	if err := st.PutShip(context.Background(), ship); err != nil {
		t.Fatal(err)
	}
	ship, err = st.GetShip(context.Background(), ship.ID)
	if err != nil {
		t.Fatal(err)
	}
	capture := filepath.Join(root, "args.json")
	t.Setenv("HARBOR_UTILITY_MAPPING_HELPER", "1")
	t.Setenv("HARBOR_UTILITY_MAPPING_ARGS", capture)
	ex := New(st, supervisor.New(st, nil))
	ex.resources = func(context.Context, string) (scheduler.Resources, error) {
		// Resource admission is separate from map sizing; supply a measured
		// profile so Meld reaches the utility launch on supported ARM64 targets.
		hardware := "Apple M2 Max"
		if target, err := vr.NativePlatform(); err == nil && target.OS == "linux" {
			hardware = "Raspberry Pi 4 Model B Rev 1.4"
		}
		return scheduler.Resources{AvailableMemory: 8 << 30, TotalMemory: 32 << 30, FreeDisk: 20 << 30, SnapshotBytes: 1, Hardware: hardware}, nil
	}
	return ex, st, ship, capture
}

func mappingTestJob(t *testing.T, st *store.Store, ship model.Ship, op model.Operation) model.Job {
	t.Helper()
	if op == "meld" {
		target, err := vr.NativePlatform()
		if err != nil {
			t.Fatal(err)
		}
		if target.Arch != "arm64" {
			t.Skip("Meld resource admission requires a measured ARM64 profile")
		}
	}
	j := model.Job{ID: model.NewID(), ShipID: ship.ID, Kind: op, Trigger: "manual", Phase: "queued"}
	if err := st.CreateJob(context.Background(), j); err != nil {
		t.Fatal(err)
	}
	return j
}

func TestMaintenanceUtilityRecomputesMapFromDatabaseSize(t *testing.T) {
	for _, op := range []model.Operation{"pack", "meld", "roll", "chop"} {
		t.Run(string(op), func(t *testing.T) {
			ex, st, ship, capture := utilityMappingExecutor(t)
			dbPath := filepath.Join(ship.PierPath, ".urb/log/0i123/data.mdb")
			if err := os.MkdirAll(filepath.Dir(dbPath), 0700); err != nil {
				t.Fatal(err)
			}
			for _, trial := range []struct {
				bytes    int64
				mapBytes string
			}{{1 << 20, "68719476736"}, {40 << 30, "137438953472"}, {100 << 30, "274877906944"}} {
				db, err := os.Create(dbPath)
				if err != nil {
					t.Fatal(err)
				}
				err = db.Truncate(trial.bytes) // Sparse logical growth, without allocating the disk space.
				db.Close()
				if err != nil {
					t.Fatal(err)
				}
				j := mappingTestJob(t, st, ship, op)
				if err := ex.maintain(context.Background(), &j, ship); err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(capture)
				if err != nil {
					t.Fatal(err)
				}
				var args []string
				if err := json.Unmarshal(data, &args); err != nil {
					t.Fatal(err)
				}
				want := []string{string(op), "--lmdb-map-size", trial.mapBytes, "--loom", "31", ship.PierPath}
				if !reflect.DeepEqual(args, want) {
					t.Errorf("database %d: child received %v; want %v", trial.bytes, args, want)
				}
			}
		})
	}
}

func TestMaintenanceUtilityUncheckedLMDBRefusesSpawnAndReleasesGuard(t *testing.T) {
	for _, op := range []model.Operation{"pack", "meld", "roll", "chop"} {
		t.Run(string(op), func(t *testing.T) {
			ex, st, ship, capture := utilityMappingExecutor(t)
			if err := os.Symlink(t.TempDir(), filepath.Join(ship.PierPath, ".urb/log/data.mdb")); err != nil {
				t.Fatal(err)
			}
			j := mappingTestJob(t, st, ship, op)
			if err := ex.maintain(context.Background(), &j, ship); err == nil || !strings.Contains(err.Error(), "symlink") {
				t.Error("missing unsafe-storage refusal", err)
			}
			if _, err := os.Stat(capture); !os.IsNotExist(err) {
				t.Error("child spawned despite unchecked LMDB", err)
			}
			records, err := st.ProcessesForJob(context.Background(), j.ID)
			if err != nil || len(records) != 0 {
				t.Error("refused utility recorded a process", records, err)
			}
			got, err := st.GetShip(context.Background(), ship.ID)
			if err != nil || got.State != "stopped" || got.RecoveryBlocked {
				t.Error("refused utility changed ship state", got, err)
			}
			if release, err := pier.Guard(ship.PierPath); err != nil {
				t.Error("refused utility retained the pier guard", err)
			} else {
				release()
			}
		})
	}
}

func TestReplayAndUpdateUtilitiesKeepOriginalArgs(t *testing.T) {
	for _, trial := range []struct {
		op   model.Operation
		args []string
	}{{"replay", []string{"play", "--full", "--loom", "31"}}, {"update", []string{"prep", "--loom", "31"}}} {
		t.Run(string(trial.op), func(t *testing.T) {
			ex, st, ship, capture := utilityMappingExecutor(t)
			// These subcommands cannot accept the map override. They still require
			// the common native-storage safety checks before executing.
			if err := os.WriteFile(filepath.Join(ship.PierPath, ".urb/log/data.mdb"), []byte("fixture"), 0600); err != nil {
				t.Fatal(err)
			}
			j := mappingTestJob(t, st, ship, trial.op)
			if err := ex.maintain(context.Background(), &j, ship); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(capture)
			if err != nil {
				t.Fatal(err)
			}
			var args []string
			if err := json.Unmarshal(data, &args); err != nil {
				t.Fatal(err)
			}
			if want := append(trial.args, ship.PierPath); !reflect.DeepEqual(args, want) {
				t.Fatalf("child received %v; want %v", args, want)
			}
		})
	}
}
