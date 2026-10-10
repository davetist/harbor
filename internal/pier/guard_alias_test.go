package pier

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGuardAliasesShareExistingAndVirginOwnership(t *testing.T) {
	root := t.TempDir()
	real, alias := filepath.Join(root, "data"), filepath.Join(root, "alias")
	if err := os.MkdirAll(filepath.Join(real, "piers"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, alias); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"", "piers/new"} {
		t.Run(suffix, func(t *testing.T) {
			release, err := Guard(filepath.Join(real, suffix))
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			other, err := Guard(filepath.Join(alias, suffix))
			if err == nil {
				other()
				t.Fatal("two aliases acquired the same ownership")
			}
		})
	}
}
