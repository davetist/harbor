package supervisor

import (
	"context"
	"errors"
	"harbor/internal/model"
	vr "harbor/internal/runtime"
	"time"
)

var ErrWebResponsive = errors.New("The ship is responding again. Try Open ship.")

// Interrupt sends Vere's supported Ctrl+C through the owned terminal. This is
// an explicit request to cancel the current event, never a timeout policy.
func (s *Supervisor) Interrupt(ctx context.Context, id model.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.children[id]
	if c == nil || c.stopping || c.initializing {
		return errors.New("The running ship's terminal is not available for interruption.")
	}
	ship, err := s.store.GetShip(ctx, id)
	if err != nil {
		return err
	}
	if ship.RecoveryBlocked {
		return errors.New(ship.RecoveryBlockReason)
	}
	if ship.State != "running" {
		return errors.New("Only a running ship can be interrupted.")
	}
	pending, err := s.store.HasPendingJob(ctx, id)
	if err != nil {
		return err
	}
	if pending {
		return errors.New("Wait for this ship's active or queued operation to finish.")
	}
	if err = vr.CheckWeb(ctx, ship); err == nil {
		return ErrWebResponsive
	} else if !errors.Is(err, vr.ErrWebUnresponsive) {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	// Hold final admission through the write, including scheduled work that
	// arrives during the probe. mu also serializes stop and process replacement.
	return s.store.WithIdleRunningShip(ctx, id, func(current model.Ship) error {
		if current.WebPort != ship.WebPort || current.HTTPBindAddress != ship.HTTPBindAddress {
			return errors.New("The ship's listener changed during the response check. Review it before trying again.")
		}
		identity, err := StartIdentity(c.record.PID)
		if err != nil || identity == "" || identity != c.record.StartIdentity {
			return errors.New("Process ownership could not be confirmed; refusing to interrupt.")
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		c.tty.SetWriteDeadline(time.Now().Add(2 * time.Second))
		_, err = c.tty.Write([]byte{3})
		return err
	})
}
