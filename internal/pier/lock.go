package pier

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"sync"
	"syscall"
)

var lockMu sync.Mutex
var readLocks = map[string]os.FileInfo{}

func lockKey(path string) string {
	p := filepath.Join(path, ".vere.lock")
	if real, err := filepath.EvalSymlinks(p); err == nil {
		return real
	}
	return p
}

// WriterLocked observes Vere's one-byte POSIX record lock without taking over.
func WriterLocked(path string) (bool, error) {
	lockMu.Lock()
	defer lockMu.Unlock()
	// POSIX locks belong to the process: closing another descriptor for this
	// inode would release our export lock. Observe our own holds without opening.
	if info, err := os.Stat(filepath.Join(path, ".vere.lock")); err == nil {
		for _, held := range readLocks {
			if os.SameFile(info, held) {
				return true, nil
			}
		}
	}
	f, e := os.OpenFile(filepath.Join(path, ".vere.lock"), os.O_RDWR, 0)
	if os.IsNotExist(e) {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	defer f.Close()
	lock := unix.Flock_t{Type: unix.F_WRLCK, Whence: 0, Start: 0, Len: 1}
	if e = unix.FcntlFlock(f.Fd(), unix.F_GETLK, &lock); e != nil {
		return false, e
	}
	return lock.Type != unix.F_UNLCK, nil
}
func Guard(path string) (func(), error) {
	// Controller/reader ownership is tied to the physical path, including
	// aliases such as macOS /var and /private/var. Otherwise an offline cleanup
	// and a live service can take different locks for the same data directory.
	absolute, e := filepath.Abs(path)
	if e != nil {
		return nil, e
	}
	path, e = filepath.EvalSymlinks(absolute)
	if os.IsNotExist(e) {
		// Virgin piers do not exist yet; their permitted parent does.
		parent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
		if err != nil {
			return nil, err
		}
		path, e = filepath.Join(parent, filepath.Base(absolute)), nil
	}
	if e != nil {
		return nil, e
	}
	f, e := os.OpenFile(filepath.Join(filepath.Dir(path), fmt.Sprintf(".harbor-%x.lock", sha256.Sum256([]byte(filepath.Clean(path))))), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); e != nil {
		f.Close()
		return nil, e
	}
	return func() { unix.Flock(int(f.Fd()), unix.LOCK_UN); f.Close() }, nil
}

// ExcludeWriter holds Vere's one-byte POSIX lock for a quiescent read. Vere
// requires an exclusive write lock on the same byte before changing the pier.
func ExcludeWriter(path string) (func(), error) {
	lockMu.Lock()
	defer lockMu.Unlock()
	if info, e := os.Stat(filepath.Join(path, ".vere.lock")); e == nil {
		for _, held := range readLocks {
			if os.SameFile(info, held) {
				return nil, errors.New("pier is already being read for export")
			}
		}
	}
	key := lockKey(path)
	if _, held := readLocks[key]; held {
		return nil, errors.New("pier is already being read for export")
	}
	f, err := os.OpenFile(filepath.Join(path, ".vere.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok && stat.Nlink != 1 {
		f.Close()
		return nil, errors.New("pier lock file must not have hard links")
	}
	lock := unix.Flock_t{Type: unix.F_RDLCK, Whence: 0, Start: 0, Len: 1}
	if err = unix.FcntlFlock(f.Fd(), unix.F_SETLK, &lock); err != nil {
		f.Close()
		return nil, errors.New("stop the ship before exporting the pier")
	}
	readLocks[key] = info
	return func() { lockMu.Lock(); defer lockMu.Unlock(); f.Close(); delete(readLocks, key) }, nil
}
