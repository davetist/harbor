package storage

import (
	"errors"
	"os"
	"path/filepath"
)

// ContainsPath checks physical ancestry as well as spelling. On macOS, case,
// Unicode and symlink aliases can name the same folder with different paths.
// Missing descendants are checked from their nearest existing ancestor.
func ContainsPath(directory, path string) (bool, error) {
	if !filepath.IsAbs(directory) || !filepath.IsAbs(path) {
		return false, errors.New("storage paths must be absolute")
	}
	if contains(directory, path) {
		return true, nil
	}
	root, err := os.Stat(directory)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	at := filepath.Clean(path)
	for {
		real, err := filepath.EvalSymlinks(at)
		if err == nil {
			at = real
			break
		}
		if !os.IsNotExist(err) {
			return false, err
		}
		parent := filepath.Dir(at)
		if parent == at {
			return false, err
		}
		at = parent
	}
	for {
		info, err := os.Stat(at)
		if err != nil {
			return false, err
		}
		if os.SameFile(root, info) {
			return true, nil
		}
		parent := filepath.Dir(at)
		if parent == at {
			return false, nil
		}
		at = parent
	}
}

// RefuseMountsWithin prevents recursive deletion from entering a mounted pier,
// a nested filesystem or a same-device bind mount.
func RefuseMountsWithin(path string) error {
	points, err := mountPoints()
	if err != nil {
		return err
	}
	return refuseMountsWithin(path, points)
}

func refuseMountsWithin(path string, points []string) error {
	for _, point := range points {
		inside, err := ContainsPath(path, point)
		if err != nil {
			return err
		}
		if inside {
			return errors.New("pier contains a mount point; unmount it before deleting pier files")
		}
	}
	return nil
}
