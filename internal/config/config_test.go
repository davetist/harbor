package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultConfigSupportsLANAndTailscale(t *testing.T) {
	c, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if c.ListenAddress != "0.0.0.0:18789" || c.DefaultShipHTTPBindAddress != "0.0.0.0" {
		t.Fatalf("network defaults: %+v", c)
	}
}

func TestExplicitLoopbackConfigPreserved(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(p, []byte(`{"listenAddress":"127.0.0.1:8789","defaultShipHTTPBindAddress":"127.0.0.1"}`), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil || c.ListenAddress != "127.0.0.1:8789" || c.DefaultShipHTTPBindAddress != "127.0.0.1" {
		t.Fatalf("explicit configuration changed: %+v %v", c, err)
	}
}
func TestInvalidConfigRejected(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	for _, content := range []string{`{"timezone":"nonsense"}`, `{"unknown":true}`, `{"listenAddress":"invalid"}`} {
		if err := os.WriteFile(p, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(p); err == nil {
			t.Fatalf("accepted %s", content)
		}
	}
}

func TestHostTimezoneDefaultAndExplicitOverride(t *testing.T) {
	t.Setenv("TZ", "Europe/Madrid")
	c, err := Load("")
	if err != nil || c.Timezone != "Europe/Madrid" {
		t.Fatal("host zone ignored", err)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(`{"timezone":"America/New_York"}`), 0600)
	c, err = Load(path)
	if err != nil || c.Timezone != "America/New_York" {
		t.Fatal("explicit zone ignored", err)
	}
}

func TestExternalStorageRequiresPinnedVolume(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	for _, data := range []string{"/mnt/harbor/harbor-data", "/media/operator/Harbor/data", "/run/media/operator/Harbor/data", "/Volumes/Harbor/data"} {
		content := `{"dataDir":"` + data + `"}`
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "requiredVolumes") {
			t.Fatalf("unprotected external path accepted: %s (%v)", data, err)
		}
	}
}

func TestPinnedVolumeConfigAcceptedWithoutCreatingFolders(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	content := `{"dataDir":"/mnt/harbor/harbor-data","requiredVolumes":[{"mountPoint":"/mnt/harbor","uuid":"11111111-2222-3333-4444-555555555555"}]}`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err != nil {
		t.Fatal("valid pinned configuration rejected", err)
	}
}
