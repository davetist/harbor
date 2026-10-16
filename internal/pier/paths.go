package pier

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func Canonical(path string, newDestination bool) (string, error) {
	if !filepath.IsAbs(path) {
		return "", errors.New("choose an absolute path on the Harbor host")
	}
	path = filepath.Clean(path)
	if newDestination {
		_, e := os.Lstat(path)
		if e == nil {
			return "", errors.New("destination already exists")
		}
		if !os.IsNotExist(e) {
			return "", e
		}
		parent, err := filepath.EvalSymlinks(filepath.Dir(path))
		if err != nil {
			return "", err
		}
		return filepath.Join(parent, filepath.Base(path)), nil
	}
	return filepath.EvalSymlinks(path)
}

type Inspection struct {
	Path        string
	HasSnapshot bool
	HasEvents   bool
}

// VerifyRegistered rejects path drift; it never adopts a new symlink target.
func VerifyRegistered(p string, newDestination bool) error {
	if !filepath.IsAbs(p) || filepath.Clean(p) != p {
		return errors.New("invalid registered pier path")
	}
	real, err := Canonical(p, newDestination)
	if err != nil {
		return err
	}
	if real != p {
		return errors.New("registered pier path changed")
	}
	if newDestination {
		return nil
	}
	if err := privateStorageDirectory(p); err != nil {
		return err
	}
	for _, name := range []string{".urb", ".urb/chk", ".urb/log", ".urb/put", ".vere.lock", ".http.ports"} {
		info, e := os.Lstat(filepath.Join(p, name))
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return e
		}
		if name == ".vere.lock" {
			if e = dedicatedLock(info); e != nil {
				return e
			}
		}
		target, e := filepath.EvalSymlinks(filepath.Join(p, name))
		if e != nil {
			return e
		}
		rel, e := filepath.Rel(p, target)
		if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return errors.New("pier storage path escapes registered folder")
		}
		if info.IsDir() || info.Mode()&os.ModeSymlink != 0 && name != ".http.ports" && name != ".vere.lock" {
			if e = privateStorageDirectory(filepath.Join(p, name)); e != nil {
				return e
			}
		}
	}
	return verifyNativeStorage(p)
}

// All launch/utility paths, including play and prep, call VerifyRegistered.
// Native storage safety must not depend on whether a utility accepts map flags.
func verifyNativeStorage(p string) error {
	for _, name := range []string{"image.bin", "north.bin", "south.bin"} {
		file := filepath.Join(p, ".urb/chk", name)
		if _, err := os.Lstat(file); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return err
		}
		target, err := trustedPath(file, false)
		rel, relErr := filepath.Rel(p, target)
		if err != nil || relErr != nil || rel == ".." || strings.HasPrefix(rel, "../") {
			return errors.New("snapshot target escapes pier or is unavailable")
		}
		if err := privateStorageDirectory(filepath.Dir(target)); err != nil {
			return err
		}
		info, err := os.Stat(target)
		if err != nil || !info.Mode().IsRegular() {
			return errors.New("snapshot must resolve to a regular file")
		}
	}
	log := filepath.Join(p, ".urb/log")
	if _, err := os.Lstat(log); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	if err := privateStorageDirectory(log); err != nil {
		return err
	}
	check := func(dir string) error {
		for _, name := range []string{"data.mdb", "lock.mdb"} {
			info, err := os.Lstat(filepath.Join(dir, name))
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return errors.New("native LMDB storage must be regular files without symlinks")
			}
			if name == "lock.mdb" {
				if err := dedicatedLock(info); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := check(log); err != nil {
		return err
	}
	f, err := os.Open(log)
	if err != nil {
		return err
	}
	defer f.Close()
	count := 0
	for {
		entries, readErr := f.ReadDir(128)
		for _, entry := range entries {
			count++
			if count > 10000 {
				return errors.New("event log entry limit exceeded")
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return errors.New("event log symlinks are not supported")
			}
			if entry.IsDir() {
				dir := filepath.Join(log, entry.Name())
				if err := privateStorageDirectory(dir); err != nil {
					return err
				}
				if err := check(dir); err != nil {
					return err
				}
			}
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

func Inspect(path string) (Inspection, error) {
	v := Inspection{Path: path}
	st, e := os.Stat(filepath.Join(path, ".urb"))
	if e != nil || !st.IsDir() {
		return v, errors.New("folder is not an Urbit pier")
	}
	for _, name := range []string{"chk/image.bin", "chk/north.bin", "chk/south.bin"} {
		if st, e := os.Stat(filepath.Join(path, ".urb", name)); e == nil && st.Mode().IsRegular() {
			v.HasSnapshot = true
		}
	}
	for _, name := range []string{"log", "put"} {
		if st, e := os.Stat(filepath.Join(path, ".urb", name)); e == nil && st.IsDir() {
			v.HasEvents = true
		}
	}
	if !v.HasSnapshot || !v.HasEvents {
		return v, fmt.Errorf("pier is missing snapshots or event storage")
	}
	return v, nil
}
