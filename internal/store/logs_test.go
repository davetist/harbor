package store

import (
	"bytes"
	"context"
	"harbor/internal/model"
	"path/filepath"
	"testing"
)

func TestJobOutputIsBoundedSeparateAndSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []model.ID{"a", "b"} {
		if err = s.CreateJob(ctx, model.Job{ID: id, ShipID: "ship", Kind: "pack", Trigger: "manual", Phase: "queued"}); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.AppendJobLog(ctx, "a", bytes.Repeat([]byte("A"), 3<<20)); err != nil {
		t.Fatal(err)
	}
	s.AppendJobLog(ctx, "b", []byte("other operation"))
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a, err := s.JobLog(ctx, "a")
	if err != nil || len(a) > 2<<20 || !bytes.Equal(a, bytes.Repeat([]byte("A"), len(a))) {
		t.Fatal("unbounded or mixed output", err)
	}
	b, _ := s.JobLog(ctx, "b")
	if string(b) != "other operation" {
		t.Fatal("lost retained job output")
	}
}
