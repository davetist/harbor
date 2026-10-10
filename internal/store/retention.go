package store

import (
	"context"
	"encoding/json"
	"harbor/internal/model"
)

// Unfinished and interrupted work, failed archive imports and pending
// registrations may still need their original input for recovery.
func (s *Store) ProtectedUploads(ctx context.Context) (map[model.ID]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT payload FROM jobs WHERE phase NOT IN ('completed','skipped') OR id IN (SELECT job_id FROM pending_registrations)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	protected := make(map[model.ID]bool)
	for rows.Next() {
		var payload string
		var job model.Job
		if err = rows.Scan(&payload); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(payload), &job); err != nil {
			return nil, err
		}
		if job.UploadID != "" {
			protected[job.UploadID] = true
		}
	}
	return protected, rows.Err()
}

func (s *Store) DeleteBackup(ctx context.Context, id model.ID) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM backups WHERE id=?", id)
	return err
}
