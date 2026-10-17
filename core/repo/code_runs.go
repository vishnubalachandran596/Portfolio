package repo

import (
	"context"
	"database/sql"
	"time"
)

// CodeRunRepo records runs sent to the code runner and counts them for the
// rate limits. Counting is best-effort (read, then insert), which is fine for
// a limit meant to stop a runaway loop, not to meter exactly.
type CodeRunRepo struct{ db *sql.DB }

func NewCodeRunRepo(db *sql.DB) *CodeRunRepo { return &CodeRunRepo{db: db} }

func (r *CodeRunRepo) Record(ctx context.Context, courseID int64, actor, lang string) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO code_runs (course_id, actor, lang) VALUES ($1,$2,$3)`, courseID, actor, lang)
	return err
}

// CountByActorSince is how many runs one learner started since t.
func (r *CodeRunRepo) CountByActorSince(ctx context.Context, actor string, since time.Time) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM code_runs WHERE actor = $1 AND created_at >= $2`, actor, since).Scan(&n)
	return n, err
}

// CountSince is how many runs everyone started since t (the plan-wide cap).
func (r *CodeRunRepo) CountSince(ctx context.Context, since time.Time) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM code_runs WHERE created_at >= $1`, since).Scan(&n)
	return n, err
}
