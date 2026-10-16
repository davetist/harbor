package jobs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func recoveryCopySource(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "pier")
	for _, dir := range []string{".urb/chk", ".urb/log/0i0", ".urb/log/0i100", ".urb/log/notes"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range []string{".urb/chk/image.bin", ".urb/log/data.mdb", ".urb/log/0i0/data.mdb", ".urb/log/0i100/epoc.txt", ".urb/log/lock.mdb", ".urb/log/0i0/lock.mdb", ".urb/log/notes/lock.mdb"} {
		if err := os.WriteFile(filepath.Join(root, file), []byte(file), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestRecoveryCopyRejectsDataAndTopologyChanges(t *testing.T) {
	for _, trial := range []struct {
		name   string
		mutate func(string) error
		allow  bool
	}{
		{"unchanged", func(string) error { return nil }, true},
		{"snapshot modified", func(root string) error {
			return os.WriteFile(filepath.Join(root, ".urb/chk/image.bin"), []byte("changed"), 0600)
		}, false},
		{"epoch discarded", func(root string) error { return os.RemoveAll(filepath.Join(root, ".urb/log/0i100")) }, false},
		{"empty epoch directory discarded", func(root string) error { return os.Remove(filepath.Join(root, ".urb/log/0i200")) }, false},
		{"epoch created", func(root string) error { return os.Mkdir(filepath.Join(root, ".urb/log/0i300"), 0700) }, false},
		{"root LMDB lock", func(root string) error {
			return os.WriteFile(filepath.Join(root, ".urb/log/lock.mdb"), []byte("reader bookkeeping"), 0600)
		}, true},
		{"epoch LMDB lock", func(root string) error {
			return os.WriteFile(filepath.Join(root, ".urb/log/0i0/lock.mdb"), []byte("reader bookkeeping"), 0600)
		}, true},
		{"unrelated lock filename", func(root string) error {
			return os.WriteFile(filepath.Join(root, ".urb/log/notes/lock.mdb"), []byte("changed"), 0600)
		}, false},
	} {
		t.Run(trial.name, func(t *testing.T) {
			ctx := context.Background()
			source := recoveryCopySource(t)
			if err := os.Mkdir(filepath.Join(source, ".urb/log/0i200"), 0700); err != nil {
				t.Fatal(err)
			}
			before, err := recoveryManifest(ctx, source, "")
			if err != nil {
				t.Fatal(err)
			}
			copy, err := newRecoveryCopy(ctx, source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { copy.close() })
			original, err := os.Stat(filepath.Join(source, ".urb/chk/image.bin"))
			if err != nil {
				t.Fatal(err)
			}
			duplicate, err := os.Stat(filepath.Join(copy.path, ".urb/chk/image.bin"))
			if err != nil || os.SameFile(original, duplicate) {
				t.Fatal("inspection shares snapshot storage with the original", err)
			}
			if err = trial.mutate(copy.path); err != nil {
				t.Fatal(err)
			}
			if err = copy.unchanged(ctx); (err == nil) != trial.allow {
				t.Fatalf("unexpected inspection outcome: %v", err)
			}
			originalCopy := &recoveryCopy{path: source, expected: before}
			if err = originalCopy.unchanged(ctx); err != nil {
				t.Fatal("inspector changed the original", err)
			}
			if err = copy.close(); err != nil {
				t.Fatal(err)
			}
			if _, err = os.Stat(copy.path); !os.IsNotExist(err) {
				t.Fatal("private inspection copy was not removed", err)
			}
		})
	}
}

func TestRecoveryCopyRefusesLinksAndCancellationWithoutLeavingCopies(t *testing.T) {
	for _, trial := range []string{"snapshot link", "directory link", "cancelled"} {
		t.Run(trial, func(t *testing.T) {
			source := recoveryCopySource(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch trial {
			case "snapshot link":
				if err := os.Symlink(t.TempDir(), filepath.Join(source, ".urb/chk/link")); err != nil {
					t.Fatal(err)
				}
			case "directory link":
				if err := os.Rename(filepath.Join(source, ".urb/chk"), filepath.Join(source, "snapshot")); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("../snapshot", filepath.Join(source, ".urb/chk")); err != nil {
					t.Fatal(err)
				}
			case "cancelled":
				cancel()
			}
			if copy, err := newRecoveryCopy(ctx, source); err == nil || copy != nil {
				t.Fatal("unsafe inspection copy was accepted", err)
			}
			leftovers, err := filepath.Glob(filepath.Join(filepath.Dir(source), ".harbor-inspect-*"))
			if err != nil || len(leftovers) != 0 {
				t.Fatal("failed preparation left private copies", leftovers, err)
			}
		})
	}
}

func TestRecoveryCopyRefusesUnrepresentableCapacity(t *testing.T) {
	if err := recoverySpace(t.TempDir(), ^uint64(0)); err == nil {
		t.Fatal("impossible copy capacity was accepted")
	}
}
