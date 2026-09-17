package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

type ArticleRepo struct{ db *sql.DB }

func NewArticleRepo(db *sql.DB) *ArticleRepo { return &ArticleRepo{db: db} }

const articleColumns = `id, title, summary, source_url, source_name, published_date, is_published, sort_order, created_at`

func scanArticle(row interface{ Scan(...any) error }) (models.Article, error) {
	var a models.Article
	var published sql.NullTime
	err := row.Scan(&a.ID, &a.Title, &a.Summary, &a.SourceURL, &a.SourceName, &published, &a.IsPublished, &a.SortOrder, &a.CreatedAt)
	if published.Valid {
		a.PublishedDate = &published.Time
	}
	return a, err
}

func (r *ArticleRepo) ListPublished(ctx context.Context) ([]models.Article, error) {
	return r.list(ctx, `SELECT `+articleColumns+` FROM articles WHERE is_published = TRUE ORDER BY sort_order, published_date DESC NULLS LAST, id DESC`)
}

func (r *ArticleRepo) ListAll(ctx context.Context) ([]models.Article, error) {
	return r.list(ctx, `SELECT `+articleColumns+` FROM articles ORDER BY sort_order, published_date DESC NULLS LAST, id DESC`)
}

func (r *ArticleRepo) list(ctx context.Context, query string) ([]models.Article, error) {
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Article
	for rows.Next() {
		a, err := scanArticle(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *ArticleRepo) GetByID(ctx context.Context, id int64) (models.Article, error) {
	a, err := scanArticle(r.db.QueryRowContext(ctx, `SELECT `+articleColumns+` FROM articles WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return models.Article{}, ErrNotFound
	}
	return a, err
}

func (r *ArticleRepo) Create(ctx context.Context, a models.Article) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO articles (title, summary, source_url, source_name, published_date, is_published, sort_order)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		a.Title, a.Summary, a.SourceURL, a.SourceName, a.PublishedDate, a.IsPublished, a.SortOrder,
	).Scan(&id)
	return id, err
}

func (r *ArticleRepo) Update(ctx context.Context, a models.Article) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE articles SET title=$1, summary=$2, source_url=$3, source_name=$4, published_date=$5, is_published=$6, sort_order=$7
		WHERE id=$8`,
		a.Title, a.Summary, a.SourceURL, a.SourceName, a.PublishedDate, a.IsPublished, a.SortOrder, a.ID,
	)
	return err
}

func (r *ArticleRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM articles WHERE id = $1`, id)
	return err
}
