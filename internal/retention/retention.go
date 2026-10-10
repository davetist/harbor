// Package retention plans explicit, offline cleanup of Harbor-owned archives.
package retention

import (
	"context"
	"errors"
	"harbor/internal/backup"
	"harbor/internal/model"
	"harbor/internal/store"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

type Policy struct {
	UploadDays  int
	ExportDays  int
	KeepExports int
	BudgetBytes int64
}
type Entry struct {
	Path           string `json:"path"`
	Kind           string `json:"kind"`
	Bytes          int64  `json:"bytes"`
	AllocatedBytes int64  `json:"allocatedBytes"`
	Action         string `json:"action"`
	Reason         string `json:"reason"`
	backupID       model.ID
	info           os.FileInfo
}
type Report struct {
	Entries        []Entry `json:"entries"`
	TotalBytes     int64   `json:"totalBytes"`
	AllocatedBytes int64   `json:"allocatedBytes"`
	ReclaimBytes   int64   `json:"reclaimBytes"`
	RemainingBytes int64   `json:"remainingBytes"`
	BudgetBytes    int64   `json:"budgetBytes,omitempty"`
	OverBudget     bool    `json:"overBudget"`
	Applied        bool    `json:"applied"`
}

var ownedName = regexp.MustCompile(`^[0-9a-f]{32}\.(key|tar\.gz)$`)

// The caller must hold the controller lock and keep Harbor stopped throughout
// planning/application. A live cleanup can race new jobs and archive readers.
func Plan(ctx context.Context, st *store.Store, dataDir string, policy Policy, now time.Time) (Report, error) {
	r := Report{Entries: []Entry{}, BudgetBytes: policy.BudgetBytes}
	if policy.UploadDays < 0 || policy.ExportDays < 0 || policy.UploadDays > 36500 || policy.ExportDays > 36500 || policy.KeepExports < 2 || policy.BudgetBytes < 0 {
		return r, errors.New("ages must be 0–36500 days, keep-exports at least 2, and budget nonnegative")
	}
	// macOS aliases /var to /private/var; canonicalize the configured root,
	// then reject redirection of the managed subdirectories themselves.
	dataDir, err := filepath.EvalSymlinks(dataDir)
	if err != nil {
		return r, err
	}
	uploads, err := st.ProtectedUploads(ctx)
	if err != nil {
		return r, err
	}
	ships, err := st.ListShips(ctx)
	if err != nil {
		return r, err
	}
	backups := map[string]model.Backup{}
	keep := map[model.ID]string{}
	for _, ship := range ships {
		rows, err := st.ListBackups(ctx, ship.ID)
		if err != nil {
			return r, err
		}
		pending, err := st.HasPendingJob(ctx, ship.ID)
		if err != nil {
			return r, err
		}
		verified := 0
		for _, b := range rows {
			info, err := os.Lstat(b.ArchivePath)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return r, err
			}
			if !info.Mode().IsRegular() {
				continue
			}
			canonical, err := filepath.EvalSymlinks(b.ArchivePath)
			if err != nil {
				return r, err
			}
			if filepath.Dir(canonical) != filepath.Join(dataDir, "backups") {
				continue
			}
			backups[canonical] = b
			if ship.RecoveryBlocked || pending {
				keep[b.ID] = "ship has active work or a recovery hold"
				continue
			}
			if !b.Verified {
				keep[b.ID] = "unverified export preserved for inspection"
				continue
			}
			if verified < policy.KeepExports {
				if _, err := backup.Validate(ctx, canonical); err != nil {
					if ctx.Err() != nil {
						return r, ctx.Err()
					}
					keep[b.ID] = "export failed integrity check; preserved for inspection"
					continue
				}
				verified++
				keep[b.ID] = "newest integrity-checked exports retained"
			}
		}
	}
	for _, kind := range []string{"uploads", "backups"} {
		dir := filepath.Join(dataDir, kind)
		// Refuse a redirected managed directory; do not traverse symlinks.
		real, err := filepath.EvalSymlinks(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return r, err
		}
		if real != dir {
			return r, errors.New("retention directory is not canonical")
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return r, err
		}
		for _, file := range entries {
			if err := ctx.Err(); err != nil {
				return r, err
			}
			info, err := file.Info()
			if err != nil {
				return r, err
			}
			if !info.Mode().IsRegular() {
				continue
			}
			e := Entry{Path: filepath.Join(dir, file.Name()), Kind: kind, Bytes: info.Size(), Action: "retain", Reason: "unrecognized file preserved", info: info}
			if stat, ok := info.Sys().(*syscall.Stat_t); ok {
				e.AllocatedBytes = stat.Blocks * 512
			}
			if ownedName.MatchString(file.Name()) {
				if kind == "uploads" {
					id := model.ID(strings.SplitN(file.Name(), ".", 2)[0])
					if uploads[id] {
						e.Reason = "input referenced by unfinished or failed work"
					} else {
						days := policy.UploadDays
						if strings.HasSuffix(file.Name(), ".key") {
							days = 1
							e.Kind = "key"
						}
						e.Reason = "retention disabled or age threshold not reached"
						if days > 0 && now.Sub(info.ModTime()) > time.Duration(days)*24*time.Hour {
							e.Action = "prune"
							e.Reason = "unreferenced input past retention age"
						}
					}
				} else if b, ok := backups[e.Path]; ok {
					e.backupID = b.ID
					if reason, protected := keep[b.ID]; protected {
						e.Reason = reason
					} else {
						e.Reason = "retention disabled or age threshold not reached"
						// Both the record and file must be old; a replaced file is not silently pruned.
						age := time.Duration(policy.ExportDays) * 24 * time.Hour
						if policy.ExportDays > 0 && now.Sub(b.CreatedAt) > age && now.Sub(info.ModTime()) > age {
							e.Action = "prune"
							e.Reason = "older export past retention age"
						}
					}
				}
			}
			r.Entries = append(r.Entries, e)
			r.TotalBytes += e.Bytes
			r.AllocatedBytes += e.AllocatedBytes
			if e.Action == "prune" {
				r.ReclaimBytes += e.Bytes
			}
		}
	}
	r.RemainingBytes = r.TotalBytes - r.ReclaimBytes
	r.OverBudget = policy.BudgetBytes > 0 && r.RemainingBytes > policy.BudgetBytes
	return r, nil
}

func Apply(ctx context.Context, st *store.Store, r *Report) error {
	if r.OverBudget {
		return errors.New("protected or recent files exceed the budget; no files were removed")
	}
	for _, e := range r.Entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if e.Action != "prune" && e.Reason != "newest integrity-checked exports retained" {
			continue
		}
		info, err := os.Lstat(e.Path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || !os.SameFile(info, e.info) || info.Size() != e.Bytes || !info.ModTime().Equal(e.info.ModTime()) {
			return errors.New("retention target changed; cleanup stopped")
		}
	}
	for _, e := range r.Entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if e.Action != "prune" {
			continue
		}
		if err := os.Remove(e.Path); err != nil {
			return err
		}
		if e.backupID != "" {
			if err := st.DeleteBackup(ctx, e.backupID); err != nil {
				return err
			}
		}
	}
	r.Applied = true
	return nil
}
