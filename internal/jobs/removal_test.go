package jobs

import (
	"context"
	"errors"
	"harbor/internal/model"
	"harbor/internal/pier"
	"harbor/internal/store"
	"harbor/internal/supervisor"
	"os"
	"path/filepath"
	"testing"
)

func TestPendingDeletionSurvivesRestartAndCanBeFinished(t *testing.T) {
	for _, alreadyRemoved := range []bool{false, true} {
		t.Run(map[bool]string{false: "partial", true: "files-gone"}[alreadyRemoved], func(t *testing.T) {
			ctx := context.Background()
			db := filepath.Join(t.TempDir(), "db")
			s, err := store.Open(db)
			if err != nil {
				t.Fatal(err)
			}
			parent, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(parent, "pier")
			if err = os.Mkdir(path, 0700); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(path, "remaining"), []byte("data"), 0600); err != nil {
				t.Fatal(err)
			}
			ship := model.Ship{ID: "comet", PierPath: path, State: "stopped"}
			if err = s.PutShip(ctx, ship); err != nil {
				t.Fatal(err)
			}
			d, err := pier.PrepareDeletion(path, "")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.BeginPierDeletion(ctx, ship.ID, path, d.Identity()); err != nil {
				t.Fatal(err)
			}
			if alreadyRemoved {
				if err = d.Remove(); err != nil {
					t.Fatal(err)
				}
			}
			d.Close()
			s.Close()
			s, err = store.Open(db)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			e := New(s, supervisor.New(s, nil))
			if err = e.supervisor.Observe(ctx); err != nil {
				t.Fatal(err)
			}
			if err = e.Reconcile(ctx); err != nil {
				t.Fatal(err)
			}
			e.SetReady(true)
			held, err := s.GetShip(ctx, ship.ID)
			if err != nil || !held.RecoveryBlocked || held.DesiredRunning || held.PierDeletionIdentity == "" {
				t.Fatalf("lost deletion hold: %+v %v", held, err)
			}
			for _, operation := range []model.Operation{"start", "backup"} {
				if _, err = e.Submit(ctx, model.JobRequest{ShipID: ship.ID, Kind: operation, Trigger: "manual"}); err == nil {
					t.Fatalf("accepted %s during deletion", operation)
				}
			}
			if err = e.VerifyAndReleaseHold(ctx, ship.ID); err == nil {
				t.Fatal("recovery verification released a deletion hold")
			}
			if err = e.RemoveShip(ctx, ship.ID); err == nil {
				t.Fatal("ordinary removal bypassed deletion hold")
			}
			if err = e.DeleteShip(ctx, ship.ID, path); err != nil {
				t.Fatal(err)
			}
			if _, err = s.GetShip(ctx, ship.ID); !errors.Is(err, store.ErrNotFound) {
				t.Fatalf("registration remains: %v", err)
			}
			if _, err = os.Lstat(path); !os.IsNotExist(err) {
				t.Fatalf("pier remains: %v", err)
			}
		})
	}
}

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
