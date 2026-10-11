package installation

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Opt-in OS-service acceptance. Only a unique disposable service and registry
// are admitted; the ordinary Harbor service and existing ships are untouched.
func TestNativeServiceLifecycle(t *testing.T) {
	if os.Getenv("HARBOR_NATIVE_INSTALL_TEST") != "1" {
		t.Skip("opt-in native service test")
	}
	source := os.Getenv("HARBOR_INSTALL_BINARY")
	if !filepath.IsAbs(source) {
		t.Fatal("set HARBOR_INSTALL_BINARY to the rebuilt native runner")
	}
	m, err := New()
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	base, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	m.Paths = locations(m.Platform, tmp, filepath.Join(tmp, "config"))
	m.Paths.Label = "org.harbor.install-test-" + filepath.Base(filepath.Dir(tmp))
	if m.Platform == "linux" {
		m.Paths.Unit = filepath.Join(base, "systemd", "user", m.Paths.Label+".service")
	} else {
		m.Paths.Unit = filepath.Join(tmp, m.Paths.Label+".plist")
	}
	m.Source = source
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	if err = os.MkdirAll(filepath.Dir(m.Paths.Config), 0700); err != nil {
		t.Fatal(err)
	}
	cfg, _ := json.Marshal(map[string]string{"dataDir": filepath.Join(tmp, "data"), "listenAddress": address, "defaultShipHTTPBindAddress": "127.0.0.1"})
	if err = os.WriteFile(m.Paths.Config, cfg, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	t.Cleanup(func() {
		cleanup, done := context.WithTimeout(context.Background(), 90*time.Second)
		defer done()
		if err := m.Uninstall(cleanup, ""); err != nil {
			t.Errorf("disposable-service cleanup: %v", err)
		}
	})
	if err = m.Install(ctx, Options{Config: m.Paths.Config}); err != nil {
		t.Fatal(err)
	}
	status, err := m.Status(ctx)
	if err != nil || !status.Running || !status.Enabled {
		t.Fatalf("fresh install: %+v %v", status, err)
	}
	firstPID := m.servicePID(ctx)
	if firstPID < 1 {
		t.Fatal("no owned service PID")
	}
	marker := filepath.Join(status.DataDir, "retained-installation-test")
	if err = os.WriteFile(marker, []byte("keep ship data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = m.Install(ctx, Options{}); err != nil {
		t.Fatal(err)
	}
	if m.servicePID(ctx) != firstPID {
		t.Fatal("opening an installed runner restarted it")
	}
	if newer := os.Getenv("HARBOR_INSTALL_UPGRADE_BINARY"); newer != "" {
		m.Source = newer
		if err = m.Install(ctx, Options{Upgrade: true}); err != nil {
			t.Fatal(err)
		}
		if pid := m.servicePID(ctx); pid == firstPID || pid < 1 {
			t.Fatal("upgrade did not restart the installed runner")
		}
		if got, _ := os.ReadFile(marker); string(got) != "keep ship data" {
			t.Fatal("upgrade changed data")
		}
	}
	if err = m.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if status, err = m.Status(ctx); err != nil || status.Running || status.Enabled {
		t.Fatalf("stop: %+v %v", status, err)
	}
	if err = m.Install(ctx, Options{Upgrade: true}); err != nil {
		t.Fatal(err)
	}
	if status, _ = m.Status(ctx); status.Running || status.Enabled {
		t.Fatal("reinstallation restarted a deliberately stopped service")
	}
	if err = m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err = m.Uninstall(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(marker); string(got) != "keep ship data" {
		t.Fatal("uninstall changed data")
	}
	if got, _ := os.ReadFile(m.Paths.Config); !strings.Contains(string(got), address) {
		t.Fatal("uninstall changed configuration")
	}
	for _, file := range []string{m.Paths.Binary, m.Paths.Unit, m.Paths.Record} {
		if _, err := os.Lstat(file); !os.IsNotExist(err) {
			t.Fatalf("installed file remains: %s", file)
		}
	}
	t.Log("native install, idempotent launch, upgrade (when supplied), stop/start and removal passed; configuration and data retained")
}
