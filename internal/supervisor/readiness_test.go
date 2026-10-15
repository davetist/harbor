package supervisor

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadinessRetryPreservesConcurrentState(t *testing.T) {
	for _, change := range []string{"hold", "configuration", "identity", "cancel", "stop"} {
		t.Run(change, func(t *testing.T) {
			s, ship := bootstrapSupervisor(t)
			ship.Identity = "~zod"
			if err := s.store.PutShip(context.Background(), ship); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			err := s.LaunchNewShip(ctx, "first", ship, []string{"--http-port", fmt.Sprint(ship.WebPort), ship.PierPath})
			cancel()
			if err == nil {
				t.Fatal("unready helper was accepted")
			}
			if err := os.WriteFile(filepath.Join(ship.PierPath, "finish-bootstrap"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			entered, release := make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				select {
				case entered <- struct{}{}:
				default:
				}
				select {
				case <-release:
				case <-r.Context().Done():
					return
				}
				identity := "~zod"
				if change == "identity" {
					identity = "~nec"
				}
				fmt.Fprintf(w, "%q", identity)
			}))
			defer server.Close()
			port := server.Listener.Addr().(*net.TCPAddr).Port
			portsPath := filepath.Join(ship.PierPath, ".http.ports")
			originalPorts, err := os.ReadFile(portsPath)
			if err != nil {
				t.Fatal(err)
			}
			defer os.WriteFile(portsPath, originalPorts, 0600)
			if err := os.WriteFile(filepath.Join(ship.PierPath, ".http.ports"), []byte(fmt.Sprintf("%d insecure loopback\n", port)), 0600); err != nil {
				t.Fatal(err)
			}
			retryCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
			defer stop()
			result := make(chan error, 1)
			go func() { result <- s.Start(retryCtx, "retry", ship.ID) }()
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				close(release)
				t.Fatal("retry did not query private readiness")
			}
			if err := s.Start(context.Background(), "duplicate", ship.ID); err == nil {
				close(release)
				t.Fatal("concurrent Start was accepted")
			}
			stopped := make(chan error, 1)
			if change == "stop" {
				go func() { stopped <- s.Stop(context.Background(), ship.ID) }()
				deadline := time.Now().Add(2 * time.Second)
				for {
					s.mu.Lock()
					stopping := s.children[ship.ID] != nil && s.children[ship.ID].stopping
					s.mu.Unlock()
					if stopping {
						break
					}
					if time.Now().After(deadline) {
						close(release)
						t.Fatal("Stop did not claim the child")
					}
					time.Sleep(10 * time.Millisecond)
				}
			} else if change == "cancel" {
				stop()
			} else if change != "identity" {
				s.mu.Lock()
				current, err := s.store.GetShip(context.Background(), ship.ID)
				if err != nil {
					s.mu.Unlock()
					close(release)
					t.Fatal(err)
				}
				if change == "hold" {
					current.RecoveryBlocked = true
					current.RecoveryBlockReason = "Preserve the uncertain outcome."
					current.Error = "Recovery required."
					current.State = "failed"
				} else {
					current.LoomExponent = 32
				}
				err = s.store.PutShip(context.Background(), current)
				s.mu.Unlock()
				if err != nil {
					close(release)
					t.Fatal(err)
				}
			}
			close(release)
			if err := <-result; err == nil {
				t.Fatal("changed or canceled readiness was accepted")
			}
			if change == "stop" {
				if err := <-stopped; err != nil {
					t.Fatal(err)
				}
			}
			current, err := s.store.GetShip(context.Background(), ship.ID)
			if err != nil || current.State == "running" || current.State == "starting" {
				t.Fatal("retry overwrote changed state", current.State, err)
			}
			if change == "hold" && (!current.RecoveryBlocked || current.Error != "Recovery required." || current.RecoveryBlockReason != "Preserve the uncertain outcome.") {
				t.Fatal("retry erased the recovery hold")
			}
			if change == "configuration" && current.LoomExponent != 32 {
				t.Fatal("retry overwrote changed configuration")
			}
			if change == "identity" && current.Identity != "~zod" {
				t.Fatal("retry replaced the expected identity")
			}
			if change == "stop" && current.State != "stopped" {
				t.Fatal("retry overwrote graceful stop", current.State)
			}
		})
	}
}

func TestReadinessRetryRejectsChangedOwnershipAtAdmission(t *testing.T) {
	for _, change := range []string{"process", "binary", "configuration", "hold", "cancel", "strict-launch"} {
		t.Run(change, func(t *testing.T) {
			s, ship := bootstrapSupervisor(t)
			ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
			err := s.LaunchNewShip(ctx, "first", ship, []string{"--http-port", fmt.Sprint(ship.WebPort), ship.PierPath})
			cancel()
			if err == nil {
				t.Fatal("unready helper was accepted")
			}
			s.mu.Lock()
			c := s.children[ship.ID]
			original := c.record
			if change == "process" {
				c.record.StartIdentity = "changed"
			}
			if change == "binary" {
				c.record.BinaryDigest = "changed"
			}
			current, err := s.store.GetShip(context.Background(), ship.ID)
			if err == nil {
				if change == "configuration" {
					current.LoomExponent = 32
				}
				if change == "hold" {
					current.RecoveryBlocked = true
					current.RecoveryBlockReason = "Preserve recovery."
				}
				err = s.store.PutShip(context.Background(), current)
			}
			s.mu.Unlock()
			defer func() { s.mu.Lock(); c.record = original; s.mu.Unlock() }()
			if err != nil {
				t.Fatal(err)
			}
			retryCtx, stop := context.WithCancel(context.Background())
			defer stop()
			if change == "cancel" {
				stop()
			}
			if change == "strict-launch" {
				err = s.LaunchShip(retryCtx, "retry", current, []string{})
			} else {
				err = s.Start(retryCtx, "retry", ship.ID)
			}
			if err == nil {
				t.Fatal("invalid retry admission was accepted")
			}
			after, err := s.store.GetShip(context.Background(), ship.ID)
			if err != nil || after.State != "failed" || after.RecoveryBlocked != current.RecoveryBlocked || after.LoomExponent != current.LoomExponent {
				t.Fatal("refused retry changed durable state", after.State, err)
			}
		})
	}
}
