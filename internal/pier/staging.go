package pier

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// TrustedDirectory protects pathname consumers (including native Vere) against
// another OS user renaming an ancestor. Sticky shared temporary roots are safe
// for entries owned by this user; a non-sticky shared parent is not.
func TrustedDirectory(name string) error {
	_, err := trustedPath(name, true)
	return err
}

// Sticky ancestors can protect existing owned entries, but actual storage
// directories must also prevent insertion of an absent native control file.
func privateStorageDirectory(name string) error {
	resolved, err := trustedPath(name, true)
	if err != nil {
		return err
	}
	info, err := os.Lstat(resolved)
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0022 != 0 {
		return errors.New("native storage directories must prevent writes by other OS users, including sticky directories")
	}
	return nil
}

// Check every traversed directory, including intermediate symlink targets.
// Checking only EvalSymlinks' final result misses replaceable link chains.
func trustedPath(name string, directory bool) (string, error) {
	unsafe := errors.New("storage requires directory ancestry protected from other OS users; choose a private folder")
	if !filepath.IsAbs(name) {
		return "", unsafe
	}
	protected := func(info os.FileInfo) bool {
		st, ok := info.Sys().(*syscall.Stat_t)
		return info.IsDir() && ok && (st.Uid == 0 || st.Uid == uint32(os.Getuid())) && (info.Mode().Perm()&0022 == 0 || info.Mode()&os.ModeSticky != 0)
	}
	root, err := os.Lstat("/")
	if err != nil || !protected(root) {
		return "", unsafe
	}
	at := "/"
	parent := root
	parts := strings.Split(name, "/")
	links := 0
	for len(parts) > 0 {
		part := parts[0]
		parts = parts[1:]
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			at = filepath.Dir(at)
			parent, err = os.Lstat(at)
			if err != nil {
				return "", err
			}
			continue
		}
		next := filepath.Join(at, part)
		info, err := os.Lstat(next)
		if err != nil {
			return "", err
		}
		if parent.Mode()&os.ModeSticky != 0 {
			st, ok := info.Sys().(*syscall.Stat_t)
			if !ok || st.Uid != 0 && st.Uid != uint32(os.Getuid()) {
				return "", unsafe
			}
		}
		if info.Mode()&os.ModeSymlink != 0 {
			links++
			if links > 255 {
				return "", errors.New("storage symlink resolution limit exceeded")
			}
			target, err := os.Readlink(next)
			if err != nil {
				return "", err
			}
			if filepath.IsAbs(target) {
				at = "/"
				parent = root
			}
			parts = append(strings.Split(target, "/"), parts...)
			continue
		}
		if info.IsDir() {
			if !protected(info) {
				return "", unsafe
			}
		} else if directory || len(parts) > 0 {
			return "", unsafe
		}
		at = next
		parent = info
	}
	return at, nil
}

// TempDir retains the created directory, so cleanup cannot recursively remove
// a replacement at its former name. Keep is used only after successful publish.
type TempDir struct {
	Path         string
	root, parent *os.Root
	info         os.FileInfo
	keep, closed bool
}

func NewTempDir(parent, prefix string) (*TempDir, error) {
	if err := TrustedDirectory(parent); err != nil {
		return nil, err
	}
	p, err := os.MkdirTemp(parent, prefix)
	if err != nil {
		return nil, err
	}
	r, err := os.OpenRoot(p)
	if err != nil {
		os.Remove(p)
		return nil, err
	}
	pr, err := os.OpenRoot(parent)
	if err != nil {
		r.Close()
		os.Remove(p)
		return nil, err
	}
	info, err := r.Stat(".")
	named, namedErr := pr.Lstat(filepath.Base(p))
	if err != nil || namedErr != nil || !os.SameFile(info, named) {
		r.Close()
		pr.Close()
		return nil, errors.New("temporary directory identity changed")
	}
	return &TempDir{Path: p, root: r, parent: pr, info: info}, nil
}

func (d *TempDir) Keep() { d.keep = true }

func (d *TempDir) Close() error {
	if d.closed {
		return nil
	}
	d.closed = true
	defer d.root.Close()
	defer d.parent.Close()
	if d.keep {
		return nil
	}
	named, err := d.parent.Lstat(filepath.Base(d.Path))
	changed := err != nil || !os.SameFile(d.info, named)
	f, err := d.root.Open(".")
	if err != nil {
		return err
	}
	defer f.Close()
	for {
		entries, readErr := f.ReadDir(128)
		for _, entry := range entries {
			if err := d.root.RemoveAll(entry.Name()); err != nil {
				return err
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	if changed {
		return errors.New("temporary directory was replaced; replacement preserved")
	}
	// Nonrecursive removal cannot delete contents even if the name changes.
	return d.parent.Remove(filepath.Base(d.Path))
}
