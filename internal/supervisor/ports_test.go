package supervisor

import (
	"context"
	"fmt"
	"harbor/internal/model"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func freeShipPort(t *testing.T, address string, first int) int {
	t.Helper()
	for port := first; port <= 65535; port++ {
		listener, err := net.Listen("tcp4", net.JoinHostPort(address, fmt.Sprint(port)))
		if err == nil {
			listener.Close()
			return port
		}
		if port == 80 {
			port = 8079
		}
	}
	t.Fatal("no free fixture port")
	return 0
}

func TestStartChoosesPortAtLaunchAndReusesStoppedPorts(t *testing.T) {
	s, st, ship := testSupervisor(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	want := freeShipPort(t, "127.0.0.1", 80)
	other := ship
	other.ID = model.ID("other-ship")
	other.PierPath = t.TempDir()
	other.WebPort = want // Stale metadata must not reserve a stopped ship's port.
	if err := os.MkdirAll(filepath.Join(other.PierPath, ".urb/log"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := st.PutShip(ctx, other); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Shutdown(context.Background()) })
	if err := s.Start(ctx, "first-start", ship.ID); err != nil {
		t.Fatal(err)
	}
	first, err := st.GetShip(ctx, ship.ID)
	if err != nil || first.WebPort != want || first.State != "running" {
		t.Fatalf("first start: port=%d want=%d state=%s err=%v", first.WebPort, want, first.State, err)
	}
	if err := s.Start(ctx, "other-start", other.ID); err != nil {
		t.Fatal(err)
	}
	second, err := st.GetShip(ctx, other.ID)
	if err != nil || second.WebPort == first.WebPort || second.State != "running" {
		t.Fatalf("running ships did not get separate ports: %+v, %v", second, err)
	}
	if err := s.Stop(ctx, ship.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Stop(ctx, other.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(ctx, "other-restart", other.ID); err != nil {
		t.Fatal(err)
	}
	restarted, err := st.GetShip(ctx, other.ID)
	if err != nil || restarted.WebPort != want || restarted.State != "running" {
		t.Fatalf("restart did not reuse the stopped ship's port: %+v want=%d err=%v", restarted, want, err)
	}
}

func TestStartSkipsBusyPreferredPorts(t *testing.T) {
	s, st, ship := testSupervisor(t)
	for _, port := range []int{80, 8080} {
		listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			t.Cleanup(func() { listener.Close() })
		}
	}
	want := freeShipPort(t, "127.0.0.1", 8081)
	ship.WebPort = 8080
	if err := st.PutShip(context.Background(), ship); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	t.Cleanup(func() { s.Shutdown(context.Background()) })
	if err := s.Start(ctx, "busy-start", ship.ID); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetShip(ctx, ship.ID)
	if err != nil || got.WebPort != want || got.State != "running" {
		t.Fatalf("busy-port fallback: %+v want=%d err=%v", got, want, err)
	}
}

func TestHTTPPortReservationsCoverTheIntervalBeforeVereBinds(t *testing.T) {
	s, _, _ := testSupervisor(t)
	first, releaseFirst, err := s.reserveHTTPPort(context.Background(), "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	defer releaseFirst()
	second, releaseSecond, err := s.reserveHTTPPort(context.Background(), "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	defer releaseSecond()
	if first == second {
		t.Fatal("overlapping launches reserved the same port before either runtime bound it")
	}
	releaseFirst()
	third, releaseThird, err := s.reserveHTTPPort(context.Background(), "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	defer releaseThird()
	if third != first {
		t.Fatalf("released reservation was not reusable: got %d want %d", third, first)
	}
	releaseFirst() // Cleanup must not release the third launch's reservation.
	fourth, releaseFourth, err := s.reserveHTTPPort(context.Background(), "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	defer releaseFourth()
	if fourth == third || fourth == second {
		t.Fatal("repeated cleanup released another launch's reservation")
	}
}

func TestFailedLaunchReleasesHTTPPortReservation(t *testing.T) {
	s, st, ship := testSupervisor(t)
	want := freeShipPort(t, "127.0.0.1", 80)
	binary := ship.RuntimePath
	ship.RuntimePath = filepath.Join(t.TempDir(), "missing-runtime")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := st.PutShip(ctx, ship); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(ctx, "failed-start", ship.ID); err == nil {
		t.Fatal("missing runtime was launched")
	}
	ship.RuntimePath = binary
	if err := st.PutShip(ctx, ship); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Shutdown(context.Background()) })
	if err := s.Start(ctx, "retry-start", ship.ID); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetShip(ctx, ship.ID)
	if err != nil || got.WebPort != want {
		t.Fatalf("failed launch retained its port: %+v want=%d err=%v", got, want, err)
	}
}

func TestWildcardHTTPPortSelectionSkipsIPv4Listeners(t *testing.T) {
	s, _, _ := testSupervisor(t)
	for _, port := range []int{80, 8080} {
		listener, err := net.Listen("tcp4", fmt.Sprintf("0.0.0.0:%d", port))
		if err == nil {
			t.Cleanup(func() { listener.Close() })
		}
	}
	want := freeShipPort(t, "0.0.0.0", 8081)
	got, release, err := s.reserveHTTPPort(context.Background(), "0.0.0.0")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if got != want {
		t.Fatalf("selected port %d despite occupied IPv4 listeners; want %d", got, want)
	}
}
