package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/lib/pq"
	"golang.org/x/sync/errgroup"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

type ArticleRepo struct{ db *sql.DB }

func NewArticleRepo(db *sql.DB) *ArticleRepo { return &ArticleRepo{db: db} }

const articleColumns = `id, slug, title, summary, body, content_object, cover_image_url, tags, thumbs_up_count, published_date, is_published, sort_order, created_at`

// articleListColumns is articleColumns without body. Only the article's own
// page renders a body; every list — /articles, the home page teaser, the
// sitemap — shows the summary and throws the body away. Bodies average
// ~10 KB, so a five-article page was pulling ~50 KB of unused text across
// a cross-region link on every render, and the sitemap pulled every body
// on the site to emit a slug and a date. Article.Body is therefore empty
// on rows that came from a list query; anything that needs it wants
// GetBySlug or GetByID.
const articleListColumns = `id, slug, title, summary, cover_image_url, tags, thumbs_up_count, published_date, is_published, sort_order, created_at`

func scanArticle(row interface{ Scan(...any) error }) (models.Article, error) {
	var a models.Article
	var published sql.NullTime
	var tags []string
	err := row.Scan(&a.ID, &a.Slug, &a.Title, &a.Summary, &a.Body, &a.ContentObject, &a.CoverImageURL, pq.Array(&tags),
		&a.ThumbsUpCount, &published, &a.IsPublished, &a.SortOrder, &a.CreatedAt)
	a.Tags = tags
	if published.Valid {
		a.PublishedDate = &published.Time
	}
	return a, err
}

// scanArticleListRow scans a row selected with articleListColumns, leaving
// Body zero.
func scanArticleListRow(row interface{ Scan(...any) error }) (models.Article, error) {
	var a models.Article
	var published sql.NullTime
	var tags []string
	err := row.Scan(&a.ID, &a.Slug, &a.Title, &a.Summary, &a.CoverImageURL, pq.Array(&tags),
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

	countQuery := "SELECT count(*) FROM articles " + whereClause
	countArgs := args

	listArgs := append(append([]any{}, args...), pageSize, (page-1)*pageSize)
	listQuery := fmt.Sprintf("SELECT %s FROM articles %s %s LIMIT $%d OFFSET $%d",
		articleListColumns, whereClause, orderClause, len(listArgs)-1, len(listArgs))

	// The count and the page of rows are independent of each other, and
	// running them one after the other made /articles two round-trip waves
	// where one would do — the page's total is only needed to render the
	// pager, not to build the list query. Both must succeed for the page to
	// be correct, so this is an errgroup rather than a WaitGroup.
	var (
		total int
		out   []models.Article
	)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return r.db.QueryRowContext(gctx, countQuery, countArgs...).Scan(&total)
	})
	g.Go(func() error {
		rows, err := r.db.QueryContext(gctx, listQuery, listArgs...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			a, err := scanArticleListRow(rows)
			if err != nil {
				return err
			}
			out = append(out, a)
		}
		return rows.Err()
	})
	if err := g.Wait(); err != nil {
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

// ListAllTags returns every distinct tag used by a published article,
// heaviest first, each with the number of published articles carrying it.
// The count comes free from the same scan the distinct list already needed,
// and is what lets /articles present its topics in order of how much is
// actually written under each one.
func (r *ArticleRepo) ListAllTags(ctx context.Context) ([]models.ArticleTag, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT tag, COUNT(*) AS n
		FROM (SELECT unnest(tags) AS tag FROM articles WHERE is_published = TRUE) t
		WHERE tag <> ''
		GROUP BY tag
		ORDER BY n DESC, tag`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ArticleTag
	for rows.Next() {
		var t models.ArticleTag
		if err := rows.Scan(&t.Name, &t.Count); err != nil {
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
		INSERT INTO articles (slug, title, summary, body, content_object, cover_image_url, tags, published_date, is_published, sort_order)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`,
		a.Slug, a.Title, a.Summary, a.Body, a.ContentObject, a.CoverImageURL, pq.Array(a.Tags), a.PublishedDate, a.IsPublished, a.SortOrder,
	).Scan(&id)
	return id, err
}

func (r *ArticleRepo) Update(ctx context.Context, a models.Article) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE articles SET slug=$1, title=$2, summary=$3, body=$4, content_object=$5, cover_image_url=$6, tags=$7, published_date=$8, is_published=$9, sort_order=$10
		WHERE id=$11`,
		a.Slug, a.Title, a.Summary, a.Body, a.ContentObject, a.CoverImageURL, pq.Array(a.Tags), a.PublishedDate, a.IsPublished, a.SortOrder, a.ID,
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
