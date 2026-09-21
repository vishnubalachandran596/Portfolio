package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

type ArticleVoteRepo struct{ db *sql.DB }

func NewArticleVoteRepo(db *sql.DB) *ArticleVoteRepo { return &ArticleVoteRepo{db: db} }

// ErrAlreadyVoted means this user has already thumbs-up'd this article —
// the unique constraint on (article_id, user_id) is the single source of
// truth for that, not a cookie, so it holds even across browsers/devices
// for the same account and survives a race between two requests.
var ErrAlreadyVoted = errors.New("already voted")

// Create records one thumbs-up on an article by a real or guest-by-email
// user (either way, a row in the users table — the unique constraint
// de-dupes on that id).
func (r *ArticleVoteRepo) Create(ctx context.Context, articleID, userID int64) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO article_thumbs_up (article_id, user_id) VALUES ($1, $2)`,
		articleID, userID,
	)
	if isUniqueViolation(err) {
		return ErrAlreadyVoted
	}
	return err
}

// CreateGuest records one thumbs-up from a visitor who gave a name but no
// email (or nothing at all) — there's no stable identity to dedupe on,
// so every call is a new, distinct vote, same as the site behaved before
// any identity capture existed.
func (r *ArticleVoteRepo) CreateGuest(ctx context.Context, articleID int64, name string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO article_thumbs_up (article_id, guest_name) VALUES ($1, $2)`,
		articleID, name,
	)
	return err
}

// HasVoted reports whether this user has already thumbs-up'd this
// article, for rendering the vote button's initial disabled/voted state.
func (r *ArticleVoteRepo) HasVoted(ctx context.Context, articleID, userID int64) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM article_thumbs_up WHERE article_id = $1 AND user_id = $2)`,
		articleID, userID,
	).Scan(&exists)
	return exists, err
}

// ListByArticle returns every vote on an article, newest first, with the
// voter's name/email joined in — the admin "who thumbs-up'd this" view.
// Name falls back to the vote row's own guest_name when there's no
// linked user (a name-only, no-email vote); email is only ever present
// through a linked user row, real or guest-by-email.
func (r *ArticleVoteRepo) ListByArticle(ctx context.Context, articleID int64) ([]models.ArticleVote, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT t.id, COALESCE(u.name, t.guest_name, ''), COALESCE(u.email, ''), COALESCE(u.is_guest, false), t.created_at
		FROM article_thumbs_up t
		LEFT JOIN users u ON u.id = t.user_id
		WHERE t.article_id = $1
		ORDER BY t.created_at DESC`, articleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ArticleVote
	for rows.Next() {
		var v models.ArticleVote
		if err := rows.Scan(&v.ID, &v.UserName, &v.UserEmail, &v.IsGuest, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
