package pier

import (
	"errors"
	"fmt"
	"harbor/internal/storage"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

// Deletion retains directory descriptors and writer exclusion until registration
// removal finishes. Its identity is persisted before deleting the first file.
type Deletion struct {
	path, identity string
	root, parent   *os.Root
	info           os.FileInfo
	unblock        func()
}

func PrepareDeletion(path, expectedIdentity string) (_ *Deletion, err error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || filepath.Dir(path) == path {
		return nil, errors.New("invalid pier deletion path")
	}
	if err = TrustedDirectory(filepath.Dir(path)); err != nil {
		return nil, err
	}
	parentPath, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil || parentPath != filepath.Dir(path) {
		return nil, errors.New("registered pier parent changed")
	}
	if err = storage.RefuseMountsWithin(path); err != nil {
		return nil, err
	}
	parent, err := os.OpenRoot(parentPath)
	if err != nil {
		return nil, err
	}
	d := &Deletion{path: path, parent: parent, identity: expectedIdentity}
	defer func() {
		if err != nil {
			d.Close()
		}
	}()
	_, err = parent.Lstat(filepath.Base(path))
	if os.IsNotExist(err) && expectedIdentity != "" {
		return d, nil // Files were deleted before an interrupted registry commit.
	}
	if err != nil {
		return nil, err
	}
	real, err := Canonical(path, false)
	if err != nil || real != path {
		return nil, errors.New("registered pier path changed")
	}
	// Deleting an already partial pier must not require intact snapshots or
	// symlink targets. Only the containing directory and lock need be safe.
	if err = privateStorageDirectory(path); err != nil {
		return nil, err
	}
	d.root, err = os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	d.info, err = d.root.Stat(".")
	if err != nil {
		return nil, err
	}
	st := d.info.Sys().(*syscall.Stat_t)
	d.identity = fmt.Sprintf("%d:%d", st.Dev, st.Ino)
	if expectedIdentity != "" && d.identity != expectedIdentity {
		return nil, errors.New("pier folder was replaced after deletion began; replacement preserved")
	}
	if err = d.unchanged(); err != nil {
		return nil, err
	}
	if locked, checkErr := WriterLocked(path); checkErr != nil || locked {
		return nil, errors.New("pier has a writer or reader; files were not deleted")
	}
	d.unblock, err = ExcludeWriter(path)
	if err != nil {
		return nil, err
	}
	return d, nil
}

func (d *Deletion) Identity() string { return d.identity }

func (d *Deletion) unchanged() error {
	named, err := d.parent.Lstat(filepath.Base(d.path))
	if d.root == nil && os.IsNotExist(err) {
		return nil
	}
	if err != nil || d.info == nil || !os.SameFile(d.info, named) {
		return errors.New("pier directory identity changed; replacement preserved")
	}
	return nil
}

func (d *Deletion) Remove() error {
	if err := d.unchanged(); err != nil {
		return err
	}
	if d.root == nil {
		return nil
	}
	if err := storage.RefuseMountsWithin(d.path); err != nil {
		return err
	}
	f, err := d.root.Open(".")
	if err != nil {
		return err
	}
	defer f.Close()
	for {
		entries, readErr := f.ReadDir(128)
		for _, entry := range entries {
			// Keep the exclusion inode named until all pier contents are gone.
			if entry.Name() == ".vere.lock" {
				continue
			}
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
	if err = d.unchanged(); err != nil {
		return err
	}
	if err = d.root.Remove(".vere.lock"); err != nil {
		return err
	}
	return d.parent.Remove(filepath.Base(d.path)) // Never recurse through the name.
}

func (d *Deletion) Close() {
	if d.unblock != nil {
		d.unblock()
	}
	if d.root != nil {
		d.root.Close()
	}
	if d.parent != nil {
		d.parent.Close()
	}
}
