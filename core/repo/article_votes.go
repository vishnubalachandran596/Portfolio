package repo

import (
	"context"
	"database/sql"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

type ArticleVoteRepo struct{ db *sql.DB }

func NewArticleVoteRepo(db *sql.DB) *ArticleVoteRepo { return &ArticleVoteRepo{db: db} }

// Create records one thumbs-up on an article. userID is nil for an
// anonymous (not-logged-in) visitor — voting never requires an account,
// so this is the normal case, not an edge case.
func (r *ArticleVoteRepo) Create(ctx context.Context, articleID int64, userID *int64) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO article_thumbs_up (article_id, user_id) VALUES ($1, $2)`,
		articleID, userID,
	)
	return err
}

// ListByArticle returns every vote on an article, newest first, with the
// voter's name/email joined in when they were logged in — the admin "who
// thumbs-up'd this" view.
func (r *ArticleVoteRepo) ListByArticle(ctx context.Context, articleID int64) ([]models.ArticleVote, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT t.id, COALESCE(u.name, ''), COALESCE(u.email, ''), t.created_at
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
		if err := rows.Scan(&v.ID, &v.UserName, &v.UserEmail, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
