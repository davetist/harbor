package supervisor

import (
	"context"
	"fmt"
	"harbor/internal/model"
	"harbor/internal/pier"
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

// Public HTTP can answer before the worker can handle a private query. Before
// the bootstrap marker, this helper also ignores TERM like Vere's empty pier
// list. Only a later TERM can stop the fully initialized helper cleanly.
func TestBootstrapProcessHelper(t *testing.T) {
	if os.Getenv("HARBOR_BOOTSTRAP_HELPER") != "1" {
		return
	}
	port := ""
	for i, a := range os.Args {
		if a == "--http-port" && i+1 < len(os.Args) {
			port = os.Args[i+1]
		}
	}
	path := os.Args[len(os.Args)-1]
	if err := os.MkdirAll(filepath.Join(path, ".urb/log"), 0700); err != nil {
		os.Exit(2)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		os.Exit(2)
	}
	os.WriteFile(filepath.Join(path, ".http.ports"), []byte(port+" insecure loopback\n"), 0600)
	os.WriteFile(filepath.Join(path, "helper-started"), nil, 0600)
	ready := func() bool {
		_, err := os.Stat(filepath.Join(path, "finish-bootstrap"))
		return err == nil
	}
	go http.Serve(listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			if !ready() {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			fmt.Fprint(w, `"~zod"`)
			return
		}
		fmt.Fprint(w, "ready")
	}))
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM)
	for range signals {
		if ready() {
			listener.Close()
			os.Exit(0)
		}
		os.WriteFile(filepath.Join(path, "ignored-term"), nil, 0600)
	}
}

func bootstrapSupervisor(t *testing.T) (*Supervisor, model.Ship) {
	t.Helper()
	s, st, ship := testSupervisor(t)
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ship.PierPath = filepath.Join(parent, "new-pier")
	if err := st.PutShip(context.Background(), model.Ship{ID: "bootstrap", Name: "Comet", PierPath: ship.PierPath, RuntimePath: ship.RuntimePath, LoomExponent: 31, HTTPBindAddress: "127.0.0.1", WebPort: ship.WebPort}); err != nil {
		t.Fatal(err)
	}
	ship, _ = st.GetShip(context.Background(), "bootstrap")
	s.launch = func(binary string, args ...string) *exec.Cmd {
		cmd := exec.Command(binary, append([]string{"-test.run=^TestBootstrapProcessHelper$", "--"}, args...)...)
		cmd.Env = append(os.Environ(), "HARBOR_BOOTSTRAP_HELPER=1")
		return cmd
	}
	t.Cleanup(func() {
		os.WriteFile(filepath.Join(ship.PierPath, "finish-bootstrap"), nil, 0600)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.Stop(ctx, ship.ID)
	})
	return s, ship
}

func waitBootstrapHelper(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(path, "helper-started")); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("helper did not start")
}

func TestBootstrapReadinessRequiresPrivateWorkerIdentity(t *testing.T) {
	s, ship := bootstrapSupervisor(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	args := []string{"--http-port", fmt.Sprint(ship.WebPort), ship.PierPath}
	started := make(chan error, 1)
	go func() { started <- s.LaunchNewShip(ctx, "", ship, args) }()
	waitBootstrapHelper(t, ship.PierPath)
	select {
	case err := <-started:
		t.Fatalf("public HTTP was mistaken for worker readiness: %v", err)
	case <-time.After(600 * time.Millisecond):
	}
	os.WriteFile(filepath.Join(ship.PierPath, "finish-bootstrap"), nil, 0600)
	if err := <-started; err != nil {
		t.Fatal(err)
	}
	got, _ := s.store.GetShip(ctx, ship.ID)
	if got.State != "running" || got.Identity != "~zod" {
		t.Fatal("ready worker identity was not persisted", got)
	}
}

func TestShutdownDuringBootstrapWaitsForHandledGracefulStop(t *testing.T) {
	s, ship := bootstrapSupervisor(t)
	launchCtx, cancel := context.WithCancel(context.Background())
	started := make(chan error, 1)
	go func() {
		started <- s.LaunchNewShip(launchCtx, "", ship, []string{"--http-port", fmt.Sprint(ship.WebPort), ship.PierPath})
	}()
	waitBootstrapHelper(t, ship.PierPath)
	cancel()
	<-started
	ctx, stop := context.WithTimeout(context.Background(), 3*time.Second)
	defer stop()
	shutdown := make(chan error, 1)
	go func() { shutdown <- s.Shutdown(ctx) }()
	time.Sleep(300 * time.Millisecond)
	os.WriteFile(filepath.Join(ship.PierPath, "finish-bootstrap"), nil, 0600)
	if err := <-shutdown; err != nil {
		t.Fatal("bootstrap stop request was lost", err)
	}
	got, _ := s.store.GetShip(context.Background(), ship.ID)
	if got.State != "stopped" {
		t.Fatal("clean exit was not recorded", got.State)
	}
	processes, err := s.store.ListOpenProcesses(context.Background())
	if err != nil || len(processes) != 0 {
		t.Fatal("shutdown returned with an owned process", processes, err)
	}
	if release, err := pier.Guard(ship.PierPath); err != nil {
		t.Fatal("shutdown retained the writer guard", err)
	} else {
		release()
	}
}

func TestCanceledBootstrapStopPreservesOwnership(t *testing.T) {
	s, ship := bootstrapSupervisor(t)
	launchCtx, cancel := context.WithCancel(context.Background())
	started := make(chan error, 1)
	go func() {
		started <- s.LaunchNewShip(launchCtx, "", ship, []string{"--http-port", fmt.Sprint(ship.WebPort), ship.PierPath})
	}()
	waitBootstrapHelper(t, ship.PierPath)
	cancel()
	<-started
	stopCtx, stop := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer stop()
	if err := s.Stop(stopCtx, ship.ID); err == nil {
		t.Fatal("incomplete bootstrap stop reported success")
	}
	if _, err := os.Stat(filepath.Join(ship.PierPath, "ignored-term")); !os.IsNotExist(err) {
		t.Fatal("TERM sent before bootstrap could handle it")
	}
	processes, err := s.store.ListOpenProcesses(context.Background())
	if err != nil || len(processes) != 1 || processes[0].Phase != "live" {
		t.Fatal("canceled stop lost its owned process", processes, err)
	}
	if release, err := pier.Guard(ship.PierPath); err == nil {
		release()
		t.Fatal("canceled stop released its writer guard")
	}
	os.WriteFile(filepath.Join(ship.PierPath, "finish-bootstrap"), nil, 0600)
	ctx, cleanup := context.WithTimeout(context.Background(), 5*time.Second)
	defer cleanup()
	if err := s.Stop(ctx, ship.ID); err != nil {
		t.Fatal("owned process could not finish after cancellation", err)
	}
}
