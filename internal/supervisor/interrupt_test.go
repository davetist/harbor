package supervisor

import (
	"context"
	"errors"
	"harbor/internal/model"
	vr "harbor/internal/runtime"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInterruptUnresponsiveOwnedShipKeepsProcessAndIntent(t *testing.T) {
	s, st, ship := testSupervisor(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	t.Cleanup(func() { s.Stop(context.Background(), ship.ID) })
	if err := s.Start(ctx, "start", ship.ID); err != nil {
		t.Fatal(err)
	}
	ship, _ = st.GetShip(ctx, ship.ID)
	ship.DesiredRunning = true
	if err := st.PutShip(ctx, ship); err != nil {
		t.Fatal(err)
	}
	before, _ := st.ListOpenProcesses(ctx)
	if err := os.WriteFile(filepath.Join(ship.PierPath, "block-web"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.Interrupt(ctx, ship.ID); err != nil {
		t.Fatal(err)
	}
	for vr.CheckWeb(ctx, ship) != nil {
		if ctx.Err() != nil {
			t.Fatal("interrupt did not unblock web requests")
		}
	}
	after, _ := st.ListOpenProcesses(ctx)
	if len(before) != 1 || len(after) != 1 || before[0].LaunchID != after[0].LaunchID || before[0].PID != after[0].PID {
		t.Fatal("interrupt replaced or exited the runtime")
	}
	current, err := st.GetShip(ctx, ship.ID)
	if err != nil || current.State != "running" || !current.DesiredRunning || current.RecoveryBlocked {
		t.Fatal("interrupt changed lifecycle intent or recovery state", err)
	}
	if err := s.Interrupt(ctx, ship.ID); !errors.Is(err, ErrWebResponsive) {
		t.Fatal("responsive ship was interrupted", err)
	}
}

func TestInterruptRechecksJobsAndCancellationAfterProbe(t *testing.T) {
	for _, reason := range []string{"job", "cancel", "hold", "state"} {
		t.Run(reason, func(t *testing.T) {
			s, st, ship := testSupervisor(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			t.Cleanup(func() { s.Stop(context.Background(), ship.ID) })
			if err := s.Start(ctx, "start", ship.ID); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(ship.PierPath, "block-web"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			probe, stopProbe := context.WithCancel(ctx)
			defer stopProbe()
			result := make(chan error, 1)
			go func() { result <- s.Interrupt(probe, ship.ID) }()
			for {
				if _, err := os.Stat(filepath.Join(ship.PierPath, "probe-started")); err == nil {
					break
				}
				if ctx.Err() != nil {
					t.Fatal("response check never reached the listener")
				}
				time.Sleep(10 * time.Millisecond)
			}
			if reason == "job" {
				if err := st.CreateJob(ctx, model.Job{ID: "queued", ShipID: ship.ID, Kind: "stop", Trigger: "manual", Phase: "queued"}); err != nil {
					t.Fatal(err)
				}
			} else if reason == "cancel" {
				stopProbe()
			} else {
				current, err := st.GetShip(ctx, ship.ID)
				if err != nil {
					t.Fatal(err)
				}
				if reason == "hold" {
					current.RecoveryBlocked = true
					current.RecoveryBlockReason = "Preserve uncertain outcome."
				} else {
					current.State = "stopping"
				}
				if err = st.PutShip(ctx, current); err != nil {
					t.Fatal(err)
				}
			}
			if err := <-result; err == nil {
				t.Fatal("interrupt ignored changed admission or canceled request")
			}
			if _, err := os.Stat(filepath.Join(ship.PierPath, "block-web")); err != nil {
				t.Fatal("unsafe interrupt reached the worker")
			}
		})
	}
}

func TestInterruptRefusesUnsafeOrCanceledRequests(t *testing.T) {
	for _, reason := range []string{"stopped", "starting", "stopping", "maintaining", "externally_running", "hold", "pending", "initializing", "stop-race", "identity", "unowned", "canceled"} {
		t.Run(reason, func(t *testing.T) {
			s, st, ship := testSupervisor(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := s.Start(ctx, "start", ship.ID); err != nil {
				t.Fatal(err)
			}
			c := s.children[ship.ID]
			defer func() {
				s.mu.Lock()
				c.initializing = false
				c.stopping = false
				s.children[ship.ID] = c
				s.mu.Unlock()
				s.Stop(context.Background(), ship.ID)
			}()
			ship, _ = st.GetShip(ctx, ship.ID)
			if err := os.WriteFile(filepath.Join(ship.PierPath, "block-web"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			if reason == "pending" {
				if err := st.CreateJob(ctx, model.Job{ID: "queued", ShipID: ship.ID, Kind: "pack", Trigger: "manual", Phase: "queued"}); err != nil {
					t.Fatal(err)
				}
			}
			s.mu.Lock()
			switch reason {
			case "hold":
				ship.RecoveryBlocked = true
				ship.RecoveryBlockReason = "Preserve uncertain utility outcome."
			case "pending":
			case "initializing":
				c.initializing = true
			case "stop-race":
				c.stopping = true
			case "identity":
				impostor := *c
				impostor.record.StartIdentity = "different-process"
				s.children[ship.ID] = &impostor
			case "unowned":
				delete(s.children, ship.ID)
			case "canceled":
				cancel()
			default:
				ship.State = reason
			}
			s.mu.Unlock()
			if err := st.PutShip(context.Background(), ship); err != nil {
				t.Fatal(err)
			}
			if err := s.Interrupt(ctx, ship.ID); err == nil {
				t.Fatal("unsafe interrupt accepted")
			}
			if _, err := os.Stat(filepath.Join(ship.PierPath, "block-web")); err != nil {
				t.Fatal("refused request sent an interrupt")
			}
			current, _ := st.GetShip(context.Background(), ship.ID)
			if current.RecoveryBlocked != ship.RecoveryBlocked || current.RecoveryBlockReason != ship.RecoveryBlockReason {
				t.Fatal("interrupt cleared a recovery hold")
			}
		})
	}
}
