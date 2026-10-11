package installation

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func fixture(t *testing.T, platform string) (*Manager, string) {
	t.Helper()
	home := t.TempDir()
	p := locations(platform, home, filepath.Join(home, "settings"))
	source := filepath.Join(home, "downloaded harbor")
	if err := os.WriteFile(source, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	m := &Manager{Paths: p, Platform: platform, Source: source, UID: "501", Username: "tester"}
	m.Run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name == "systemctl" && strings.Contains(strings.Join(args, " "), "FragmentPath") {
			return nil, nil
		}
		if name == "systemctl" && strings.Contains(strings.Join(args, " "), "MainPID") {
			return []byte(strconv.Itoa(os.Getpid())), nil
		}
		if name == "launchctl" && args[0] == "print" {
			return nil, os.ErrNotExist
		}
		return []byte("yes\n"), nil
	}
	return m, home
}

func TestInstallCreatesDefaultsAndPreservesDataOnRemoval(t *testing.T) {
	for _, platform := range []string{"darwin", "linux"} {
		t.Run(platform, func(t *testing.T) {
			m, home := fixture(t, platform)
			if err := m.Install(context.Background(), Options{NoStart: true}); err != nil {
				t.Fatal(err)
			}
			cfg, err := m.configuration()
			if err != nil {
				t.Fatal(err)
			}
			if cfg.DataDir != filepath.Join(home, "settings", "Harbor") {
				t.Fatalf("unexpected data directory: %s", cfg.DataDir)
			}
			keep := filepath.Join(cfg.DataDir, "ship-data")
			if err := os.WriteFile(keep, []byte("private pier"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := m.Uninstall(context.Background(), ""); err != nil {
				t.Fatal(err)
			}
			if got, err := os.ReadFile(keep); err != nil || string(got) != "private pier" {
				t.Fatalf("ship data changed: %q %v", got, err)
			}
			if _, err := os.Stat(m.Paths.Config); err != nil {
				t.Fatal("configuration was removed", err)
			}
			for _, path := range []string{m.Paths.Binary, m.Paths.Record, m.Paths.Unit} {
				if _, err := os.Lstat(path); !os.IsNotExist(err) {
					t.Fatalf("installed file remains: %s: %v", path, err)
				}
			}
		})
	}
}

func TestInstallRefusesForeignRegistrationAndSymlink(t *testing.T) {
	for _, symlink := range []bool{false, true} {
		m, home := fixture(t, "linux")
		os.MkdirAll(filepath.Dir(m.Paths.Unit), 0700)
		original := []byte("operator service")
		if symlink {
			target := filepath.Join(home, "operator-file")
			os.WriteFile(target, original, 0600)
			os.Symlink(target, m.Paths.Unit)
		} else {
			os.WriteFile(m.Paths.Unit, original, 0600)
		}
		if err := m.Install(context.Background(), Options{NoStart: true}); err == nil {
			t.Fatal("foreign registration accepted")
		}
		if got, _ := os.ReadFile(m.Paths.Unit); string(got) != string(original) {
			t.Fatal("foreign registration changed")
		}
		if _, err := os.Stat(m.Paths.Binary); !os.IsNotExist(err) {
			t.Fatal("binary copied before checking registration")
		}
	}
}

func TestUpgradeNeedsExplicitFlagAndPreservesStoppedState(t *testing.T) {
	m, _ := fixture(t, "linux")
	if err := m.Install(context.Background(), Options{NoStart: true}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(m.Paths.Binary)
	os.WriteFile(m.Source, []byte("#!/bin/sh\nexit 1\n"), 0700)
	if err := m.Install(context.Background(), Options{}); err == nil {
		t.Fatal("unrequested upgrade accepted")
	}
	if got, _ := os.ReadFile(m.Paths.Binary); string(got) != string(before) {
		t.Fatal("binary changed without upgrade")
	}
	if err := m.Install(context.Background(), Options{Upgrade: true}); err != nil {
		t.Fatal(err)
	}
	r, err := m.readRecord()
	if err != nil || r.Enabled {
		t.Fatalf("upgrade enabled a stopped installation: %+v %v", r, err)
	}
	if got, _ := os.ReadFile(m.Paths.Binary); string(got) == string(before) {
		t.Fatal("upgrade did not replace runner")
	}
}

func TestFailedServiceStopPreservesInstalledFiles(t *testing.T) {
	m, _ := fixture(t, "linux")
	if err := m.Install(context.Background(), Options{NoStart: true}); err != nil {
		t.Fatal(err)
	}
	m.Run = func(context.Context, string, ...string) ([]byte, error) {
		return nil, errors.New("service manager unavailable")
	}
	if err := m.Uninstall(context.Background(), ""); err == nil {
		t.Fatal("uninstall ignored stop failure")
	}
	for _, path := range []string{m.Paths.Binary, m.Paths.Record, m.Paths.Unit} {
		if _, err := os.Stat(path); err != nil {
			t.Fatal("installed file removed after failed stop", err)
		}
	}
}

func TestReadinessRejectsOtherServerAndRedirects(t *testing.T) {
	for _, behavior := range []string{"wrong-directory", "wrong-process", "redirect", "valid"} {
		t.Run(behavior, func(t *testing.T) {
			m, _ := fixture(t, "linux")
			dataDir := filepath.Join(t.TempDir(), "data")
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if behavior == "redirect" {
					http.Redirect(w, r, "http://example.invalid/", 302)
					return
				}
				dir := dataDir
				if behavior == "wrong-directory" {
					dir += "-other"
				}
				pid := os.Getpid()
				if behavior == "wrong-process" {
					pid++
				}
				json.NewEncoder(w).Encode(map[string]any{"dataDir": dir, "platform": "linux/arm64", "managerPID": pid})
			}))
			defer srv.Close()
			if got := m.ready(context.Background(), srv.URL, dataDir); got != (behavior == "valid") {
				t.Fatalf("readiness for %s = %v", behavior, got)
			}
		})
	}
}

