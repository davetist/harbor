package store

import (
	"context"
	"harbor/internal/model"
)

func (s *Store) PutBackup(ctx context.Context, b model.Backup) error {
	_, e := s.db.ExecContext(ctx, "INSERT INTO backups(id,ship_id,archive_path,created_at,verified) VALUES(?,?,?,?,?)", b.ID, b.ShipID, b.ArchivePath, b.CreatedAt, b.Verified)
	return e
}
func (s *Store) GetBackup(ctx context.Context, id model.ID) (model.Backup, error) {
	var b model.Backup
	e := s.db.QueryRowContext(ctx, "SELECT id,ship_id,archive_path,created_at,verified FROM backups WHERE id=?", id).Scan(&b.ID, &b.ShipID, &b.ArchivePath, &b.CreatedAt, &b.Verified)
	return b, e
}
func (s *Store) ListBackups(ctx context.Context, id model.ID) ([]model.Backup, error) {
	rows, e := s.db.QueryContext(ctx, "SELECT id,ship_id,archive_path,created_at,verified FROM backups WHERE ship_id=? ORDER BY created_at DESC", id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []model.Backup{}
	for rows.Next() {
		var b model.Backup
		if e = rows.Scan(&b.ID, &b.ShipID, &b.ArchivePath, &b.CreatedAt, &b.Verified); e != nil {
			return nil, e
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
