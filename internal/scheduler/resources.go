package scheduler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// ReadResources checks the actual pier volume, including an external mount.
// Nothing is created and no swap or operating-system setting is changed.
func ReadResources(ctx context.Context, path string) (Resources, error) {
	r, err := hostResources(ctx)
	if err != nil {
		return r, err
	}
	if st, e := os.Stat(filepath.Join(path, ".urb/chk/image.bin")); e == nil {
		r.SnapshotBytes = uint64(st.Size())
	}
	volume := path
	for {
		if _, e := os.Stat(volume); e == nil {
			break
		}
		parent := filepath.Dir(volume)
		if parent == volume {
			return r, fmt.Errorf("storage volume unavailable")
		}
		volume = parent
	}
	var fs syscall.Statfs_t
	if err = syscall.Statfs(volume, &fs); err != nil {
		return r, err
	}
	r.FreeDisk = uint64(fs.Bavail) * uint64(fs.Bsize)
	return r, nil
}
