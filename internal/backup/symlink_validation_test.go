package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// Cancels at a deterministic validation-work boundary, without timing races.
type validationContext struct {
	context.Context
	cancel      context.CancelFunc
	checks      int
	cancelAfter int
}

func (c *validationContext) Err() error {
	c.checks++
	if c.checks >= c.cancelAfter {
		c.cancel()
	}
	return c.Context.Err()
}

func manifestGraph(files []File) (Manifest, map[string]File) {
	observed := make(map[string]File, len(files))
	for _, file := range files {
		observed[file.Path] = file
	}
	return Manifest{Version: 1, Files: files}, observed
}

func symlinkChain(count int) []File {
	files := make([]File, 0, count+1)
	for i := 0; i < count; i++ {
		files = append(files, File{Path: fmt.Sprintf("link-%05d", i), Kind: "symlink", Link: fmt.Sprintf("link-%05d", i+1)})
	}
	return append(files, File{Path: fmt.Sprintf("link-%05d", count), Kind: "file"})
}

func TestManifestCancellationDuringSymlinkTraversal(t *testing.T) {
	for _, cancelAfter := range []int{8, 40} {
		t.Run(fmt.Sprintf("cancelAfter=%d", cancelAfter), func(t *testing.T) {
			m, observed := manifestGraph(symlinkChain(32))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			checked := &validationContext{Context: ctx, cancel: cancel, cancelAfter: cancelAfter}
			if err := validateManifest(checked, m, observed); !errors.Is(err, context.Canceled) {
				t.Fatalf("post-read traversal ignored cancellation: %v", err)
			}
		})
	}
}

func TestManifestSymlinkChainHasLinearWork(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprintf("reverse=%v", reverse), func(t *testing.T) {
			files := symlinkChain(20000)
			// Multiple roots share the same long suffix.
			files = append(files, File{Path: "shared-a", Kind: "symlink", Link: "link-00000"}, File{Path: "shared-b", Kind: "symlink", Link: "link-00000"})
			if reverse {
				slices.Reverse(files)
			}
			m, observed := manifestGraph(files)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			// A quadratic suffix walk exhausts this budget long before completion.
			checked := &validationContext{Context: ctx, cancel: cancel, cancelAfter: 8 * len(files)}
			if err := validateManifest(checked, m, observed); err != nil {
				t.Fatalf("valid chain exceeded linear validation budget: %v (%d checks)", err, checked.checks)
			}
		})
	}
}

func TestManifestSymlinkControlsRemain(t *testing.T) {
	for _, test := range []struct {
		name  string
		files []File
		want  string
	}{
		{"self cycle", []File{{Path: "a", Kind: "symlink", Link: "./a"}}, "cyclic archive symlink"},
		{"tail into cycle", []File{{Path: "tail", Kind: "symlink", Link: "a"}, {Path: "a", Kind: "symlink", Link: "b"}, {Path: "b", Kind: "symlink", Link: "a"}}, "cyclic archive symlink"},
		{"missing target", []File{{Path: "a", Kind: "symlink", Link: "absent"}}, "symlink target missing from archive"},
		{"symlink parent", []File{{Path: "a/child", Kind: "file"}, {Path: "a", Kind: "symlink", Link: "directory"}, {Path: "directory", Kind: "directory"}}, "archive writes through a symlink"},
	} {
		t.Run(test.name, func(t *testing.T) {
			m, observed := manifestGraph(test.files)
			if err := validateManifest(context.Background(), m, observed); err == nil || err.Error() != test.want {
				t.Fatalf("want %q, got %v", test.want, err)
			}
		})
	}
	// Even a cached target must still pass its own manifest integrity check.
	m, observed := manifestGraph([]File{{Path: "a", Kind: "symlink", Link: "leaf"}, {Path: "leaf", Kind: "file"}})
	m.Files[1].Mode = 0600
	if err := validateManifest(context.Background(), m, observed); err == nil || err.Error() != `archive integrity check failed for "leaf"` {
		t.Fatalf("cached target bypassed integrity: %v", err)
	}
}

func TestValidateAndRestoreRelativeSymlinkChains(t *testing.T) {
	data := []byte("preserved ship data")
	sum := sha256.Sum256(data)
	m, _ := manifestGraph([]File{
		{Path: "dir", Kind: "directory", Mode: 0700},
		{Path: "dir/a", Kind: "symlink", Link: ".././leaf", Mode: 0777},
		{Path: "b", Kind: "symlink", Link: "dir//a", Mode: 0777},
		{Path: "directory-link", Kind: "symlink", Link: "./dir", Mode: 0777},
		{Path: "leaf", Kind: "file", Mode: 0600, Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:])},
	})
	root := t.TempDir()
	archive := filepath.Join(root, "chain.tar.gz")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for _, file := range m.Files {
		kind := byte(tar.TypeReg)
		if file.Kind == "directory" {
			kind = tar.TypeDir
		} else if file.Kind == "symlink" {
			kind = tar.TypeSymlink
		}
		if err := tw.WriteHeader(&tar.Header{Name: "pier/" + file.Path, Typeflag: kind, Linkname: file.Link, Size: file.Size, Mode: int64(file.Mode)}); err != nil {
			t.Fatal(err)
		}
		if file.Kind == "file" {
			if _, err := tw.Write(data); err != nil {
				t.Fatal(err)
			}
		}
	}
	manifest, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := tw.WriteHeader(&tar.Header{Name: manifestName, Typeflag: tar.TypeReg, Size: int64(len(manifest)), Mode: 0600}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(manifest); err != nil {
		t.Fatal(err)
	}
	for _, close := range []func() error{tw.Close, gz.Close, f.Close} {
		if err := close(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Validate(context.Background(), archive); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "restored")
	if _, err := Restore(context.Background(), archive, destination); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(destination, "b"))
	if err != nil || string(got) != string(data) {
		t.Fatalf("chain lost data: %q, %v", got, err)
	}
	info, err := os.Stat(filepath.Join(destination, "directory-link"))
	if err != nil || !info.IsDir() {
		t.Fatalf("directory endpoint lost: %v", err)
	}
}

func BenchmarkManifestSymlinkChain(b *testing.B) {
	m, observed := manifestGraph(symlinkChain(2000))
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := validateManifest(context.Background(), m, observed); err != nil {
			b.Fatal(err)
		}
	}
}
