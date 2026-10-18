package pier

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDeletionStaysInsideConfirmedDirectory(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(parent, "pier")
	if err := os.MkdirAll(filepath.Join(path, ".urb", "log"), 0700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "keep")
	if err := os.WriteFile(outside, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(outside), filepath.Join(path, "mounted-desk")); err != nil {
		t.Fatal(err)
	}
	d, err := PrepareDeletion(path, "")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err = d.Remove(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("pier remains: %v", err)
	}
	if b, err := os.ReadFile(outside); err != nil || string(b) != "keep" {
		t.Fatalf("symlink target changed: %v", err)
	}
}

func TestDeletionRejectsReplacedFolder(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(parent, "pier")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	d, err := PrepareDeletion(path, "")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	identity := d.Identity()
	if err = os.Rename(path, path+"-old"); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(path, "keep"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = d.Remove(); err == nil {
		t.Fatal("replacement was accepted")
	}
	if next, err := PrepareDeletion(path, identity); err == nil {
		next.Close()
		t.Fatal("retry adopted a different directory")
	}
	if _, err = os.Stat(filepath.Join(path, "keep")); err != nil {
		t.Fatal("replacement data was removed", err)
	}
}

func TestDeletionRefusesLinkedPierAndExistingWriter(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(parent, "pier")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	if d, err := PrepareDeletion(alias, ""); err == nil {
		d.Close()
		t.Fatal("linked pier was accepted")
	}
	unblock, err := ExcludeWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	defer unblock()
	if d, err := PrepareDeletion(path, ""); err == nil {
		d.Close()
		t.Fatal("pier with an existing read/write lock was accepted")
	}
}

func TestDeletionRetryAcceptsPartiallyRemovedSnapshot(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(parent, "pier")
	chk := filepath.Join(path, ".urb", "chk")
	if err = os.MkdirAll(chk, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(chk, "image.bin"), []byte("snapshot"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink("image.bin", filepath.Join(chk, "north.bin")); err != nil {
		t.Fatal(err)
	}
	d, err := PrepareDeletion(path, "")
	if err != nil {
		t.Fatal(err)
	}
	identity := d.Identity()
	d.Close()
	if err = os.Remove(filepath.Join(chk, "image.bin")); err != nil {
		t.Fatal(err)
	}
	d, err = PrepareDeletion(path, identity)
	if err != nil {
		t.Fatal("partial snapshot prevented authorized retry", err)
	}
	defer d.Close()
	if err = d.Remove(); err != nil {
		t.Fatal(err)
	}
}
