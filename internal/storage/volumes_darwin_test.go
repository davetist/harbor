//go:build darwin

package storage

import "testing"

func TestDiskInfoUsesTopLevelFilesystemIdentity(t *testing.T) {
	b := []byte(`<plist><dict><key>Nested</key><dict><key>VolumeUUID</key><string>wrong</string></dict><key>VolumeUUID</key><string>` + testUUID + `</string><key>MountPoint</key><string>/Volumes/Harbor &amp; ships</string><key>Writable</key><true/></dict></plist>`)
	info, err := parseDiskInfo(b)
	if err != nil || info.UUID != testUUID || info.MountPoint != "/Volumes/Harbor & ships" {
		t.Fatal("wrong volume identity", info, err)
	}
	for _, b := range [][]byte{[]byte(`<plist><dict><key>VolumeUUID</key><integer>123</integer></dict></plist>`), []byte(`<plist><dict><key>MountPoint</key></dict></plist>`)} {
		if _, err := parseDiskInfo(b); err == nil {
			t.Fatal("invalid metadata accepted")
		}
	}
}
