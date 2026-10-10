package jobs

import (
	"context"
	"harbor/internal/backup"
	"harbor/internal/model"
	"harbor/internal/store"
	"harbor/internal/supervisor"
	"os"
	"path/filepath"
	"testing"
)

func TestBackupRecordsSourceRuntimeVersion(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	st, err := store.Open(filepath.Join(root, "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p := filepath.Join(root, "pier")
	for _, d := range []string{".urb/chk", ".urb/log"} {
		if err = os.MkdirAll(filepath.Join(p, d), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err = os.WriteFile(filepath.Join(p, ".urb/chk/image.bin"), []byte("snapshot"), 0600); err != nil {
		t.Fatal(err)
	}
	ship := model.Ship{ID: "old-runtime", Name: "~zod", PierPath: p, RuntimeVersion: "Vere 4.5", LoomExponent: 31, State: "stopped"}
	if err = st.PutShip(ctx, ship); err != nil {
		t.Fatal(err)
	}
	ex := New(st, supervisor.New(st, nil))
	ex.EnableBackups(filepath.Join(root, "backups"))
	ex.SetReady(true)
	id, err := ex.Submit(ctx, model.JobRequest{ShipID: ship.ID, Kind: "backup", Trigger: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if err = ex.RunOne(ctx); err != nil {
		t.Fatal(err)
	}
	j, err := st.GetJob(ctx, id)
	if err != nil || j.Phase != "completed" {
		t.Fatalf("backup failed: %+v %v", j, err)
	}
	rows, err := st.ListBackups(ctx, ship.ID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("backups: %+v %v", rows, err)
	}
	m, err := backup.Validate(ctx, rows[0].ArchivePath)
	if err != nil {
		t.Fatal(err)
	}
	if m.VereVersion != "4.5" {
		t.Fatalf("source Vere 4.5 was mislabeled %q", m.VereVersion)
	}
}
