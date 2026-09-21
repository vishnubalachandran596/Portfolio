package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/lib/pq"
	"github.com/vishnubalachandran596/portfolio/core/models"
)

type ArticleRepo struct{ db *sql.DB }

func NewArticleRepo(db *sql.DB) *ArticleRepo { return &ArticleRepo{db: db} }

const articleColumns = `id, slug, title, summary, body, cover_image_url, source_url, source_name, tags, thumbs_up_count, published_date, is_published, sort_order, created_at`

func scanArticle(row interface{ Scan(...any) error }) (models.Article, error) {
	var a models.Article
	var published sql.NullTime
	var tags []string
	err := row.Scan(&a.ID, &a.Slug, &a.Title, &a.Summary, &a.Body, &a.CoverImageURL, &a.SourceURL, &a.SourceName, pq.Array(&tags),
		&a.ThumbsUpCount, &published, &a.IsPublished, &a.SortOrder, &a.CreatedAt)
	a.Tags = tags
	if published.Valid {
		a.PublishedDate = &published.Time
	}
	return a, err
}

// ArticleListOptions filters and paginates the public /articles listing.
type ArticleListOptions struct {
	Query    string // matched against title + summary, case-insensitive
	Tag      string // exact tag match
	Sort     string // "date_desc" (default), "date_asc", "thumbs_desc"
	Page     int    // 1-indexed, defaults to 1
	PageSize int    // defaults to 5
}

type ArticleListResult struct {
	Articles []models.Article
	Total    int
}

// ListPublishedFiltered powers the public articles page: search, tag
// filter, sort, and pagination all in one query.
func (r *ArticleRepo) ListPublishedFiltered(ctx context.Context, opt ArticleListOptions) (ArticleListResult, error) {
	page, pageSize := opt.Page, opt.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 5
	}

	where := []string{"is_published = TRUE"}
	args := []any{}
	if q := strings.TrimSpace(opt.Query); q != "" {
		args = append(args, "%"+q+"%")
		where = append(where, fmt.Sprintf("(title ILIKE $%d OR summary ILIKE $%d)", len(args), len(args)))
	}
	if tag := strings.TrimSpace(opt.Tag); tag != "" {
		args = append(args, tag)
		where = append(where, fmt.Sprintf("$%d = ANY(tags)", len(args)))
	}
	whereClause := "WHERE " + strings.Join(where, " AND ")

	orderClause := "ORDER BY published_date DESC NULLS LAST, id DESC"
	switch opt.Sort {
	case "date_asc":
		orderClause = "ORDER BY published_date ASC NULLS LAST, id ASC"
	case "thumbs_desc":
		orderClause = "ORDER BY thumbs_up_count DESC, published_date DESC NULLS LAST, id DESC"
	}

	var total int
	countQuery := "SELECT count(*) FROM articles " + whereClause
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return ArticleListResult{}, err
	}

	args = append(args, pageSize, (page-1)*pageSize)
	listQuery := fmt.Sprintf("SELECT %s FROM articles %s %s LIMIT $%d OFFSET $%d",
		articleColumns, whereClause, orderClause, len(args)-1, len(args))
	rows, err := r.db.QueryContext(ctx, listQuery, args...)
	if err != nil {
		return ArticleListResult{}, err
	}
	defer rows.Close()

	var out []models.Article
	for rows.Next() {
		a, err := scanArticle(rows)
		if err != nil {
			return ArticleListResult{}, err
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return ArticleListResult{}, err
	}
	return ArticleListResult{Articles: out, Total: total}, nil
}

// ListPublished returns the most recent published articles — used for the
// home page's small "Worth reading" teaser, not the full /articles listing.
func (r *ArticleRepo) ListPublished(ctx context.Context, limit int) ([]models.Article, error) {
	res, err := r.ListPublishedFiltered(ctx, ArticleListOptions{Page: 1, PageSize: limit})
	if err != nil {
		return nil, err
	}
	return res.Articles, nil
}

// ListAllTags returns every distinct tag used by a published article, for
// the filter UI.
func (r *ArticleRepo) ListAllTags(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT DISTINCT unnest(tags) AS tag FROM articles WHERE is_published = TRUE ORDER BY tag`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
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

func (r *ArticleRepo) GetBySlug(ctx context.Context, slug string) (models.Article, error) {
	a, err := scanArticle(r.db.QueryRowContext(ctx, `SELECT `+articleColumns+` FROM articles WHERE slug = $1 AND is_published = TRUE`, slug))
	if errors.Is(err, sql.ErrNoRows) {
		return models.Article{}, ErrNotFound
	}
	return a, err
}

func (r *ArticleRepo) Create(ctx context.Context, a models.Article) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO articles (slug, title, summary, body, cover_image_url, source_url, source_name, tags, published_date, is_published, sort_order)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`,
		a.Slug, a.Title, a.Summary, a.Body, a.CoverImageURL, a.SourceURL, a.SourceName, pq.Array(a.Tags), a.PublishedDate, a.IsPublished, a.SortOrder,
	).Scan(&id)
	return id, err
}

func (r *ArticleRepo) Update(ctx context.Context, a models.Article) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE articles SET slug=$1, title=$2, summary=$3, body=$4, cover_image_url=$5, source_url=$6, source_name=$7, tags=$8, published_date=$9, is_published=$10, sort_order=$11
		WHERE id=$12`,
		a.Slug, a.Title, a.Summary, a.Body, a.CoverImageURL, a.SourceURL, a.SourceName, pq.Array(a.Tags), a.PublishedDate, a.IsPublished, a.SortOrder, a.ID,
	)
	return err
}

func (r *ArticleRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM articles WHERE id = $1`, id)
	return err
}

// IncrementThumbsUp bumps an article's thumbs-up count by one. The
// handler is responsible for not calling this twice for the same
// visitor+article (see the au_<slug> cookie set in handlers/articles.go).
func (r *ArticleRepo) IncrementThumbsUp(ctx context.Context, slug string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `UPDATE articles SET thumbs_up_count = thumbs_up_count + 1 WHERE slug = $1 RETURNING thumbs_up_count`, slug).Scan(&count)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return count, err
}
