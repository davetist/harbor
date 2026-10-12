package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackageMaintenanceUsesItsOwnSource(t *testing.T) {
	for _, format := range []string{"deb", "run"} {
		for _, uid := range []string{"501", "1000"} {
			t.Run(format+uid, func(t *testing.T) {
				tmp := t.TempDir()
				home := filepath.Join(tmp, "home")
				if err := os.MkdirAll(filepath.Dir(packageRecord(home)), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(packageRecord(home), []byte("{}"), 0600); err != nil {
					t.Fatal(err)
				}
				arguments := filepath.Join(tmp, "arguments")
				t.Setenv("HARBOR_TEST_ARGUMENTS", arguments)
				t.Setenv("PATH", tmp+":"+os.Getenv("PATH"))
				if err := os.WriteFile(filepath.Join(tmp, "runuser"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$HARBOR_TEST_ARGUMENTS\"\n"), 0700); err != nil {
					t.Fatal(err)
				}
				source := "/usr/lib/harbor/harbor"
				if format == "run" {
					source = "/usr/local/lib/harbor/harbor"
				}
				passwd := []byte("fixture:x:" + uid + ":1000::" + home + ":/bin/sh\n")
				for _, action := range []string{"upgrade", "remove"} {
					if err := maintainPackageUsers(passwd, action, source); err != nil {
						t.Fatal(err)
					}
					contents, err := os.ReadFile(arguments)
					if err != nil {
						t.Fatal(err)
					}
					args := strings.Split(strings.TrimSpace(string(contents)), "\n")
					if args[7] != source || args[len(args)-1] != source || args[len(args)-2] != "--if-source" {
						t.Fatalf("maintenance did not keep the source boundary: %v", args)
					}
					if action == "upgrade" {
						opts, err := parseInstallOptions(args[9:], "linux")
						if err != nil || opts.Headless || !opts.Upgrade {
							t.Fatalf("package upgrade must preserve lingering without prompting: %+v %v", opts, err)
						}
					}
				}
			})
		}
	}
}
