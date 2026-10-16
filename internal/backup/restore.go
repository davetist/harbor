package backup

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
	"strings"
)

// Never create through a link ancestor, including aliases recognized by the
// actual destination filesystem but not by the archive's exact-path manifest.
func restoreDirectories(root *os.Root, name string) error {
	if name == "." {
		return nil
	}
	current := ""
	for _, part := range strings.Split(name, "/") {
		if current != "" {
			current += "/"
		}
		current += part
		info, err := root.Lstat(current)
		if os.IsNotExist(err) {
			if err = root.Mkdir(current, 0700); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return errors.New("archive directory ancestor is a link or non-directory")
		}
	}
	return nil
}

func restoreCapacity(m Manifest, stat unix.Statfs_t) error {
	objects, err := archiveObjects(m.Files)
	if err != nil {
		return err
	}
	// Reserve directory/inode metadata and free-space headroom independently of
	// file payload: a forest of zero-byte files is not a zero-cost import.
	needed := uint64(64<<20) + uint64(len(objects))*4096
	for _, v := range m.Files {
		if v.Kind == "file" {
			if v.Size < 0 || uint64(v.Size) > ^uint64(0)-needed {
				return errors.New("archive capacity overflow")
			}
			needed += uint64(v.Size)
		}
	}
	if stat.Bsize <= 0 {
		return errors.New("cannot determine restore filesystem capacity")
	}
	blocks := needed / uint64(stat.Bsize)
	if needed%uint64(stat.Bsize) != 0 {
		blocks++
	}
	if blocks > uint64(stat.Bavail) || (stat.Files > 0 && uint64(len(objects))+128 > uint64(stat.Ffree)) {
		return errors.New("not enough free storage or inodes to restore full pier with metadata headroom")
	}
	return nil
}
