package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
)

type Volume struct {
	MountPoint string `json:"mountPoint"`
	UUID       string `json:"uuid"`
}

type volumeInfo struct{ MountPoint, UUID string }
type volumeLookup func(context.Context, string) (volumeInfo, error)

var uuidFormat = regexp.MustCompile(`^[0-9a-fA-F][0-9a-fA-F-]{1,127}$`)

func contains(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel)
}

// ValidateConfiguration is read-only. Conventional removable-storage paths
// require an explicit pin; other mount locations can be pinned the same way.
func ValidateConfiguration(volumes []Volume, paths []string) error {
	seen := map[string]bool{}
	for _, v := range volumes {
		if !filepath.IsAbs(v.MountPoint) || filepath.Clean(v.MountPoint) != v.MountPoint || !uuidFormat.MatchString(v.UUID) {
			return errors.New("requiredVolumes needs an absolute mountPoint and a filesystem UUID")
		}
		if seen[v.MountPoint] {
			return errors.New("requiredVolumes contains a duplicate mountPoint")
		}
		seen[v.MountPoint] = true
		used := false
		for _, path := range paths {
			used = used || contains(v.MountPoint, path)
		}
		if !used {
			return fmt.Errorf("required volume %s does not contain dataDir or an allowedRoot", v.MountPoint)
		}
	}
	for _, path := range paths {
		for _, base := range []string{"/mnt", "/media", "/run/media", "/Volumes"} {
			if !contains(base, path) {
				continue
			}
			covered := false
			for _, v := range volumes {
				covered = covered || (contains(base, v.MountPoint) && contains(v.MountPoint, path))
			}
			if !covered {
				return fmt.Errorf("external storage path %s requires a requiredVolumes entry with its mountPoint and filesystem UUID", path)
			}
		}
	}
	return nil
}

// Guard holds directory descriptors until Harbor stops. It never creates,
// mounts, formats or repairs storage. Forced removal is outside this startup
// check; the OS may still return I/O failures after a physical disconnect.
type Guard struct{ directories []*os.File }

func (g *Guard) Close() {
	for _, f := range g.directories {
		f.Close()
	}
	g.directories = nil
}

func Acquire(ctx context.Context, volumes []Volume, paths []string) (*Guard, error) {
	return acquire(ctx, volumes, paths, lookupVolume)
}

func acquire(ctx context.Context, volumes []Volume, paths []string, lookup volumeLookup) (g *Guard, err error) {
	if err = ValidateConfiguration(volumes, paths); err != nil {
		return nil, err
	}
	g = &Guard{}
	defer func() {
		if err != nil {
			g.Close()
		}
	}()
	for _, v := range volumes {
		if err = ctx.Err(); err != nil {
			return g, err
		}
		real, e := filepath.EvalSymlinks(v.MountPoint)
		if e != nil || real != v.MountPoint {
			return g, fmt.Errorf("required volume %s is not mounted at its configured canonical path", v.MountPoint)
		}
		check := func() error {
			info, e := lookup(ctx, v.MountPoint)
			if e != nil {
				return fmt.Errorf("required volume %s could not be verified: %w", v.MountPoint, e)
			}
			if info.MountPoint != v.MountPoint {
				return fmt.Errorf("required volume %s is not mounted at the expected path", v.MountPoint)
			}
			if !strings.EqualFold(info.UUID, v.UUID) {
				return fmt.Errorf("required volume %s has a different filesystem UUID; startup refused", v.MountPoint)
			}
			return nil
		}
		if err = check(); err != nil {
			return g, err
		}
		f, e := os.Open(v.MountPoint)
		if e != nil {
			return g, fmt.Errorf("required volume %s could not be opened: %w", v.MountPoint, e)
		}
		g.directories = append(g.directories, f)
		opened, e := f.Stat()
		if e != nil || !opened.IsDir() {
			return g, fmt.Errorf("required volume %s is not an accessible directory", v.MountPoint)
		}
		if err = check(); err != nil {
			return g, err
		}
		current, e := os.Stat(v.MountPoint)
		if e != nil || !os.SameFile(opened, current) {
			return g, fmt.Errorf("required volume %s changed during startup", v.MountPoint)
		}
		for _, path := range paths {
			if !contains(v.MountPoint, path) {
				continue
			}
			// Validate the existing ancestor of a new destination, before mkdir.
			ancestor := path
			for {
				info, e := os.Stat(ancestor)
				if e == nil {
					real, e := filepath.EvalSymlinks(ancestor)
					if e != nil || !contains(v.MountPoint, real) || !info.IsDir() || info.Sys().(*syscall.Stat_t).Dev != opened.Sys().(*syscall.Stat_t).Dev {
						return g, fmt.Errorf("storage path %s is outside its required filesystem", path)
					}
					break
				}
				if !os.IsNotExist(e) || ancestor == v.MountPoint {
					return g, fmt.Errorf("storage path %s could not be verified", path)
				}
				ancestor = filepath.Dir(ancestor)
			}
		}
	}
	return g, nil
}
