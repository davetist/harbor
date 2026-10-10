package supervisor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReviewChangedPierSymlink(t *testing.T) {
	s, st, ship := testSupervisor(t)
	original := ship.PierPath
	outside := t.TempDir()
	if err := os.Rename(original, original+"-preserved"); err != nil {
		t.Fatal(err)
	}
	defer os.Rename(original+"-preserved", original)
	if err := os.Symlink(outside, original); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(original)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := s.Start(ctx, "review-job", ship.ID)
	defer s.Stop(context.Background(), ship.ID)
	if err == nil {
		got, _ := st.GetShip(ctx, ship.ID)
		t.Fatalf("launched swapped pier: original=%s stored=%s target=%s", filepath.Base(original), filepath.Base(got.PierPath), filepath.Base(outside))
	}
}
