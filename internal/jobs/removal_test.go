package jobs

import (
	"context"
	"errors"
	"harbor/internal/model"
	"harbor/internal/store"
	"path/filepath"
	"testing"
)

func TestRemovalAndSubmissionCannotLeaveOrphanWork(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	e := New(s, nil)
	e.SetReady(true)
	for range 30 {
		ship := model.Ship{ID: model.NewID(), PierPath: filepath.Join(t.TempDir(), "pier"), State: "stopped"}
		if err := s.PutShip(ctx, ship); err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		removal, submission := make(chan error, 1), make(chan error, 1)
		go func() { <-start; removal <- e.RemoveShip(ctx, ship.ID) }()
		go func() {
			<-start
			_, err := e.Submit(ctx, model.JobRequest{ShipID: ship.ID, Kind: "start", Trigger: "manual"})
			submission <- err
		}()
		close(start)
		removeErr, submitErr := <-removal, <-submission
		if (removeErr == nil) == (submitErr == nil) {
			t.Fatalf("expected exactly one action to succeed: remove=%v submit=%v", removeErr, submitErr)
		}
		_, lookup := s.GetShip(ctx, ship.ID)
		pending, err := s.HasPendingJob(ctx, ship.ID)
		if err != nil {
			t.Fatal(err)
		}
		if removeErr == nil && (!errors.Is(lookup, store.ErrNotFound) || pending) {
			t.Fatalf("removed ship left work or registration: lookup=%v pending=%v", lookup, pending)
		}
		if submitErr == nil && (lookup != nil || !pending) {
			t.Fatalf("accepted work lost registration: lookup=%v pending=%v", lookup, pending)
		}
	}
}
