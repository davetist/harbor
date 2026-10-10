package supervisor

import (
	"bufio"
	"context"
	"fmt"
	"harbor/internal/model"
	"harbor/internal/pier"
	"harbor/internal/store"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestProcessHelper(t *testing.T) {
	if os.Getenv("HARBOR_PROCESS_HELPER") != "1" {
		return
	}
	port := ""
	for i, a := range os.Args {
		if a == "--http-port" && i+1 < len(os.Args) {
			port = os.Args[i+1]
		}
	}
	listener, e := net.Listen("tcp", "127.0.0.1:"+port)
	if e != nil {
		os.Exit(2)
	}
	pierPath := os.Args[len(os.Args)-1]
	os.WriteFile(filepath.Join(pierPath, ".http.ports"), []byte(port+" insecure loopback\n"), 0600)
	go http.Serve(listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			fmt.Fprint(w, `"~zod"`)
			return
		}
		fmt.Fprint(w, "ready")
	}))
	go func() {
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			fmt.Println("received:", sc.Text())
		}
	}()
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM)
	<-ch
	listener.Close()
	os.Exit(0)
}
func testSupervisor(t *testing.T) (*Supervisor, *store.Store, model.Ship) {
	t.Helper()
	st, e := store.Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { st.Close() })
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	binary, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	ship := model.Ship{ID: "test-ship", Name: "~zod", PierPath: t.TempDir(), RuntimePath: binary, LoomExponent: 31, HTTPBindAddress: "127.0.0.1", WebPort: port, State: "stopped"}
	if err := os.MkdirAll(filepath.Join(ship.PierPath, ".urb/log"), 0700); err != nil {
		t.Fatal(err)
	}
	if e = st.PutShip(context.Background(), ship); e != nil {
		t.Fatal(e)
	}
	launcher := func(binary string, args ...string) *exec.Cmd {
		cmd := exec.Command(binary, append([]string{"-test.run=TestProcessHelper", "--"}, args...)...)
		cmd.Env = append(os.Environ(), "HARBOR_PROCESS_HELPER=1")
		return cmd
	}
	ship, e = st.GetShip(context.Background(), ship.ID)
	if e != nil {
		t.Fatal(e)
	}
	return New(st, launcher), st, ship
}

func TestLaunchRecomputesMapFromDatabaseSize(t *testing.T) {
	s, _, ship := testSupervisor(t)
	launcher := s.launch
	var launches [][]string
	s.launch = func(binary string, args ...string) *exec.Cmd {
		launches = append(launches, append([]string(nil), args...))
		return launcher(binary, args...)
	}
	t.Cleanup(func() { s.Stop(context.Background(), ship.ID) })
	for i := 0; i < 2; i++ {
		if i == 1 {
			db, err := os.Create(filepath.Join(ship.PierPath, ".urb/log/data.mdb"))
			if err != nil {
				t.Fatal(err)
			}
			err = db.Truncate(40 << 30)
			db.Close()
			if err != nil {
				t.Fatal(err)
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := s.Start(ctx, "map-job", ship.ID); err != nil {
			cancel()
			t.Fatal(err)
		}
		if err := s.Stop(ctx, ship.ID); err != nil {
			cancel()
			t.Fatal(err)
		}
		cancel()
	}
	for i, args := range launches {
		joined := strings.Join(args, " ")
		if strings.Count(joined, "--lmdb-map-size") != 1 || !strings.Contains(joined, "--lmdb-map-size "+[]string{"68719476736", "137438953472"}[i]) {
			t.Fatal("launch lost or duplicated map override", args)
		}
	}
}

func TestUncheckedLMDBRefusesLaunchAndReleasesGuard(t *testing.T) {
	s, st, ship := testSupervisor(t)
	db := filepath.Join(ship.PierPath, ".urb/log/data.mdb")
	if err := os.Symlink(t.TempDir(), db); err != nil {
		t.Fatal(err)
	}
	s.launch = func(string, ...string) *exec.Cmd {
		t.Fatal("runtime launched despite unchecked LMDB")
		return nil
	}
	ctx := context.Background()
	if err := s.Start(ctx, "map-job", ship.ID); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatal("missing unsafe-storage refusal", err)
	}
	processes, err := st.ListOpenProcesses(ctx)
	if err != nil || len(processes) != 0 {
		t.Fatal("refused launch left an uncertain process", processes, err)
	}
	if release, err := pier.Guard(ship.PierPath); err != nil {
		t.Fatal("refused launch retained the pier guard", err)
	} else {
		release()
	}
}
func TestReadinessAndDojoInput(t *testing.T) {
	s, st, ship := testSupervisor(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if e := s.Start(ctx, "job-1", ship.ID); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Stop(context.Background(), ship.ID) })
	got, e := st.GetShip(ctx, ship.ID)
	if e != nil || got.State != "running" {
		t.Fatalf("%+v %v", got, e)
	}
	if e = s.WriteInput(ctx, ship.ID, []byte("(add 2 2)\n")); e != nil {
		t.Fatal(e)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && !strings.Contains(s.Logs(ship.ID), "received: (add 2 2)") {
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(s.Logs(ship.ID), "received: (add 2 2)") {
		t.Fatal("Dojo input did not reach process")
	}
	if e = s.Stop(ctx, ship.ID); e != nil {
		t.Fatal(e)
	}
	got, _ = st.GetShip(ctx, ship.ID)
	if got.State != "stopped" {
		t.Fatal(got.State)
	}
}
func TestRecoveryHoldPreventsStart(t *testing.T) {
	s, st, ship := testSupervisor(t)
	ship.RecoveryBlocked = true
	ship.RecoveryBlockReason = "uncertain operation"
	st.PutShip(context.Background(), ship)
	if e := s.Start(context.Background(), "job-1", ship.ID); e == nil {
		t.Fatal("start bypassed hold")
	}
	ps, _ := st.ListOpenProcesses(context.Background())
	if len(ps) != 0 {
		t.Fatal("launched before hold check")
	}
}
func TestLogsAreBounded(t *testing.T) {
	b := &LogBuffer{}
	data := strings.Repeat("x", 2<<20)
	b.Write([]byte(data))
	if len(b.String()) > 1<<20 {
		t.Fatal("unbounded log")
	}
}

func TestAutonomousExitPublishesFailedState(t *testing.T) {
	s, st, ship := testSupervisor(t)
	events := make(chan model.Event, 8)
	s.OnChange = func(e model.Event) { events <- e }
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Start(ctx, "job", ship.ID); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	process := s.children[ship.ID].cmd.Process
	s.mu.Unlock()
	if err := process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	for {
		select {
		case event := <-events:
			if got, ok := event.Data.(model.Ship); ok && got.State == "failed" {
				stored, _ := st.GetShip(ctx, ship.ID)
				if stored.State != "failed" {
					t.Fatal("event preceded durable failure")
				}
				return
			}
		case <-ctx.Done():
			t.Fatal("browser received no autonomous failure event")
		}
	}
}
