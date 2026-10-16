package pier

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"path/filepath"
	"sync"
	"syscall"
)

var lockMu sync.Mutex
var readLocks = map[string]os.FileInfo{}

func dedicatedLock(info os.FileInfo) error {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || !ok || st.Nlink != 1 || st.Uid != uint32(os.Getuid()) {
		return errors.New("lock must be a dedicated regular file owned by the Harbor user, without links")
	}
	return nil
}

func openLock(directory, name string, create bool) (*os.File, error) {
	r, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	if info, err := r.Lstat(name); err == nil {
		if err = dedicatedLock(info); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) || !create {
		return nil, err
	}
	flags := os.O_RDWR | syscall.O_NOFOLLOW | syscall.O_NONBLOCK
	if create {
		flags |= os.O_CREATE
	}
	f, err := r.OpenFile(name, flags, 0600)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err == nil {
		err = dedicatedLock(info)
	}
	named, namedErr := r.Lstat(name)
	if err != nil || namedErr != nil || !os.SameFile(info, named) {
		f.Close()
		return nil, errors.New("lock file identity changed or is unsafe")
	}
	return f, nil
}

func lockKey(path string) string {
	p := filepath.Join(path, ".vere.lock")
	if real, err := filepath.EvalSymlinks(p); err == nil {
		return real
	}
	return p
}

// A file named exactly like the resource inherits the filesystem's case and
// Unicode alias rules, including before the resource exists. Keep it in an
// owned namespace to avoid filename-length expansion or unrelated-file writes.
func guardNamespace(parent string) (string, error) {
	namespace := filepath.Join(parent, ".harbor-guards-v1")
	err := os.Mkdir(namespace, 0700)
	created := err == nil
	if err != nil && !os.IsExist(err) {
		return "", err
	}
	info, err := os.Lstat(namespace)
	if err != nil {
		return "", err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !info.IsDir() || !ok || st.Uid != uint32(os.Getuid()) || info.Mode().Perm() != 0700 {
		return "", errors.New("Harbor guard namespace must be an owned private directory without links")
	}
	const marker = "Harbor directory guards v1\n"
	if created {
		f, err := os.OpenFile(filepath.Join(namespace, "version"), os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW, 0600)
		if err != nil {
			return "", err
		}
		_, writeErr := f.WriteString(marker)
		closeErr := f.Close()
		if writeErr != nil || closeErr != nil {
			return "", errors.New("could not initialize Harbor directory guards")
		}
		if err := os.Mkdir(filepath.Join(namespace, "locks"), 0700); err != nil {
			return "", err
		}
	}
	f, err := openLock(namespace, "version", false)
	if err != nil {
		return "", err
	}
	content, err := io.ReadAll(io.LimitReader(f, 64))
	f.Close()
	if err != nil || string(content) != marker {
		return "", errors.New("existing guard namespace is not owned by Harbor")
	}
	locks := filepath.Join(namespace, "locks")
	info, err = os.Lstat(locks)
	if err != nil {
		return "", err
	}
	st, ok = info.Sys().(*syscall.Stat_t)
	if !info.IsDir() || !ok || st.Uid != uint32(os.Getuid()) || info.Mode().Perm() != 0700 {
		return "", errors.New("Harbor guard directory is unsafe")
	}
	return locks, nil
}

// WriterLocked observes Vere's one-byte POSIX record lock without taking over.
func WriterLocked(path string) (bool, error) {
	lockMu.Lock()
	defer lockMu.Unlock()
	// POSIX locks belong to the process: closing another descriptor for this
	// inode would release our export lock. Observe our own holds without opening.
	if info, err := os.Lstat(filepath.Join(path, ".vere.lock")); err == nil {
		if err := dedicatedLock(info); err != nil {
			return false, err
		}
		for _, held := range readLocks {
			if os.SameFile(info, held) {
				return true, nil
			}
		}
	}
	f, e := openLock(path, ".vere.lock", false)
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
		// Virgin piers do not exist yet; their chosen parent does.
		parent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
		if err != nil {
			return nil, err
		}
		path, e = filepath.Join(parent, filepath.Base(absolute)), nil
	}
	if e != nil {
		return nil, e
	}
	if e = TrustedDirectory(filepath.Dir(path)); e != nil {
		return nil, e
	}
	f, e := openLock(filepath.Dir(path), fmt.Sprintf(".harbor-%x.lock", sha256.Sum256([]byte(filepath.Clean(path)))), true)
	if e != nil {
		return nil, e
	}
	if e = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); e != nil {
		f.Close()
		return nil, e
	}
	// Retain the legacy lock for interoperability with existing runners, and
	// also lock a filesystem-equivalent name for case/Unicode aliases.
	namespace, e := guardNamespace(filepath.Dir(path))
	if e != nil {
		f.Close()
		return nil, e
	}
	alias, e := openLock(namespace, filepath.Base(path), true)
	if e != nil {
		f.Close()
		return nil, e
	}
	if e = unix.Flock(int(alias.Fd()), unix.LOCK_EX|unix.LOCK_NB); e != nil {
		alias.Close()
		f.Close()
		return nil, e
	}
	return func() { alias.Close(); f.Close() }, nil
}

// ExcludeWriter holds Vere's one-byte POSIX lock for a quiescent read. Vere
// requires an exclusive write lock on the same byte before changing the pier.
func ExcludeWriter(path string) (func(), error) {
	lockMu.Lock()
	defer lockMu.Unlock()
	if info, e := os.Lstat(filepath.Join(path, ".vere.lock")); e == nil {
		if e = dedicatedLock(info); e != nil {
			return nil, e
		}
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
	f, err := openLock(path, ".vere.lock", true)
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
