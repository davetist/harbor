package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMissingVolumeRefusesStartupBeforeCreatingData(t *testing.T) {
	base := t.TempDir()
	mount := filepath.Join(base, "absent-drive")
	data := filepath.Join(mount, "harbor-data")
	content, err := json.Marshal(map[string]any{
		"dataDir":         data,
		"requiredVolumes": []map[string]string{{"mountPoint": mount, "uuid": "11111111-2222-3333-4444-555555555555"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(base, "config.json")
	if err = os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	err = run([]string{"serve", "--config", path})
	if err == nil || !strings.Contains(err.Error(), "required volume") || !strings.Contains(err.Error(), "not mounted") {
		t.Fatalf("missing volume did not produce a mount refusal: %v", err)
	}
	if _, err = os.Lstat(mount); !os.IsNotExist(err) {
		t.Fatalf("startup wrote into the unmounted location: %v", err)
	}
}

func TestUnmountedDirectoryCannotReceiveDatabaseOrPierFolders(t *testing.T) {
	base := t.TempDir()
	mount, err := filepath.EvalSymlinks(base)
	if err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(mount, "harbor-data")
	content, _ := json.Marshal(map[string]any{
		"dataDir":         data,
		"requiredVolumes": []map[string]string{{"mountPoint": mount, "uuid": "11111111-2222-3333-4444-555555555555"}},
	})
	path := filepath.Join(base, "config.json")
	if err = os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	err = run([]string{"serve", "--config", path})
	if err == nil || !strings.Contains(err.Error(), "required volume") {
		t.Fatal("ordinary directory was accepted as a mounted drive", err)
	}
	if _, err = os.Lstat(data); !os.IsNotExist(err) {
		t.Fatal("startup created data on the underlying filesystem", err)
	}
}

func TestRejectedVolumeLeavesExistingDatabaseAndFoldersUntouched(t *testing.T) {
	point, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(point, "existing-data")
	if err = os.Mkdir(data, 0700); err != nil {
		t.Fatal(err)
	}
	original := []byte("existing private database must not be opened or replaced")
	database := filepath.Join(data, "harbor.sqlite")
	if err = os.WriteFile(database, original, 0600); err != nil {
		t.Fatal(err)
	}
	content, _ := json.Marshal(map[string]any{"dataDir": data, "requiredVolumes": []map[string]string{{"mountPoint": point, "uuid": "11111111-2222-3333-4444-555555555555"}}})
	configPath := filepath.Join(point, "config.json")
	if err = os.WriteFile(configPath, content, 0600); err != nil {
		t.Fatal(err)
	}
	err = run([]string{"serve", "--config", configPath})
	if err == nil || !strings.Contains(err.Error(), "required volume") {
		t.Fatal("volume check did not precede database open", err)
	}
	after, err := os.ReadFile(database)
	if err != nil || !bytes.Equal(after, original) {
		t.Fatal("existing database changed", err)
	}
	entries, err := os.ReadDir(data)
	if err != nil || len(entries) != 1 || entries[0].Name() != "harbor.sqlite" {
		t.Fatal("startup created pier folders or SQLite sidecars", entries, err)
	}
	entries, err = os.ReadDir(point)
	if err != nil || len(entries) != 2 {
		t.Fatal("startup created a controller lock before validation", entries, err)
	}
}
