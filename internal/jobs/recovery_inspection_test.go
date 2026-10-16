package jobs

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func init() {
	if os.Getenv("HARBOR_RECOVERY_INFO_HELPER") != "1" {
		return
	}
	if os.Getenv("HARBOR_RECOVERY_INFO_MUTATE") == "1" {
		if err := os.WriteFile(filepath.Join(os.Args[len(os.Args)-1], ".urb/chk/image.bin"), []byte("runtime changed the snapshot"), 0600); err != nil {
			os.Exit(2)
		}
	}
	data, err := json.Marshal(os.Args[1:])
	if err != nil || os.WriteFile(os.Getenv("HARBOR_RECOVERY_INFO_ARGS"), data, 0600) != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func TestOfflineRecoveryInspectionRejectsMutationAndPreservesOriginal(t *testing.T) {
	ex, _, ship, _ := utilityMappingExecutor(t)
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ship.RuntimePath = binary
	t.Setenv("HARBOR_RECOVERY_INFO_HELPER", "1")
	t.Setenv("HARBOR_RECOVERY_INFO_ARGS", filepath.Join(t.TempDir(), "args.json"))
	t.Setenv("HARBOR_RECOVERY_INFO_MUTATE", "1")
	err = ex.inspectOffline(context.Background(), ship, "mutating-verification")
	data, readErr := os.ReadFile(filepath.Join(ship.PierPath, ".urb/chk/image.bin"))
	if err == nil || readErr != nil || string(data) != "snapshot" {
		t.Fatalf("inspection accepted a mutation or changed the original: inspection=%v read=%v bytes=%q", err, readErr, data)
	}
}

func TestOfflineRecoveryInspectionSizesLMDBBeforeLaunching(t *testing.T) {
	ex, st, ship, _ := utilityMappingExecutor(t)
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ship.RuntimePath = binary
	if err = st.PutShip(context.Background(), ship); err != nil {
		t.Fatal(err)
	}
	capture := filepath.Join(t.TempDir(), "info-args.json")
	t.Setenv("HARBOR_RECOVERY_INFO_HELPER", "1")
	t.Setenv("HARBOR_RECOVERY_INFO_ARGS", capture)
	// Exercise selection from the real logical file lengths without allocating
	// a 40 GiB inspection copy. Copy safety is checked separately, including with
	// an inspector that changes its input and returns success.
	_, _, smallShip, _ := utilityMappingExecutor(t)
	var inspectedPath string
	ex.copyForInspection = func(ctx context.Context, source string) (*recoveryCopy, error) {
		if source != ship.PierPath {
			t.Fatal("inspection did not copy the current pier")
		}
		copy, err := newRecoveryCopy(ctx, smallShip.PierPath)
		if err == nil {
			inspectedPath = copy.path
		}
		return copy, err
	}
	for _, trial := range []struct {
		bytes   int64
		mapSize string
	}{{1 << 20, "68719476736"}, {40 << 30, "137438953472"}} {
		database, err := os.Create(filepath.Join(ship.PierPath, ".urb/log/data.mdb"))
		if err != nil {
			t.Fatal(err)
		}
		err = database.Truncate(trial.bytes)
		database.Close()
		if err != nil {
			t.Fatal(err)
		}
		job := mappingTestJob(t, st, ship, "stop")
		if err = ex.inspectOffline(context.Background(), ship, job.ID); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(capture)
		if err != nil {
			t.Fatal(err)
		}
		var args []string
		if err = json.Unmarshal(data, &args); err != nil {
			t.Fatal(err)
		}
		want := []string{"info", "--lmdb-map-size", trial.mapSize, "--loom", "31", inspectedPath}
		if !reflect.DeepEqual(args, want) {
			t.Fatalf("offline inspection received %v; want %v", args, want)
		}
		if inspectedPath == ship.PierPath {
			t.Fatal("runtime inspected the original pier")
		}
		if _, err := os.Stat(inspectedPath); !os.IsNotExist(err) {
			t.Fatal("temporary inspection copy was not removed", err)
		}
		processes, err := st.ProcessesForJob(context.Background(), job.ID)
		if err != nil || len(processes) != 1 || processes[0].Phase != "exited" || processes[0].ExitCode == nil || *processes[0].ExitCode != 0 {
			t.Fatal("offline inspection did not record a confirmed exit", processes, err)
		}
	}
	if err = os.Remove(capture); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(filepath.Join(ship.PierPath, ".urb/log/data.mdb")); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(t.TempDir(), filepath.Join(ship.PierPath, ".urb/log/data.mdb")); err != nil {
		t.Fatal(err)
	}
	job := mappingTestJob(t, st, ship, "stop")
	if err = ex.inspectOffline(context.Background(), ship, job.ID); err == nil {
		t.Fatal("unsafe log storage reached offline inspection")
	}
	if _, err = os.Stat(capture); !os.IsNotExist(err) {
		t.Fatal("inspection child launched despite unsafe log storage", err)
	}
	processes, err := st.ProcessesForJob(context.Background(), job.ID)
	if err != nil || len(processes) != 0 {
		t.Fatal("unsafe storage created a process record", processes, err)
	}
}
