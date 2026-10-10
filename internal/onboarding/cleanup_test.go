package onboarding

import (
	"bytes"
	"harbor/internal/model"
	"os"
	"testing"
	"time"
)

func TestStartupCleanupPreservesReferencedKey(t *testing.T) {
	u := Uploads{Dir: t.TempDir()}
	id, err := u.StageKey(bytes.NewBufferString("0w123abc"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := u.KeyPath(id)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	old := now.Add(-48 * time.Hour)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	if err := u.CleanupReferenced(now, map[model.ID]bool{id: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatal("queued key removed")
	}
	if err := u.CleanupReferenced(now, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("abandoned key retained")
	}
}
