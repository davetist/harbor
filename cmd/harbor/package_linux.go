package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func maintainPackageUsers(passwd []byte, action string) error {
	var failures []error
	for _, entry := range strings.Split(string(passwd), "\n") {
		fields := strings.Split(entry, ":")
		if len(fields) != 7 || !filepath.IsAbs(fields[5]) {
			continue
		}
		uid, err := strconv.Atoi(fields[2])
		if err != nil || uid < 1000 || uid == 65534 {
			continue
		}
		if _, err := os.Lstat(packageRecord(fields[5])); os.IsNotExist(err) {
			continue
		}
		args := []string{"-u", fields[0], "--", "env", "HOME=" + fields[5], "XDG_RUNTIME_DIR=/run/user/" + fields[2], "DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/" + fields[2] + "/bus", "/usr/lib/harbor/harbor"}
		if action == "upgrade" {
			args = append(args, "install", "--upgrade", "--if-source", "/usr/lib/harbor/harbor")
		} else {
			args = append(args, "service", "uninstall", "--if-source", "/usr/lib/harbor/harbor")
		}
		cmd := exec.Command("runuser", args...)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			failures = append(failures, fmt.Errorf("Harbor installation for %s: %w", fields[0], err))
		}
	}
	return errors.Join(failures...)
}