func TestSourceFilteredUninstallLeavesOtherInstallation(t *testing.T) {
	m, _ := fixture(t, "linux")
	if err := m.Install(context.Background(), Options{NoStart: true}); err != nil {
		t.Fatal(err)
	}
	if err := m.Uninstall(context.Background(), "/usr/bin/harbor"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(m.Paths.Binary); err != nil {
		t.Fatal("package removal touched another installation")
	}
	os.WriteFile(m.Paths.Config, []byte("invalid operator configuration"), 0600)
	if matches, err := m.FromSource("/usr/lib/harbor/harbor"); err != nil || matches {
		t.Fatal("ownership filtering depends on unrelated configuration", err)
	}
}

func TestInstallRefusesLoadedServiceFromAnotherLocation(t *testing.T) {
	m, _ := fixture(t, "linux")
	m.Run = func(context.Context, string, ...string) ([]byte, error) {
		return []byte("/operator/harbor.service\n"), nil
	}
	if err := m.Install(context.Background(), Options{NoStart: true}); err == nil {
		t.Fatal("installation accepted a loaded service owned elsewhere")
	}
	if _, err := os.Stat(m.Paths.Binary); !os.IsNotExist(err) {
		t.Fatal("foreign service was touched")
	}
}

func TestInstallRefusesLoadedServiceWhoseUnitWasRemoved(t *testing.T) {
	m, _ := fixture(t, "linux")
	m.Run = func(context.Context, string, ...string) ([]byte, error) { return []byte(m.Paths.Unit + "\n"), nil }
	if err := m.Install(context.Background(), Options{NoStart: true}); err == nil {
		t.Fatal("a surviving service was adopted without an installation record")
	}
}

func TestOfflinePackageUpgradeAndRemovalPreserveData(t *testing.T) {
	m, _ := fixture(t, "linux")
	if err := m.Install(context.Background(), Options{NoStart: true}); err != nil {
		t.Fatal(err)
	}
	r, _ := m.readRecord()
	r.Enabled = true
	if err := m.save(r); err != nil {
		t.Fatal(err)
	}
	wants := filepath.Join(filepath.Dir(m.Paths.Unit), "default.target.wants", m.Paths.Label+".service")
	os.MkdirAll(filepath.Dir(wants), 0700)
	os.Symlink(m.Paths.Unit, wants)
	m.Run = func(context.Context, string, ...string) ([]byte, error) { return nil, errNoSession }
	os.WriteFile(m.Source, []byte("#!/bin/sh\nexit 2\n"), 0700)
	if err := m.Install(context.Background(), Options{Upgrade: true}); err != nil {
		t.Fatal("offline update failed", err)
	}
	r, _ = m.readRecord()
	if !r.Enabled {
		t.Fatal("offline upgrade discarded the next-login startup preference")
	}
	if target, err := os.Readlink(wants); err != nil || target != m.Paths.Unit {
		t.Fatal("offline upgrade lost its enabled unit", err)
	}
	if err := m.Uninstall(context.Background(), ""); err != nil {
		t.Fatal("offline removal failed", err)
	}
	if _, err := os.Lstat(wants); !os.IsNotExist(err) {
		t.Fatal("offline removal left an enabled service link")
	}
	if _, err := os.Stat(m.Paths.Config); err != nil {
		t.Fatal("offline removal deleted configuration")
	}
}

func TestDefinitionEscapesPathsWithoutShell(t *testing.T) {
	m, _ := fixture(t, "linux")
	m.Paths.Binary = "/home/a b/$USER%here/harbor"
	unit := string(m.definition("/home/a b/config.json"))
	if !strings.Contains(unit, `"/home/a b/$$USER%%here/harbor" serve --config "/home/a b/config.json"`) {
		t.Fatal("unsafe systemd argument", unit)
	}
	if !strings.Contains(unit, "TimeoutStopSec=infinity") || !strings.Contains(unit, "KillMode=process") {
		t.Fatal("graceful shutdown policy lost")
	}
	m.Platform = "darwin"
	plist := string(m.definition("/home/a & b/config.json"))
	if !strings.Contains(plist, "a &amp; b") || !strings.Contains(plist, "<key>AbandonProcessGroup</key><true/>") {
		t.Fatal("unsafe launchd definition", plist)
	}
}
