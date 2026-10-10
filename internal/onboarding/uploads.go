package onboarding

import (
	"errors"
	"harbor/internal/model"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type Uploads struct{ Dir string }

var uploadID = regexp.MustCompile(`^[0-9a-f]{32}$`)
var keyFormat = regexp.MustCompile(`^0w[0-9a-zA-Z.~\-]+$`)

func (u Uploads) StageKey(r io.Reader) (model.ID, error) {
	b, e := io.ReadAll(io.LimitReader(r, 65537))
	if e != nil {
		return "", errors.New("unable to read keyfile")
	}
	if len(b) > 65536 || !keyFormat.MatchString(strings.TrimSpace(string(b))) {
		return "", errors.New("keyfile must contain a valid @uw key no larger than 64 KiB")
	}
	if e = os.MkdirAll(u.Dir, 0700); e != nil {
		return "", e
	}
	id := model.NewID()
	f, e := os.OpenFile(filepath.Join(u.Dir, string(id)+".key"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return "", e
	}
	_, e = f.Write(b)
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e == nil {
		e = closeErr
	}
	if e != nil {
		os.Remove(f.Name())
		return "", e
	}
	return id, nil
}
func (u Uploads) KeyPath(id model.ID) (string, error) {
	if !uploadID.MatchString(string(id)) {
		return "", errors.New("invalid upload reference")
	}
	p := filepath.Join(u.Dir, string(id)+".key")
	st, e := os.Lstat(p)
	if e != nil || !st.Mode().IsRegular() {
		return "", errors.New("staged keyfile is unavailable")
	}
	return p, nil
}
func (u Uploads) Remove(id model.ID) error {
	p, e := u.KeyPath(id)
	if e != nil {
		return e
	}
	return os.Remove(p)
}
func (u Uploads) Cleanup(now time.Time) error {
	return u.CleanupReferenced(now, nil)
}
func (u Uploads) CleanupReferenced(now time.Time, protected map[model.ID]bool) error {
	entries, e := os.ReadDir(u.Dir)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	for _, entry := range entries {
		id := strings.TrimSuffix(entry.Name(), ".key")
		if entry.IsDir() || !uploadID.MatchString(id) || entry.Name() != id+".key" || protected[model.ID(id)] {
			continue
		}
		st, e := entry.Info()
		if e != nil {
			return e
		}
		if st.Mode().IsRegular() && now.Sub(st.ModTime()) > 24*time.Hour {
			if e = os.Remove(filepath.Join(u.Dir, entry.Name())); e != nil {
				return e
			}
		}
	}
	return nil
}

func (u Uploads) StageArchive(r io.Reader, limit int64) (model.ID, error) {
	if e := os.MkdirAll(u.Dir, 0700); e != nil {
		return "", e
	}
	id := model.NewID()
	p := filepath.Join(u.Dir, string(id)+".tar.gz")
	f, e := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return "", e
	}
	n, e := io.Copy(f, io.LimitReader(r, limit+1))
	if e == nil {
		e = f.Sync()
	}
	f.Close()
	if e != nil || n > limit || n == 0 {
		os.Remove(p)
		return "", errors.New("archive upload was interrupted, empty, or exceeded the configured limit")
	}
	return id, nil
}
func (u Uploads) ArchivePath(id model.ID) (string, error) {
	if !uploadID.MatchString(string(id)) {
		return "", errors.New("invalid archive reference")
	}
	p := filepath.Join(u.Dir, string(id)+".tar.gz")
	st, e := os.Lstat(p)
	if e != nil || !st.Mode().IsRegular() {
		return "", errors.New("uploaded archive is unavailable")
	}
	return p, nil
}
