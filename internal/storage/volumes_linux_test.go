//go:build linux

package storage

import "testing"

func TestMountInfoHandlesKernelEscapesAndOptionalFields(t *testing.T) {
	b := []byte("26 1 179:2 / / rw,noatime shared:1 - ext4 /dev/mmcblk0p2 rw\n40 26 8:1 / /mnt/Harbor\\040ships rw shared:2 extra:value - ext4 /dev/sda1 rw\n")
	info, err := parseMountInfo(b)
	if err != nil || len(info) != 2 || info[1].MountPoint != "/mnt/Harbor ships" || info[1].Major != 8 || info[1].Minor != 1 {
		t.Fatal("mount metadata misread", info, err)
	}
	for _, b := range [][]byte{[]byte("bad mount line\n"), []byte("26 1 bad / / rw - ext4 /dev/sda1 rw\n")} {
		if _, err := parseMountInfo(b); err == nil {
			t.Fatal("malformed kernel metadata accepted")
		}
	}
}
