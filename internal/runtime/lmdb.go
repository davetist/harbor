package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
)

// Temporary POSIX collision mitigation; it does not reserve loom addresses.
// See docs/lmdb-mapping-workaround.md for evidence, utility coverage and the
// released-runtime validation required before changing or retiring this policy.
const minLMDBMapBytes int64 = 64 << 30

// LMDBMapArgs sizes the mapping on every ship launch and supported maintenance
// utility launch. Vere 4.6 can overwrite a live default-sized LMDB mapping when
// it maps the loom; choosing a smaller map mitigates, but does not fix, that bug.
// This is virtual address space, not allocated RAM or a limit on pier disk size.
//
// Check logical database lengths (including sparse files), not allocated disk
// blocks or a persisted historical map size. Epochs are independent environments;
// use the largest, not their sum. Keep at least half the selected map available
// for growth, rounding up to a power of two with a 64 GiB minimum. Refuse
// unchecked storage or unrepresentable sizes, without falling back to a map
// smaller than the data. This chooses a size at startup, not a live resize.
// Call after verifying the registered path and acquiring the Harbor pier guard.
func LMDBMapArgs(ctx context.Context, pierPath string, newPier bool) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var largest int64
	if newPier {
		if _, err := os.Lstat(pierPath); !os.IsNotExist(err) {
			return nil, errors.New("LMDB map check requires a new, absent pier destination")
		}
	} else {
		var err error
		largest, err = largestLMDBSize(ctx, pierPath)
		if err != nil {
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	mapBytes, err := lmdbMapSize(largest)
	if err != nil {
		return nil, err
	}
	return []string{"--lmdb-map-size", strconv.FormatInt(mapBytes, 10)}, nil
}

func lmdbMapSize(databaseBytes int64) (int64, error) {
	if databaseBytes < 0 {
		return 0, errors.New("LMDB database size is invalid")
	}
	size := minLMDBMapBytes
	for databaseBytes > size/2 {
		if size > math.MaxInt64/2 {
			return 0, errors.New("LMDB database is too large to represent a mapping with growth headroom")
		}
		size *= 2
	}
	return size, nil
}

func largestLMDBSize(ctx context.Context, pierPath string) (int64, error) {
	logPath := filepath.Join(pierPath, ".urb", "log")
	for _, path := range []string{pierPath, filepath.Join(pierPath, ".urb"), logPath} {
		if err := realLMDBDirectory(path); err != nil {
			return 0, err
		}
	}
	largest, err := lmdbFileSize(filepath.Join(logPath, "data.mdb"))
	if err != nil {
		return 0, err
	}
	dir, err := os.Open(logPath)
	if err != nil {
		return 0, fmt.Errorf("LMDB size check: %w", err)
	}
	defer dir.Close()
	// Only the log directory and its immediate epoch directories hold LMDB
	// environments. Read bounded batches, without traversing snapshot contents.
	entries := 0
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		batch, readErr := dir.ReadDir(128)
		for _, entry := range batch {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
			entries++
			if entries > 10000 {
				return 0, errors.New("LMDB size check: event log has too many entries to check safely")
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return 0, errors.New("LMDB size check: event log symlinks are not supported")
			}
			if entry.IsDir() {
				path := filepath.Join(logPath, entry.Name())
				if err := realLMDBDirectory(path); err != nil {
					return 0, err
				}
				size, err := lmdbFileSize(filepath.Join(path, "data.mdb"))
				if err != nil {
					return 0, err
				}
				largest = max(largest, size)
			}
		}
		if readErr == io.EOF {
			return largest, ctx.Err()
		}
		if readErr != nil {
			return 0, fmt.Errorf("LMDB size check: %w", readErr)
		}
	}
}

func realLMDBDirectory(path string) error {
	st, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("LMDB size check: %w", err)
	}
	if !st.IsDir() {
		return errors.New("LMDB size check requires real storage directories, without symlinks")
	}
	return nil
}

func lmdbFileSize(path string) (int64, error) {
	st, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("LMDB size check: %w", err)
	}
	if !st.Mode().IsRegular() {
		return 0, errors.New("LMDB size check requires regular database files, without symlinks")
	}
	return st.Size(), nil
}
