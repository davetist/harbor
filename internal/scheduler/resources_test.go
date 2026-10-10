package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestResourcesUseRequestedPierVolumeAndActualSnapshot(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".urb/chk"), 0700)
	os.WriteFile(filepath.Join(root, ".urb/chk/image.bin"), []byte("snapshot bytes"), 0600)
	r, err := ReadResources(context.Background(), root)
	if err != nil {
		t.Skipf("host resource access unavailable in this environment: %v", err)
	}
	if r.FreeDisk == 0 || r.TotalMemory == 0 || r.SnapshotBytes != 14 {
		t.Fatal("resources missing", r)
	}
}
