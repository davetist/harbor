package pier

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestSecurityGuardRejectsSymlinkWithoutCreatingTarget(t *testing.T) {
	for _, virgin := range []bool{false, true} {
		t.Run(fmt.Sprint(virgin), func(t *testing.T) {
			parent, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			p := filepath.Join(parent, "pier")
			if !virgin {
				if err := os.Mkdir(p, 0700); err != nil {
					t.Fatal(err)
				}
			}
			target := filepath.Join(parent, "sentinel")
			guard := filepath.Join(parent, fmt.Sprintf(".harbor-%x.lock", sha256.Sum256([]byte(p))))
			if err := os.Symlink(target, guard); err != nil {
				t.Fatal(err)
			}
			if release, err := Guard(p); err == nil {
				release()
				t.Error("guard followed a symlink")
			}
			if _, err := os.Lstat(target); !os.IsNotExist(err) {
				t.Fatal("guard created the outside target", err)
			}
		})
	}
}

func TestSecurityNativeLockRejectsAliases(t *testing.T) {
	for _, kind := range []string{"contained", "dangling", "hardlink"} {
		t.Run(kind, func(t *testing.T) {
			p := t.TempDir()
			target := filepath.Join(p, "snapshot")
			if kind != "dangling" {
				if err := os.WriteFile(target, []byte("snapshot"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			lock := filepath.Join(p, ".vere.lock")
			var err error
			if kind == "hardlink" {
				err = os.Link(target, lock)
			} else {
				err = os.Symlink("snapshot", lock)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := WriterLocked(p); err == nil {
				t.Error("lock observer accepted an aliased lock")
			}
			if release, err := ExcludeWriter(p); err == nil {
				release()
				t.Error("writer exclusion accepted an aliased lock")
			}
			if kind == "dangling" {
				if _, err := os.Lstat(target); !os.IsNotExist(err) {
					t.Fatal("lock created dangling target", err)
				}
			}
		})
	}
}

func TestSecurityRegisteredStorageRejectsEscapingChildren(t *testing.T) {
	for _, name := range []string{".urb/log/lock.mdb", ".urb/log/0i0/lock.mdb", ".urb/chk/image.bin"} {
		t.Run(name, func(t *testing.T) {
			p, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(p, filepath.Dir(name)), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(p, ".urb/chk/north.bin"), nil, 0600); err != nil && name == ".urb/chk/image.bin" {
				t.Fatal(err)
			}
			outside := filepath.Join(t.TempDir(), "absent")
			if err := os.Symlink(outside, filepath.Join(p, name)); err != nil {
				t.Fatal(err)
			}
			if err := VerifyRegistered(p, false); err == nil {
				t.Fatal("unchecked native storage link accepted")
			}
			if _, err := os.Lstat(outside); !os.IsNotExist(err) {
				t.Fatal("preflight created outside storage", err)
			}
		})
	}
}

func TestSecurityRegisteredStoragePreservesContainedSnapshots(t *testing.T) {
	p, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(p, ".urb/chk"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "snapshot"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../snapshot", filepath.Join(p, ".urb/chk/image.bin")); err != nil {
		t.Fatal(err)
	}
	if err := VerifyRegistered(p, false); err != nil {
		t.Fatal("safe contained snapshot link rejected", err)
	}
	if err := os.Chmod(filepath.Join(p, ".urb"), 0777); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(filepath.Join(p, ".urb"), 0700)
	if err := VerifyRegistered(p, false); err == nil {
		t.Fatal("replaceable native storage ancestry accepted")
	}
}

func TestSecurityPrivateStagingRequiresProtectedAncestry(t *testing.T) {
	parent := t.TempDir()
	if err := os.Chmod(parent, 0777); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(parent, 0700)
	child := filepath.Join(parent, "private")
	if err := os.Mkdir(child, 0700); err != nil {
		t.Fatal(err)
	}
	if stage, err := NewTempDir(child, ".trial-"); err == nil {
		stage.Close()
		t.Fatal("checking only the immediate parent admitted a replaceable ancestor")
	}
	if err := os.Chmod(parent, os.ModeSticky|0777); err != nil {
		t.Fatal(err)
	}
	stage, err := NewTempDir(child, ".trial-")
	if err != nil {
		t.Fatal("sticky temporary ancestor should preserve owned entries", err)
	}
	if err := stage.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(stage.Path); !os.IsNotExist(err) {
		t.Fatal("ordinary staging cleanup left its directory", err)
	}
}

func TestSecuritySnapshotRejectsReplaceableIntermediateLinkTarget(t *testing.T) {
	p, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{".urb/chk", "links"} {
		if err := os.MkdirAll(filepath.Join(p, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(p, "snapshot"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../snapshot", filepath.Join(p, "links/hop")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../links/hop", filepath.Join(p, ".urb/chk/image.bin")); err != nil {
		t.Fatal(err)
	}
	if err := VerifyRegistered(p, false); err != nil {
		t.Fatal("ordinary protected contained link chain rejected", err)
	}
	if err := os.Chmod(filepath.Join(p, "links"), 0777); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(filepath.Join(p, "links"), 0700)
	if err := VerifyRegistered(p, false); err == nil {
		t.Fatal("only final target ancestry was checked, leaving a replaceable link hop")
	}
}

func TestSecurityGuardRejectsHardlinkedStorage(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(parent, "virgin")
	data := filepath.Join(parent, "data")
	if err := os.WriteFile(data, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	guard := filepath.Join(parent, fmt.Sprintf(".harbor-%x.lock", sha256.Sum256([]byte(p))))
	if err := os.Link(data, guard); err != nil {
		t.Fatal(err)
	}
	if release, err := Guard(p); err == nil {
		release()
		t.Fatal("guard accepted a data inode alias")
	}
	if b, err := os.ReadFile(data); err != nil || string(b) != "keep" {
		t.Fatal("refused guard changed data", err)
	}
}

func TestSecurityGuardFilesystemAliasesShareVirginAndExistingOwnership(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	probe := filepath.Join(parent, "case-probe")
	if err := os.WriteFile(probe, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(parent, "CASE-PROBE")); os.IsNotExist(err) {
		t.Skip("fixture requires a case-insensitive filesystem")
	}
	for _, pair := range [][2]string{{"pier", "PIER"}, {"caf\u00e9", "cafe\u0301"}} {
		p, alias := filepath.Join(parent, pair[0]), filepath.Join(parent, pair[1])
		release, err := Guard(p)
		if err != nil {
			t.Fatal(err)
		}
		for _, existing := range []bool{false, true} {
			if existing {
				if err := os.Mkdir(p, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if other, err := Guard(alias); err == nil {
				other()
				t.Errorf("filesystem alias acquired a second guard (existing=%v)", existing)
			}
		}
		release()
	}
}

func TestSecurityNativeStorageRefusesWritableStickyDirectories(t *testing.T) {
	for _, name := range []string{".", ".urb", ".urb/chk", ".urb/log", ".urb/log/0i0"} {
		t.Run(name, func(t *testing.T) {
			p, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			for _, dir := range []string{".urb/chk", ".urb/log/0i0"} {
				if err := os.MkdirAll(filepath.Join(p, dir), 0700); err != nil {
					t.Fatal(err)
				}
			}
			dir := filepath.Join(p, name)
			if err := os.Chmod(dir, os.ModeSticky|0777); err != nil {
				t.Fatal(err)
			}
			defer os.Chmod(dir, 0700)
			if err := VerifyRegistered(p, false); err == nil {
				t.Fatal("native storage permits insertion of an absent control file after preflight")
			}
		})
	}
}

func TestSecurityGuardPreservesForeignNamespace(t *testing.T) {
	parent := t.TempDir()
	namespace := filepath.Join(parent, ".harbor-guards-v1")
	if err := os.Mkdir(namespace, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(namespace, "keep"), []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if release, err := Guard(filepath.Join(parent, "pier")); err == nil {
		release()
		t.Fatal("foreign directory was adopted as a guard namespace")
	}
	entries, err := os.ReadDir(namespace)
	if err != nil || len(entries) != 1 || entries[0].Name() != "keep" {
		t.Fatal("guard changed an unrelated directory", entries, err)
	}
}
