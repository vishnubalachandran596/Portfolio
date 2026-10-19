package repo

import (
	"context"
	"database/sql"
	"errors"
)

// SlugRedirectRepo reads the permanent redirects written when a course
// slug is renamed (migration 086). It is consulted only when a request
// is about to 404, so live routes never pay for it.
type SlugRedirectRepo struct{ db *sql.DB }

func NewSlugRedirectRepo(db *sql.DB) *SlugRedirectRepo { return &SlugRedirectRepo{db: db} }

// Lookup returns the new path for an old one, or ErrNotFound.
func (r *SlugRedirectRepo) Lookup(ctx context.Context, oldPath string) (string, error) {
	var to string
	err := r.db.QueryRowContext(ctx, `SELECT new_path FROM slug_redirects WHERE old_path = $1`, oldPath).Scan(&to)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return to, err
}

// execer is satisfied by *sql.DB and *sql.Tx.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// upsertRedirect records oldPath -> newPath and keeps the table free of
// chains and loops: rows that pointed at oldPath are re-pointed at
// newPath, and a row whose old_path is newPath (renaming back) is
// removed. Idempotent.
func upsertRedirect(ctx context.Context, x execer, oldPath, newPath string) error {
	if oldPath == newPath {
		return nil
	}
	if _, err := x.ExecContext(ctx, `UPDATE slug_redirects SET new_path = $2 WHERE new_path = $1`, oldPath, newPath); err != nil {
		return err
	}
	if _, err := x.ExecContext(ctx, `DELETE FROM slug_redirects WHERE old_path = $1`, newPath); err != nil {
		return err
	}
	_, err := x.ExecContext(ctx, `
		INSERT INTO slug_redirects (old_path, new_path) VALUES ($1, $2)
		ON CONFLICT (old_path) DO UPDATE SET new_path = EXCLUDED.new_path`, oldPath, newPath)
	return err
}
