package pier

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFolderSymlinkCannotEscapeAllowedRoots(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	os.Symlink(outside, filepath.Join(root, "escape"))
	if _, e := Canonical(filepath.Join(root, "escape"), []string{root}, false); e == nil {
		t.Fatal("escaped root")
	}
	if _, e := Canonical(filepath.Join(root, "new"), []string{root}, true); e != nil {
		t.Fatal(e)
	}
}
func TestExistingDestinationUnchanged(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "pier")
	os.Mkdir(p, 0700)
	os.WriteFile(filepath.Join(p, "keep"), []byte("original"), 0600)
	if _, e := Canonical(p, []string{root}, true); e == nil {
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
