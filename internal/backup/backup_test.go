package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFullPierRoundTripHashes(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	source := filepath.Join(root, "source")
	os.MkdirAll(filepath.Join(source, ".urb", "chk"), 0700)
	os.WriteFile(filepath.Join(source, ".urb", "chk", "image.bin"), []byte("snapshot bytes"), 0600)
	os.MkdirAll(filepath.Join(source, ".urb", "log"), 0700)
	os.WriteFile(filepath.Join(source, ".urb", "log", "events"), []byte("every event"), 0600)
	os.Symlink("log/events", filepath.Join(source, ".urb", "events-link"))
	archive := filepath.Join(root, "backup.tar.gz")
	manifest, e := Export(ctx, source, archive)
	if e != nil {
		t.Fatal(e)
	}
	if len(manifest.Files) != 6 {
		t.Fatalf("incomplete tree: %d", len(manifest.Files))
	}
	if _, e = Validate(ctx, archive); e != nil {
		t.Fatal(e)
	}
	destination := filepath.Join(root, "restored")
	if _, e = Restore(ctx, archive, destination); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(filepath.Join(destination, ".urb", "events-link"))
	if e != nil || string(b) != "every event" {
		t.Fatalf("link/content lost: %v", e)
	}
	if _, e = Restore(ctx, archive, destination); e == nil {
		t.Fatal("overwrote existing pier")
	}
}
func TestTraversalAndSymlinkEscapeRejected(t *testing.T) {
	for _, h := range []*tar.Header{{Name: "../escape", Typeflag: tar.TypeReg, Size: 1}, {Name: "pier/escape", Typeflag: tar.TypeSymlink, Linkname: "../../outside"}, {Name: "pier/device", Typeflag: tar.TypeChar}} {
		t.Run(h.Name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "bad.tgz")
			f, _ := os.Create(file)
			gz := gzip.NewWriter(f)
			tw := tar.NewWriter(gz)
			tw.WriteHeader(h)
			if h.Size > 0 {
				tw.Write([]byte("x"))
			}
			tw.Close()
			gz.Close()
			f.Close()
			if _, e := Validate(context.Background(), file); e == nil {
				t.Fatal("unsafe archive accepted")
			}
		})
	}
}
func TestCancelledExportDoesNotPublish(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	os.Mkdir(source, 0700)
	os.WriteFile(filepath.Join(source, "keep"), []byte("keep"), 0600)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dest := filepath.Join(root, "backup.tgz")
	if _, e := Export(ctx, source, dest); e == nil {
		t.Fatal("cancelled export succeeded")
	}
	if _, e := os.Stat(dest); !os.IsNotExist(e) {
		t.Fatal("partial archive published")
	}
}
