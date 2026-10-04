package mysqlrepo

import (
	"context"
	"os"
	"path/filepath"
	"time"
)

func (s Store) Cleanup(ctx context.Context, uploadDir string) error {
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < UTC_TIMESTAMP(6)`); err != nil {
		return err
	}
	before := time.Now().UTC().Add(-24 * time.Hour)
	rows, err := s.DB.QueryContext(ctx, `SELECT id,filename FROM images WHERE article_id IS NULL AND created_at < ?`, before)
	if err != nil {
		return err
	}
	type staleImage struct {
		id       int64
		filename string
	}
	var stale []staleImage
	for rows.Next() {
		var v staleImage
		if err = rows.Scan(&v.id, &v.filename); err != nil {
			rows.Close()
			return err
		}
		stale = append(stale, v)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	_ = rows.Close()
	for _, v := range stale {
		result, e := s.DB.ExecContext(ctx, `DELETE FROM images WHERE id=? AND article_id IS NULL`, v.id)
		if e != nil {
			return e
		}
		n, _ := result.RowsAffected()
		if n == 0 {
			continue
		}
		if e = os.Remove(filepath.Join(uploadDir, v.filename)); e != nil && !os.IsNotExist(e) {
			return e
		}
	}
	return nil
}
