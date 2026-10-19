package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

// SEOKind names a content table that carries the four SEO columns.
type SEOKind string

const (
	SEOCourse   SEOKind = "courses"
	SEOArticle  SEOKind = "articles"
	SEOSoftware SEOKind = "software"
)

// table maps the kind onto its table name. The switch (rather than
// formatting the caller's string into SQL) is what keeps the identifier
// interpolation below safe.
func (k SEOKind) table() (string, error) {
	switch k {
	case SEOCourse, SEOArticle, SEOSoftware:
		return string(k), nil
	}
	return "", fmt.Errorf("seo: unknown kind %q", k)
}

// SEORepo owns page_seo and the SEO columns of courses, articles and
// software. It is deliberately separate from CourseRepo/ArticleRepo/
// SoftwareRepo so the catalog queries and scans stay untouched.
type SEORepo struct{ db *sql.DB }

func NewSEORepo(db *sql.DB) *SEORepo { return &SEORepo{db: db} }

const pageSEOColumns = `path, seo_title, meta_description, primary_keyword, og_image, noindex, updated_at`

func scanPageSEO(row interface{ Scan(...any) error }) (models.PageSEO, error) {
	var p models.PageSEO
	err := row.Scan(&p.Path, &p.SEOTitle, &p.MetaDescription, &p.PrimaryKeyword, &p.OGImage, &p.Noindex, &p.UpdatedAt)
	return p, err
}

func (r *SEORepo) GetPage(ctx context.Context, path string) (models.PageSEO, error) {
	p, err := scanPageSEO(r.db.QueryRowContext(ctx, `SELECT `+pageSEOColumns+` FROM page_seo WHERE path = $1`, path))
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

// ListPages returns every stored override keyed by path.
func (r *SEORepo) ListPages(ctx context.Context) (map[string]models.PageSEO, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+pageSEOColumns+` FROM page_seo`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]models.PageSEO{}
	for rows.Next() {
		p, err := scanPageSEO(rows)
		if err != nil {
			return nil, err
		}
		out[p.Path] = p
	}
	return out, rows.Err()
}

func (r *SEORepo) UpsertPage(ctx context.Context, p models.PageSEO) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO page_seo (path, seo_title, meta_description, primary_keyword, og_image, noindex, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6, now())
		ON CONFLICT (path) DO UPDATE SET seo_title=EXCLUDED.seo_title, meta_description=EXCLUDED.meta_description,
			primary_keyword=EXCLUDED.primary_keyword, og_image=EXCLUDED.og_image, noindex=EXCLUDED.noindex, updated_at=now()`,
		p.Path, p.SEOTitle, p.MetaDescription, p.PrimaryKeyword, p.OGImage, p.Noindex)
	return err
}

// DeletePage removes a page's override, restoring its built-in defaults.
func (r *SEORepo) DeletePage(ctx context.Context, path string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM page_seo WHERE path = $1`, path)
	return err
}

func (r *SEORepo) GetFields(ctx context.Context, kind SEOKind, id int64) (models.SEOFields, error) {
	return r.getFields(ctx, kind, "id", id)
}

func (r *SEORepo) GetFieldsBySlug(ctx context.Context, kind SEOKind, slug string) (models.SEOFields, error) {
	return r.getFields(ctx, kind, "slug", slug)
}

func (r *SEORepo) getFields(ctx context.Context, kind SEOKind, col string, key any) (models.SEOFields, error) {
	t, err := kind.table()
	if err != nil {
		return models.SEOFields{}, err
	}
	var f models.SEOFields
	err = r.db.QueryRowContext(ctx,
		`SELECT seo_title, meta_description, primary_keyword, og_image FROM `+t+` WHERE `+col+` = $1`, key,
	).Scan(&f.SEOTitle, &f.MetaDescription, &f.PrimaryKeyword, &f.OGImage)
	if errors.Is(err, sql.ErrNoRows) {
		return f, ErrNotFound
	}
	return f, err
}

func (r *SEORepo) SetFields(ctx context.Context, kind SEOKind, id int64, f models.SEOFields) error {
	t, err := kind.table()
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx,
		`UPDATE `+t+` SET seo_title=$1, meta_description=$2, primary_keyword=$3, og_image=$4 WHERE id=$5`,
		f.SEOTitle, f.MetaDescription, f.PrimaryKeyword, f.OGImage, id)
	return err
}

// SitemapDates carries the "last really edited" timestamps the sitemap
// needs: per-article lastmod (updated_at, falling back to created_at),
// plus the newest edit across each content type, which becomes the
// lastmod of that type's listing page.
type SitemapDates struct {
	Articles       map[string]time.Time
	LatestArticle  time.Time
	LatestCourse   time.Time
	LatestSoftware time.Time
	ProfileUpdated time.Time
}

func (r *SEORepo) SitemapDates(ctx context.Context) (SitemapDates, error) {
	d := SitemapDates{Articles: map[string]time.Time{}}
	rows, err := r.db.QueryContext(ctx,
		`SELECT slug, COALESCE(updated_at, created_at) FROM articles WHERE is_published = TRUE`)
	if err != nil {
		return d, err
	}
	defer rows.Close()
	for rows.Next() {
		var slug string
		var t time.Time
		if err := rows.Scan(&slug, &t); err != nil {
			return d, err
		}
		d.Articles[slug] = t
		if t.After(d.LatestArticle) {
			d.LatestArticle = t
		}
	}
	if err := rows.Err(); err != nil {
		return d, err
	}
	// Best-effort aggregates: a missing value just omits that lastmod.
	var c, s, p sql.NullTime
	_ = r.db.QueryRowContext(ctx, `SELECT MAX(updated_at) FROM courses WHERE is_published = TRUE`).Scan(&c)
	_ = r.db.QueryRowContext(ctx, `SELECT MAX(updated_at) FROM software WHERE is_published = TRUE`).Scan(&s)
	_ = r.db.QueryRowContext(ctx, `SELECT MAX(updated_at) FROM profile`).Scan(&p)
	d.LatestCourse, d.LatestSoftware, d.ProfileUpdated = c.Time, s.Time, p.Time
	return d, nil
}
