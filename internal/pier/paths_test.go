package pier

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFolderSymlinksResolveToChosenLocation(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "chosen")); err != nil {
		t.Fatal(err)
	}
	real, err := filepath.EvalSymlinks(outside)
	if err != nil {
		t.Fatal(err)
	}
	for _, newDestination := range []bool{false, true} {
		path, want := filepath.Join(root, "chosen"), real
		if newDestination {
			path, want = filepath.Join(path, "new"), filepath.Join(want, "new")
		}
		got, err := Canonical(path, newDestination)
		if err != nil || got != want {
			t.Fatalf("chosen symlink location: got %q, want %q (%v)", got, want, err)
		}
	}
}

func TestNewDestinationRejectsDanglingSymlink(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "pier")
	if err := os.Symlink(filepath.Join(root, "missing"), p); err != nil {
		t.Fatal(err)
	}
	if _, err := Canonical(p, true); err == nil {
		t.Fatal("existing dangling symlink accepted as a new destination")
	}
}
func TestExistingDestinationUnchanged(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "pier")
	os.Mkdir(p, 0700)
	os.WriteFile(filepath.Join(p, "keep"), []byte("original"), 0600)
	if _, e := Canonical(p, true); e == nil {
		t.Fatal("existing destination accepted")
	}
	b, _ := os.ReadFile(filepath.Join(p, "keep"))
	if string(b) != "original" {
		t.Fatal("destination changed")
	}
}

func TestModernSnapshotLayout(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".urb", "chk"), 0700)
	os.MkdirAll(filepath.Join(root, ".urb", "log"), 0700)
	os.WriteFile(filepath.Join(root, ".urb", "chk", "image.bin"), []byte("fixture"), 0600)
	if _, e := Inspect(root); e != nil {
		t.Fatal(e)
	}
}
