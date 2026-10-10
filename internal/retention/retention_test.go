package retention

import (
	"context"
	"fmt"
	"harbor/internal/backup"
	"harbor/internal/model"
	"harbor/internal/store"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func fixture(t *testing.T) (*store.Store, string, time.Time) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "harbor.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st, dir, time.Now()
}
func agedFile(t *testing.T, dir, name string, now time.Time) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("archive"), 0600); err != nil {
		t.Fatal(err)
	}
	old := now.Add(-40 * 24 * time.Hour)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	return p
}

func agedExport(t *testing.T, dir, name string, now time.Time) string {
	t.Helper()
	source := filepath.Join(filepath.Dir(dir), "export-source")
	if err := os.MkdirAll(filepath.Join(source, ".urb", "chk"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(source, ".urb", "log"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, ".urb", "chk", "image.bin"), []byte("snapshot"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if _, err := backup.Export(context.Background(), source, p); err != nil {
		t.Fatal(err)
	}
	old := now.Add(-40 * 24 * time.Hour)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	return p
}
func TestPreviewAndProtectedRecoveryInputs(t *testing.T) {
	ctx := context.Background()
	st, dir, now := fixture(t)
	for i, phase := range []string{"queued", "interrupted", "failed", "completed"} {
		id := model.ID(fmt.Sprintf("%032x", i+1))
		agedFile(t, filepath.Join(dir, "uploads"), string(id)+".tar.gz", now)
		j := model.Job{ID: model.NewID(), ShipID: "ship", Kind: "import_archive", Trigger: "manual", Phase: "queued", UploadID: id}
		if err := st.CreateJob(ctx, j); err != nil {
			t.Fatal(err)
		}
		j.Phase = phase
		if err := st.UpdateJob(ctx, j); err != nil {
			t.Fatal(err)
		}
	}
	unknown := agedFile(t, filepath.Join(dir, "uploads"), "operator-note", now)
	report, err := Plan(ctx, st, dir, Policy{UploadDays: 7, KeepExports: 2}, now)
	if err != nil {
		t.Fatal(err)
	}
	if report.ReclaimBytes != 7 || report.TotalBytes != 35 {
		t.Fatalf("unsafe accounting: %+v", report)
	}
	for _, entry := range report.Entries {
		if _, err := os.Stat(entry.Path); err != nil {
			t.Fatal("preview removed file", err)
		}
	}
	if err := Apply(ctx, st, &report); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(unknown); err != nil {
		t.Fatal("unrecognized file removed")
	}
	if _, err := os.Stat(filepath.Join(dir, "uploads", fmt.Sprintf("%032x.tar.gz", 4))); !os.IsNotExist(err) {
		t.Fatal("completed input was not pruned")
	}
}
func TestKeepExportsAndHeldShips(t *testing.T) {
	for _, held := range []bool{false, true} {
		t.Run(fmt.Sprint(held), func(t *testing.T) {
			ctx := context.Background()
			st, dir, now := fixture(t)
			ship := model.Ship{ID: "ship", PierPath: filepath.Join(dir, "pier"), RecoveryBlocked: held}
			if err := st.PutShip(ctx, ship); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 4; i++ {
				id := model.ID(fmt.Sprintf("%032x", i+1))
				p := agedExport(t, filepath.Join(dir, "backups"), string(id)+".tar.gz", now)
				if err := st.PutBackup(ctx, model.Backup{ID: id, ShipID: ship.ID, ArchivePath: p, CreatedAt: now.Add(-time.Duration(40+i) * 24 * time.Hour), Verified: true}); err != nil {
					t.Fatal(err)
				}
			}
			report, err := Plan(ctx, st, dir, Policy{ExportDays: 30, KeepExports: 2}, now)
			if err != nil {
				t.Fatal(err)
			}
			want := int64(0)
			for i := 3; i <= 4; i++ {
				info, err := os.Stat(filepath.Join(dir, "backups", fmt.Sprintf("%032x.tar.gz", i)))
				if err != nil {
					t.Fatal(err)
				}
				want += info.Size()
			}
			if held {
				want = 0
			}
			if report.ReclaimBytes != want {
				t.Fatal("protected export selected", report)
			}
			if err := Apply(ctx, st, &report); err != nil {
				t.Fatal(err)
			}
			rows, err := st.ListBackups(ctx, ship.ID)
			if err != nil {
				t.Fatal(err)
			}
			count := 2
			if held {
				count = 4
			}
			if len(rows) != count {
				t.Fatal("deleted metadata mismatch", rows)
			}
		})
	}
}

func TestCorruptNewestExportDoesNotPruneOnlyGoodCheckpoints(t *testing.T) {
	ctx := context.Background()
	st, dir, now := fixture(t)
	ship := model.Ship{ID: "ship", PierPath: filepath.Join(dir, "pier")}
	if err := st.PutShip(ctx, ship); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		id := model.ID(fmt.Sprintf("%032x", i+1))
		p := agedExport(t, filepath.Join(dir, "backups"), string(id)+".tar.gz", now)
		if i == 0 {
			if err := os.WriteFile(p, []byte("corrupted after verification"), 0600); err != nil {
				t.Fatal(err)
			}
			old := now.Add(-40 * 24 * time.Hour)
			if err := os.Chtimes(p, old, old); err != nil {
				t.Fatal(err)
			}
		}
		if err := st.PutBackup(ctx, model.Backup{ID: id, ShipID: ship.ID, ArchivePath: p, CreatedAt: now.Add(-time.Duration(40+i) * 24 * time.Hour), Verified: true}); err != nil {
			t.Fatal(err)
		}
	}
	report, err := Plan(ctx, st, dir, Policy{ExportDays: 30, KeepExports: 2}, now)
	if err != nil {
		t.Fatal(err)
	}
	if report.ReclaimBytes != 0 {
		t.Fatal("only good checkpoints selected", report)
	}
	if err := Apply(ctx, st, &report); err != nil {
		t.Fatal(err)
	}
	rows, err := st.ListBackups(ctx, ship.ID)
	if err != nil || len(rows) != 3 {
		t.Fatal("checkpoint lost", err)
	}
}

func TestChangedProtectedCheckpointStopsApply(t *testing.T) {
	ctx := context.Background()
	st, dir, now := fixture(t)
	ship := model.Ship{ID: "ship", PierPath: filepath.Join(dir, "pier")}
	if err := st.PutShip(ctx, ship); err != nil {
		t.Fatal(err)
	}
	var newest, oldest string
	for i := 0; i < 3; i++ {
		id := model.ID(fmt.Sprintf("%032x", i+1))
		p := agedExport(t, filepath.Join(dir, "backups"), string(id)+".tar.gz", now)
		if i == 0 {
			newest = p
		}
		if i == 2 {
			oldest = p
		}
		if err := st.PutBackup(ctx, model.Backup{ID: id, ShipID: ship.ID, ArchivePath: p, CreatedAt: now.Add(-time.Duration(40+i) * 24 * time.Hour), Verified: true}); err != nil {
			t.Fatal(err)
		}
	}
	report, err := Plan(ctx, st, dir, Policy{ExportDays: 30, KeepExports: 2}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newest, []byte("changed checkpoint"), 0600); err != nil {
		t.Fatal(err)
	}
	if Apply(ctx, st, &report) == nil {
		t.Fatal("changed retained checkpoint accepted")
	}
	if _, err := os.Stat(oldest); err != nil {
		t.Fatal("older good checkpoint removed")
	}
}
func TestRetentionBudgetAndChangedTargetFailClosed(t *testing.T) {
	ctx := context.Background()
	st, dir, now := fixture(t)
	path := agedFile(t, filepath.Join(dir, "uploads"), fmt.Sprintf("%032x.tar.gz", 1), now)
	agedFile(t, filepath.Join(dir, "uploads"), "unknown", now)
	report, err := Plan(ctx, st, dir, Policy{UploadDays: 7, KeepExports: 2, BudgetBytes: 1}, now)
	if err != nil {
		t.Fatal(err)
	}
	if !report.OverBudget || Apply(ctx, st, &report) == nil {
		t.Fatal("budget must refuse deletion")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("budget refusal removed a file")
	}
	report, err = Plan(ctx, st, dir, Policy{UploadDays: 7, KeepExports: 2}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "uploads", "unknown"), path); err != nil {
		t.Fatal(err)
	}
	if Apply(ctx, st, &report) == nil {
		t.Fatal("replacement symlink accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, "uploads", "unknown")); err != nil {
		t.Fatal("symlink target removed")
	}
}

func TestPendingRegistrationAndActiveJobPreserveInputsAndExports(t *testing.T) {
	ctx := context.Background()
	st, dir, now := fixture(t)
	id := model.ID(fmt.Sprintf("%032x", 1))
	input := agedFile(t, filepath.Join(dir, "uploads"), string(id)+".tar.gz", now)
	pending := model.Ship{ID: "pending", PierPath: filepath.Join(dir, "pending-pier")}
	j := model.Job{ID: model.NewID(), ShipID: pending.ID, Kind: "import_archive", Trigger: "manual", Phase: "queued", UploadID: id}
	if err := st.QueuePendingImport(ctx, pending, j); err != nil {
		t.Fatal(err)
	}
	// Even an inconsistent terminal phase cannot discard a pending registration's input.
	j.Phase = "completed"
	if err := st.UpdateJob(ctx, j); err != nil {
		t.Fatal(err)
	}
	ship := model.Ship{ID: "active", PierPath: filepath.Join(dir, "active-pier")}
	if err := st.PutShip(ctx, ship); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateJob(ctx, model.Job{ID: model.NewID(), ShipID: ship.ID, Kind: "replay", Phase: "queued", Trigger: "manual"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		backupID := model.ID(fmt.Sprintf("%032x", i+2))
		p := agedFile(t, filepath.Join(dir, "backups"), string(backupID)+".tar.gz", now)
		if err := st.PutBackup(ctx, model.Backup{ID: backupID, ShipID: ship.ID, ArchivePath: p, CreatedAt: now.Add(-40 * 24 * time.Hour), Verified: true}); err != nil {
			t.Fatal(err)
		}
	}
	report, err := Plan(ctx, st, dir, Policy{UploadDays: 7, ExportDays: 30, KeepExports: 2}, now)
	if err != nil {
		t.Fatal(err)
	}
	if report.ReclaimBytes != 0 {
		t.Fatal("pending input or active checkpoint selected", report)
	}
	if err := Apply(ctx, st, &report); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(input); err != nil {
		t.Fatal("pending input lost")
	}
}
