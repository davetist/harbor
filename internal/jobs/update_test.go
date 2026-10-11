package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"harbor/internal/model"
	vr "harbor/internal/runtime"
	"harbor/internal/store"
	"harbor/internal/supervisor"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Substitute only Vere; the executor, cache verification, process ownership,
// graceful stop, readiness checks and durable store remain real.
func TestRuntimeUpdateProcess(t *testing.T) {
	if os.Getenv("HARBOR_UPDATE_TEST_PROCESS") != "1" {
		return
	}
	args := os.Args[1:]
	for i, arg := range args {
		if arg == "--" {
			args = args[i+1:]
			break
		}
	}
	path := args[len(args)-1]
	if args[0] == "prep" {
		data, err := json.Marshal(args)
		if err != nil || os.WriteFile(filepath.Join(path, ".update-prepared"), data, 0600) != nil {
			os.Exit(2)
		}
		if os.Getenv("HARBOR_UPDATE_TEST_FAIL_PREP") == "1" {
			os.Exit(3)
		}
		os.Exit(0)
	}
	if os.Getenv("HARBOR_UPDATE_TEST_FAIL_START") == "1" {
		os.Exit(4)
	}
	port := ""
	for i, arg := range args {
		if arg == "--http-port" && i+1 < len(args) {
			port = args[i+1]
		}
	}
	if port == "" {
		os.Exit(2)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:"+port)
	if err != nil {
		os.Exit(2)
	}
	if os.WriteFile(filepath.Join(path, ".http.ports"), []byte(port+" insecure loopback\n"), 0600) != nil {
		os.Exit(2)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			fmt.Fprint(w, `"~zod"`)
			return
		}
		fmt.Fprint(w, "ready")
	})}
	go server.Serve(listener)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM)
	<-stop
	server.Close()
	os.Exit(0)
}

