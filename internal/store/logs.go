package store

import (
	"context"
	"harbor/internal/model"
)

// Retain at most 2 MiB per job for the latest 100 jobs. Chunked writes avoid
// rewriting the full retained output whenever Vere prints another line.
func (s *Store) AppendJobLog(ctx context.Context, id model.ID, p []byte) error {
	if len(p) > 2<<20 {
		p = p[len(p)-(2<<20):]
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for len(p) > 0 {
		n := min(len(p), 8192)
		if _, err = tx.ExecContext(ctx, "INSERT INTO job_log_chunks(job_id,output) VALUES(?,?)", id, p[:n]); err != nil {
			return err
		}
		p = p[n:]
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM job_log_chunks WHERE rowid IN (SELECT rowid FROM job_log_chunks WHERE job_id=? ORDER BY rowid DESC LIMIT -1 OFFSET 256)", id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM job_log_chunks WHERE job_id NOT IN (SELECT id FROM jobs ORDER BY sequence DESC LIMIT 100)"); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) JobLog(ctx context.Context, id model.ID) ([]byte, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT output FROM job_log_chunks WHERE job_id=? ORDER BY rowid", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []byte{}
	for rows.Next() {
		var b []byte
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		out = append(out, b...)
	}
	return out, rows.Err()
}
