package jobs

import (
	"context"
	"harbor/internal/model"
	"os"
	"path/filepath"
	"testing"
)

func TestSecurityRecoveryCleanupPreservesReplacementDirectory(t *testing.T) {
	source := recoveryCopySource(t)
	copy, err := newRecoveryCopy(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	defer copy.close()
	private := filepath.Join(filepath.Dir(source), "private")
	if err := os.Mkdir(private, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(private, "secret"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(copy.path, copy.path+"-moved"); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(private, copy.path); err != nil {
		t.Fatal(err)
	}
	if err := copy.close(); err == nil {
		t.Error("staging replacement was not detected")
	}
	if b, err := os.ReadFile(filepath.Join(copy.path, "secret")); err != nil || string(b) != "keep" {
		t.Fatalf("private sibling was deleted: %v", err)
	}
}

func TestSecurityEveryUtilityRejectsAliasedNativeStorageBeforeSpawn(t *testing.T) {
	for _, op := range []model.Operation{"pack", "meld", "roll", "chop", "replay", "update"} {
		for _, name := range []string{".urb/log/0i123/lock.mdb", ".urb/log/data.mdb"} {
			t.Run(string(op)+"/"+name, func(t *testing.T) {
				ex, st, ship, capture := utilityMappingExecutor(t)
				file := filepath.Join(ship.PierPath, name)
				if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(t.TempDir(), "absent"), file); err != nil {
					t.Fatal(err)
				}
				job := mappingTestJob(t, st, ship, op)
				if err := ex.maintain(context.Background(), &job, ship); err == nil {
					t.Fatal("native utility accepted aliased storage")
				}
				if _, err := os.Lstat(capture); !os.IsNotExist(err) {
					t.Fatal("unsafe utility child was executed", err)
				}
				if processes, err := st.ProcessesForJob(context.Background(), job.ID); err != nil || len(processes) != 0 {
					t.Fatal("unsafe utility recorded a launch", err)
				}
			})
		}
	}
}

func TestSecurityRecoveryRefusesReplaceableAncestry(t *testing.T) {
	source := recoveryCopySource(t)
	parent := filepath.Dir(source)
	if err := os.Chmod(parent, 0777); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(parent, 0700)
	copy, err := newRecoveryCopy(context.Background(), source)
	if copy != nil {
		copy.close()
	}
	if err == nil {
		t.Fatal("recovery staged beneath a replaceable non-sticky parent")
	}
}
