package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemovalRefusesPierAndDescendantMounts(t *testing.T) {
	for _, tc := range []struct {
		mount  string
		refuse bool
	}{
		{"/storage", false}, {"/storage/pier", true}, {"/storage/pier/desk", true}, {"/storage/pier-other", false},
	} {
		t.Run(tc.mount, func(t *testing.T) {
			err := refuseMountsWithin("/storage/pier", []string{tc.mount})
			if (err != nil) != tc.refuse {
				t.Fatalf("mount %q: %v", tc.mount, err)
			}
		})
	}
}

func TestContainsPathRecognizesFilesystemAliases(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(parent, "Pier")
	nested := filepath.Join(root, "nested")
	if err = os.MkdirAll(nested, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(parent, "link")
	if err = os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []string{filepath.Join(alias, "nested"), filepath.Join(alias, "not-created")} {
		inside, err := ContainsPath(root, candidate)
		if err != nil || !inside {
			t.Fatalf("symlink ancestry missed: %v %v", inside, err)
		}
	}
	caseAlias := filepath.Join(parent, "pier", "nested")
	if _, err := os.Stat(caseAlias); err == nil {
		inside, err := ContainsPath(root, caseAlias)
		if err != nil || !inside {
			t.Fatalf("case ancestry missed: %v %v", inside, err)
		}
		if err = refuseMountsWithin(root, []string{caseAlias}); err == nil {
			t.Fatal("case-aliased descendant mount accepted")
		}
	}
}

func TestRemovalChecksInaccessibleMountsFromTheirAccessibleAncestors(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pier := filepath.Join(parent, "pier")
	if err = os.Mkdir(pier, 0700); err != nil {
		t.Fatal(err)
	}
	for _, nested := range []bool{false, true} {
		root := parent
		if nested {
			root = pier
		}
		private := filepath.Join(root, "root-only")
		if err = os.MkdirAll(filepath.Join(private, "mount"), 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.Chmod(private, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(private, 0700) })
		err = refuseMountsWithin(pier, []string{filepath.Join(private, "mount")})
		if (err != nil) != nested {
			t.Fatalf("nested=%v: %v", nested, err)
		}
	}
}
