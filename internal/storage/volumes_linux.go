//go:build linux

package storage

import (
	"bufio"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

type linuxMount struct {
	MountPoint string
	Major      uint32
	Minor      uint32
}

func mountPoints() ([]string, error) {
	b, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return nil, err
	}
	mounts, err := parseMountInfo(b)
	if err != nil {
		return nil, err
	}
	points := make([]string, 0, len(mounts))
	for _, mount := range mounts {
		points = append(points, mount.MountPoint)
	}
	return points, nil
}

func parseMountInfo(b []byte) ([]linuxMount, error) {
	var mounts []linuxMount
	s := bufio.NewScanner(strings.NewReader(string(b)))
	s.Buffer(make([]byte, 4096), 1<<20)
	unescape := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	for s.Scan() {
		fields := strings.Fields(s.Text())
		if len(fields) < 10 {
			return nil, errors.New("invalid kernel mount metadata")
		}
		separator := -1
		for i := 6; i < len(fields); i++ {
			if fields[i] == "-" {
				separator = i
				break
			}
		}
		device := strings.Split(fields[2], ":")
		if len(device) != 2 || separator < 6 || len(fields)-separator < 4 {
			return nil, errors.New("invalid kernel mount metadata")
		}
		major, e1 := strconv.ParseUint(device[0], 10, 32)
		minor, e2 := strconv.ParseUint(device[1], 10, 32)
		if e1 != nil || e2 != nil {
			return nil, errors.New("invalid kernel filesystem device")
		}
		mounts = append(mounts, linuxMount{unescape.Replace(fields[4]), uint32(major), uint32(minor)})
	}
	return mounts, s.Err()
}

func lookupVolume(ctx context.Context, point string) (volumeInfo, error) {
	if err := ctx.Err(); err != nil {
		return volumeInfo{}, err
	}
	b, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return volumeInfo{}, err
	}
	mounts, err := parseMountInfo(b)
	if err != nil {
		return volumeInfo{}, err
	}
	st, err := os.Stat(point)
	if err != nil {
		return volumeInfo{}, err
	}
	device := uint64(st.Sys().(*syscall.Stat_t).Dev)
	var mounted *linuxMount
	for i := range mounts {
		m := &mounts[i]
		if m.MountPoint == point && m.Major == unix.Major(device) && m.Minor == unix.Minor(device) {
			mounted = m
		}
	}
	if mounted == nil {
		return volumeInfo{}, nil // Existing directory on the parent FS is not a mount.
	}
	aliases, err := os.ReadDir("/dev/disk/by-uuid")
	if err != nil {
		return volumeInfo{}, errors.New("filesystem UUID metadata is unavailable")
	}
	for _, alias := range aliases {
		if err := ctx.Err(); err != nil {
			return volumeInfo{}, err
		}
		st, e := os.Stat(filepath.Join("/dev/disk/by-uuid", alias.Name()))
		if e != nil || st.Mode()&os.ModeDevice == 0 || st.Mode()&os.ModeCharDevice != 0 {
			continue
		}
		block := uint64(st.Sys().(*syscall.Stat_t).Rdev)
		if unix.Major(block) == mounted.Major && unix.Minor(block) == mounted.Minor {
			return volumeInfo{point, alias.Name()}, nil
		}
	}
	return volumeInfo{}, errors.New("mounted filesystem has no verifiable UUID")
}
