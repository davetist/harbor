package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"golang.org/x/sys/unix"
	"harbor/internal/pier"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Vere info can migrate snapshots or discard an incomplete latest epoch. It
// must inspect an independent copy, never the held pier. Callers hold both the
// Harbor guard and Vere's read lock while copying and checking the result.
type recoveryCopy struct {
	path     string
	expected map[string]string
	stage    *pier.TempDir
}

func newRecoveryCopy(ctx context.Context, source string) (*recoveryCopy, error) {
	stage, err := pier.NewTempDir(filepath.Dir(source), ".harbor-inspect-")
	if err != nil {
		return nil, err
	}
	c := &recoveryCopy{path: stage.Path, stage: stage}
	c.expected, err = recoveryManifest(ctx, source, stage.Path)
	if err != nil {
		c.close()
		return nil, err
	}
	return c, nil
}

func (c *recoveryCopy) close() error { return c.stage.Close() }

func (c *recoveryCopy) unchanged(ctx context.Context) error {
	got, err := recoveryManifest(ctx, c.path, "")
	if err != nil || !maps.Equal(c.expected, got) {
		return errors.New("offline inspection changed its temporary event log or snapshot; original pier and hold remain")
	}
	return nil
}

// Hash every file and directory in the storage used by info. When destination
// is set, copy the same bytes into new private files, without hard links. LMDB
// lock.mdb is transient reader bookkeeping and is neither copied nor compared.
func recoveryManifest(ctx context.Context, source, destination string) (map[string]string, error) {
	root, err := os.OpenRoot(source)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	for _, name := range []string{".urb", ".urb/log", ".urb/chk"} {
		info, err := root.Lstat(name)
		if err != nil || !info.IsDir() {
			return nil, errors.New("inspection requires real snapshot and event-log directories")
		}
	}
	manifest := make(map[string]string)
	if destination != "" {
		// Copies may materialize sparse files. Reserve headroom using logical
		// lengths, not allocated blocks, before writing any snapshot/log bytes.
		needed := uint64(64 << 20)
		for _, storage := range []string{".urb/log", ".urb/chk"} {
			err = fs.WalkDir(root.FS(), storage, func(name string, entry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if err = ctx.Err(); err != nil {
					return err
				}
				if entry.IsDir() {
					return nil
				}
				if !entry.Type().IsRegular() {
					return errors.New("inspection storage contains a link or special file")
				}
				if inspectionLock(name) {
					return nil
				}
				info, err := entry.Info()
				if err != nil {
					return err
				}
				if info.Size() < 0 || uint64(info.Size()) > ^uint64(0)-needed {
					return errors.New("inspection storage size cannot be represented")
				}
				needed += uint64(info.Size())
				return nil
			})
			if err != nil {
				return nil, err
			}
		}
		if err = recoverySpace(destination, needed); err != nil {
			return nil, err
		}
	}
	for _, storage := range []string{".urb/log", ".urb/chk"} {
		err = fs.WalkDir(root.FS(), storage, func(name string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if err = ctx.Err(); err != nil {
				return err
			}
			if entry.IsDir() {
				manifest[name] = "directory"
				if destination != "" {
					return os.MkdirAll(filepath.Join(destination, name), 0700)
				}
				return nil
			}
			if !entry.Type().IsRegular() {
				return errors.New("inspection storage contains a link or special file")
			}
			if inspectionLock(name) {
				return nil
			}
			input, err := root.Open(name)
			if err != nil {
				return err
			}
			defer input.Close()
			hash := sha256.New()
			var output io.Writer = hash
			var file *os.File
			if destination != "" {
				file, err = os.OpenFile(filepath.Join(destination, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
				if err != nil {
					return err
				}
				defer file.Close()
				output = io.MultiWriter(hash, file)
			}
			if _, err = io.Copy(output, recoveryReader{ctx, input}); err != nil {
				return err
			}
			if file != nil {
				if err = file.Close(); err != nil {
					return err
				}
			}
			manifest[name] = hex.EncodeToString(hash.Sum(nil))
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return manifest, nil
}

func inspectionLock(name string) bool {
	if name == ".urb/log/lock.mdb" {
		return true
	}
	parts := strings.Split(name, "/")
	if len(parts) != 4 || parts[0] != ".urb" || parts[1] != "log" || parts[3] != "lock.mdb" || !strings.HasPrefix(parts[2], "0i") {
		return false
	}
	epoch, err := strconv.ParseUint(parts[2][2:], 10, 64)
	return err == nil && parts[2] == "0i"+strconv.FormatUint(epoch, 10)
}

func recoverySpace(path string, needed uint64) error {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return err
	}
	if stat.Bsize <= 0 {
		return errors.New("could not determine inspection filesystem capacity")
	}
	blocks := needed / uint64(stat.Bsize)
	if needed%uint64(stat.Bsize) != 0 {
		blocks++
	}
	if blocks > uint64(stat.Bavail) {
		return errors.New("insufficient free disk space for a temporary snapshot and event-log inspection copy")
	}
	return nil
}

type recoveryReader struct {
	ctx context.Context
	r   io.Reader
}

func (r recoveryReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}