func runtimeUpdateExecutor(t *testing.T, failRestart bool) (*Executor, *store.Store, model.Ship, string) {
	t.Helper()
	ctx := context.Background()
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
	oldRuntime := filepath.Join(root, "old-vere")
	wrapper := "#!/bin/sh\nexec '" + strings.ReplaceAll(binary, "'", "'\"'\"'") + "' -test.run='^TestRuntimeUpdateProcess$' -- \"$@\"\n"
	if err := os.WriteFile(oldRuntime, []byte(wrapper), 0700); err != nil {
		t.Fatal(err)
	}
	platform, err := vr.NativePlatform()
	if err != nil {
		t.Fatal(err)
	}
	runtimeDir := filepath.Join(root, "runtimes")
	selected := filepath.Join(runtimeDir, "4.6", platform.Artifact, "vere")
	if err := os.MkdirAll(filepath.Dir(selected), 0700); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(selected, data, 0700); err != nil {
		t.Fatal(err)
	}
	digest, err := vr.FileDigest(selected)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(selected+".sha256", []byte(digest), 0600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	ship := model.Ship{ID: "ship", Name: "~zod", PierPath: p, RuntimePath: oldRuntime, RuntimeVersion: "Vere 4.5", State: "stopped", LoomExponent: 31, HTTPBindAddress: "127.0.0.1", WebPort: port}
	if err := st.PutShip(ctx, ship); err != nil {
		t.Fatal(err)
	}
	ship, err = st.GetShip(ctx, ship.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HARBOR_UPDATE_TEST_PROCESS", "1")
	sup := supervisor.New(st, func(path string, args ...string) *exec.Cmd {
		// Launch a native helper directly so a shell exec cannot change the
		// executable identity between process recording and graceful stop.
		launch := path
		if path == oldRuntime {
			launch = binary
		}
		cmd := exec.Command(launch, append([]string{"-test.run=^TestRuntimeUpdateProcess$", "--"}, args...)...)
		if failRestart && path == selected {
			cmd.Env = append(os.Environ(), "HARBOR_UPDATE_TEST_FAIL_START=1")
		}
		return cmd
	})
	t.Cleanup(func() {
		stop, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := sup.Stop(stop, ship.ID); err != nil {
			t.Error(err)
		}
	})
	ex := New(st, sup)
	ex.EnableRuntimeUpdates(runtimeDir)
	ex.SetReady(true)
	return ex, st, ship, selected
}

func TestRuntimeUpdateWithoutExportRestoresRunningState(t *testing.T) {
	for _, running := range []bool{false, true} {
		t.Run(fmt.Sprint(running), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			ex, st, ship, selected := runtimeUpdateExecutor(t, false)
			ship.DesiredRunning = running
			if err := st.PutShip(ctx, ship); err != nil {
				t.Fatal(err)
			}
			if running {
				if err := ex.supervisor.Start(ctx, "initial-start", ship.ID); err != nil {
					t.Fatal(err)
				}
			}
			id, err := ex.Submit(ctx, model.JobRequest{ShipID: ship.ID, Kind: "update", Trigger: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			if err := ex.RunOne(ctx); err != nil {
				t.Fatal(err)
			}
			job, err := st.GetJob(ctx, id)
			if err != nil || job.Phase != "completed" || job.PriorRunning != running {
				t.Fatalf("update did not complete without an export: %+v %v", job, err)
			}
			got, err := st.GetShip(ctx, ship.ID)
			wantState := "stopped"
			if running {
				wantState = "running"
			}
			if err != nil || got.State != wantState || got.DesiredRunning != running || got.RuntimePath != selected || got.RuntimeVersion != "Vere 4.6" || got.RecoveryBlocked {
				t.Fatalf("update lost runtime or running state: %+v %v", got, err)
			}
			prepared, err := os.ReadFile(filepath.Join(ship.PierPath, ".update-prepared"))
			var args []string
			if err != nil || json.Unmarshal(prepared, &args) != nil || !reflect.DeepEqual(args, []string{"prep", "--loom", "31", ship.PierPath}) {
				t.Fatalf("old runtime did not prepare the pier: %s %v", prepared, err)
			}
			backups, err := st.ListBackups(ctx, ship.ID)
			if err != nil || len(backups) != 0 {
				t.Fatalf("runtime update created a full-pier export: %+v %v", backups, err)
			}
			if _, err := os.Stat(ship.RuntimePath); err != nil {
				t.Fatalf("previous runtime was removed: %v", err)
			}
		})
	}
}

func TestRuntimeUpdateFailuresPreserveRecoveryHold(t *testing.T) {
	for _, failure := range []string{"prep", "restart"} {
		t.Run(failure, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			ex, st, ship, selected := runtimeUpdateExecutor(t, failure == "restart")
			ship.DesiredRunning = true
			if err := st.PutShip(ctx, ship); err != nil {
				t.Fatal(err)
			}
			if err := ex.supervisor.Start(ctx, "initial-start", ship.ID); err != nil {
				t.Fatal(err)
			}
			if failure == "prep" {
				t.Setenv("HARBOR_UPDATE_TEST_FAIL_PREP", "1")
			}
			id, err := ex.Submit(ctx, model.JobRequest{ShipID: ship.ID, Kind: "update", Trigger: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			if err := ex.RunOne(ctx); err != nil {
				t.Fatal(err)
			}
			job, err := st.GetJob(ctx, id)
			if err != nil || job.Phase != "failed" || job.Error == "" {
				t.Fatalf("failed %s was not reported: %+v %v", failure, job, err)
			}
			got, err := st.GetShip(ctx, ship.ID)
			wantRuntime := ship.RuntimePath
			if failure == "restart" {
				wantRuntime = selected
			}
			if err != nil || !got.RecoveryBlocked || got.RecoveryBlockReason == "" || got.RuntimePath != wantRuntime || !got.DesiredRunning || got.State == "running" {
				t.Fatalf("failed %s lost recovery state: %+v %v", failure, got, err)
			}
			if _, err := ex.Submit(ctx, model.JobRequest{ShipID: ship.ID, Kind: "start", Trigger: "manual"}); err == nil {
				t.Fatal("start bypassed failed-update hold")
			}
			if backups, err := st.ListBackups(ctx, ship.ID); err != nil || len(backups) != 0 {
				t.Fatalf("failed update created an export: %+v %v", backups, err)
			}
		})
	}
}

func TestCurrentRuntimeUpdateLeavesRunningShipUndisturbed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ex, st, ship, selected := runtimeUpdateExecutor(t, false)
	ship.RuntimePath = selected
	ship.DesiredRunning = true
	if err := st.PutShip(ctx, ship); err != nil {
		t.Fatal(err)
	}
	if err := ex.supervisor.Start(ctx, "initial-start", ship.ID); err != nil {
		t.Fatal(err)
	}
	before, err := st.ProcessesForJob(ctx, "initial-start")
	if err != nil || len(before) != 1 {
		t.Fatalf("initial process: %+v %v", before, err)
	}
	id, err := ex.Submit(ctx, model.JobRequest{ShipID: ship.ID, Kind: "update", Trigger: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if err := ex.RunOne(ctx); err != nil {
		t.Fatal(err)
	}
	job, err := st.GetJob(ctx, id)
	if err != nil || job.Phase != "completed" {
		t.Fatalf("current runtime update: %+v %v", job, err)
	}
	after, err := st.ProcessesForJob(ctx, "initial-start")
	if err != nil || len(after) != 1 || after[0].Phase != "live" || after[0].PID != before[0].PID {
		t.Fatalf("up-to-date runtime disturbed the running ship: %+v %v", after, err)
	}
	if records, err := st.ProcessesForJob(ctx, id); err != nil || len(records) != 0 {
		t.Fatalf("up-to-date runtime launched a utility: %+v %v", records, err)
	}
	if _, err := os.Stat(filepath.Join(ship.PierPath, ".update-prepared")); !os.IsNotExist(err) {
		t.Fatalf("up-to-date runtime prepared the pier: %v", err)
	}
}
