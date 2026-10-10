package app

import (
	"context"
	"fmt"
	"harbor/internal/backup"
	"harbor/internal/jobs"
	"harbor/internal/model"
	"harbor/internal/scheduler"
	"harbor/internal/store"
	"harbor/internal/supervisor"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// A real child process exercises Harbor's ownership, readiness and graceful
// stop paths without using a live Urbit identity or the public network.
func TestExportProcessHelper(t *testing.T) {
	if os.Getenv("HARBOR_EXPORT_PROCESS_HELPER") != "1" {
		return
	}
	port := ""
	for i, arg := range os.Args {
		if arg == "--http-port" && i+1 < len(os.Args) {
			port = os.Args[i+1]
		}
	}
	if port == "" {
		os.Exit(0)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		os.Exit(2)
	}
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM)
	go http.Serve(listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			fmt.Fprint(w, `"~zod"`)
			return
		}
		w.Write([]byte("ready"))
	}))
	os.WriteFile(filepath.Join(os.Args[len(os.Args)-1], ".http.ports"), []byte(port+" insecure loopback\n"), 0600)
	<-ch
	listener.Close()
	os.Exit(0)
}

func TestPierExportLeavesSourceStoppedAcrossRestart(t *testing.T) {
	for _, tc := range []struct {
		name    string
		running bool
		fail    bool
	}{
		{"running source", true, false},
		{"stopped source with autostart enabled", false, false},
		{"failed export after stopping source", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			root := t.TempDir()
			db := filepath.Join(root, "db")
			st, err := store.Open(db)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { st.Close() })
			pierPath := filepath.Join(root, "pier")
			for _, dir := range []string{".urb/chk", ".urb/log"} {
				if err = os.MkdirAll(filepath.Join(pierPath, dir), 0700); err != nil {
					t.Fatal(err)
				}
			}
			if err = os.WriteFile(filepath.Join(pierPath, ".urb/chk/image.bin"), []byte("snapshot"), 0600); err != nil {
				t.Fatal(err)
			}
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			port := listener.Addr().(*net.TCPAddr).Port
			listener.Close()
			binary, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			ship := model.Ship{ID: "export-source", Name: "~zod", PierPath: pierPath, RuntimePath: binary, RuntimeVersion: "Vere 4.6", LoomExponent: 31, HTTPBindAddress: "127.0.0.1", WebPort: port, State: "stopped", DesiredRunning: true}
			if err = st.PutShip(ctx, ship); err != nil {
				t.Fatal(err)
			}
			launcher := func(binary string, args ...string) *exec.Cmd {
				cmd := exec.Command(binary, append([]string{"-test.run=TestExportProcessHelper", "--"}, args...)...)
				cmd.Env = append(os.Environ(), "HARBOR_EXPORT_PROCESS_HELPER=1")
				return cmd
			}
			sup := supervisor.New(st, launcher)
			t.Cleanup(func() { sup.Shutdown(context.Background()) })
			if tc.running {
				if err = sup.Start(ctx, "initial-start", ship.ID); err != nil {
					t.Fatal(err)
				}
			}
			archiveDir := filepath.Join(root, "exports")
			if tc.fail {
				if err = os.WriteFile(archiveDir, []byte("not a directory"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			ex := jobs.New(st, sup)
			ex.EnableBackups(archiveDir)
			ex.SetReady(true)
			id, err := ex.Submit(ctx, model.JobRequest{ShipID: ship.ID, Kind: "backup", Trigger: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			if err = ex.RunOne(ctx); err != nil {
				t.Fatal(err)
			}
			job, err := st.GetJob(ctx, id)
			wantPhase := "completed"
			if tc.fail {
				wantPhase = "failed"
			}
			if err != nil || job.Phase != wantPhase {
				t.Fatalf("export job: %+v, %v", job, err)
			}
			got, err := st.GetShip(ctx, ship.ID)
			if err != nil || got.State != "stopped" || got.DesiredRunning {
				t.Fatalf("export must leave the source intentionally stopped: %+v, %v", got, err)
			}
			open, err := st.ListOpenProcesses(ctx)
			if err != nil || len(open) != 0 {
				t.Fatalf("source process survived or restarted: %+v, %v", open, err)
			}
			archives, err := st.ListBackups(ctx, ship.ID)
			if err != nil {
				t.Fatal(err)
			}
			if tc.fail {
				if len(archives) != 0 {
					t.Fatal("failed export published an archive")
				}
			} else {
				if len(archives) != 1 || !archives[0].Verified {
					t.Fatalf("verified export missing: %+v", archives)
				}
				if _, err = backup.Validate(ctx, archives[0].ArchivePath); err != nil {
					t.Fatal(err)
				}
			}
			if err = st.Close(); err != nil {
				t.Fatal(err)
			}
			st, err = store.Open(db)
			if err != nil {
				t.Fatal(err)
			}
			sup = supervisor.New(st, launcher)
			ex = jobs.New(st, sup)
			if err = RecoverAndStart(ctx, st, sup, ex); err != nil {
				t.Fatal(err)
			}
			if pending, err := st.HasPendingJob(ctx, ship.ID); err != nil || pending {
				t.Fatalf("Harbor restart queued an automatic start: %v, %v", pending, err)
			}
			// Already-enabled maintenance must not restart or advance an exported
			// source when the next scheduled occurrence becomes due.
			if err = st.PutSchedule(ctx, model.Schedule{ID: "pack-after-export", ShipID: ship.ID, Kind: "pack", IntervalDays: 7, AnchorDate: "2026-10-10", LocalTime: "03:00", Timezone: "UTC", Enabled: true, NextDue: time.Now().Add(-time.Minute)}); err != nil {
				t.Fatal(err)
			}
			if _, err = st.ClaimDueAndEnqueue(ctx, time.Now(), scheduler.NextOccurrence); err != nil {
				t.Fatal(err)
			}
			if err = ex.RunOne(ctx); err != nil {
				t.Fatal(err)
			}
			all, err := st.JobsForShip(ctx, ship.ID)
			if err != nil || len(all) != 2 || all[0].Kind != "pack" || all[0].Phase != "skipped" {
				t.Fatalf("scheduled maintenance advanced the exported source: %+v, %v", all, err)
			}
		})
	}
}
