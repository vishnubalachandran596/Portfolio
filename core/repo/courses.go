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

var ErrNotFound = errors.New("not found")

type CourseRepo struct{ db *sql.DB }

func NewCourseRepo(db *sql.DB) *CourseRepo { return &CourseRepo{db: db} }

// content_html is intentionally absent. Lesson bytes live in Storage;
// selecting that column would pull them onto every catalog query.
const courseColumns = `id, slug, title, summary, description, level, duration, price_cents, image_url, is_published, sort_order, is_paid, min_price_cents, content_key, content_object, rag_context, tags, created_at, updated_at`

func scanCourse(row interface{ Scan(...any) error }) (models.Course, error) {
	var c models.Course
	var tags []string
	err := row.Scan(&c.ID, &c.Slug, &c.Title, &c.Summary, &c.Description, &c.Level, &c.Duration,
		&c.PriceCents, &c.ImageURL, &c.IsPublished, &c.SortOrder,
		&c.IsPaid, &c.MinPriceCents, &c.ContentKey, &c.ContentObject, &c.RagContext, pq.Array(&tags), &c.CreatedAt, &c.UpdatedAt)
	c.Tags = tags
	return c, err
}

func (r *CourseRepo) ListPublished(ctx context.Context) ([]models.Course, error) {
	return r.list(ctx, `SELECT `+courseColumns+` FROM courses WHERE is_published = TRUE ORDER BY sort_order, id`)
}

func (r *CourseRepo) ListAll(ctx context.Context) ([]models.Course, error) {
	return r.list(ctx, `SELECT `+courseColumns+` FROM courses ORDER BY sort_order, id`)
}

func (r *CourseRepo) list(ctx context.Context, query string) ([]models.Course, error) {
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Course
	for rows.Next() {
		c, err := scanCourse(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CourseListOptions filters, sorts, and paginates the public /courses
// listing -- same shape as ArticleListOptions.
type CourseListOptions struct {
	Query              string // matched against title + summary + description, case-insensitive
	Tag                string // exact tag match
	Sort               string // "recommended" (default, sort_order), "reviews_desc", "newest"
	Page               int    // 1-indexed, defaults to 1
	PageSize           int    // defaults to 5 -- callers pass handlers.catalogPageSize, shared with /articles
	IncludeUnpublished bool   // admin-preview: also returns draft courses
}

type CourseListResult struct {
	Courses []models.Course
	Total   int
}

// ListPublishedFiltered powers the public /courses listing: search, tag
// filter, sort (including by review count -- the only review signal ever
// shown publicly, see migration 064), and pagination all in one query.
// Mirrors ArticleRepo.ListPublishedFiltered.
func (r *CourseRepo) ListPublishedFiltered(ctx context.Context, opt CourseListOptions) (CourseListResult, error) {
	page, pageSize := opt.Page, opt.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 5
	}

	where := []string{}
	if !opt.IncludeUnpublished {
		where = append(where, "c.is_published = TRUE")
	}
	args := []any{}
	if q := strings.TrimSpace(opt.Query); q != "" {
		args = append(args, "%"+q+"%")
		n := len(args)
		where = append(where, fmt.Sprintf("(c.title ILIKE $%d OR c.summary ILIKE $%d OR c.description ILIKE $%d)", n, n, n))
	}
	if tag := strings.TrimSpace(opt.Tag); tag != "" {
		args = append(args, tag)
		where = append(where, fmt.Sprintf("$%d = ANY(c.tags)", len(args)))
	}
	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	orderClause := "ORDER BY c.sort_order, c.id"
	switch opt.Sort {
	case "reviews_desc":
		orderClause = "ORDER BY review_count DESC, c.sort_order, c.id"
	case "newest":
		orderClause = "ORDER BY c.created_at DESC, c.id DESC"
	}

	// review_count and enrolled_count are both derived aggregates (neither
	// is a stored column on courses itself), joined once here rather than
	// queried per-row -- a course with zero reviews/purchases still needs
	// to sort/display correctly, hence COALESCE rather than an inner join.
	// enrolled_count starts at 1 for every course (a baseline so a brand
	// new course never reads "0 enrolled") plus one per paid purchase --
	// deriving it live from purchases rather than a separately maintained
	// counter column means every *existing* paid purchase already counts
	// automatically, and there's nothing for checkout.go to increment or
	// risk drifting out of sync with reality.
	fromClause := `FROM courses c
		LEFT JOIN (SELECT course_id, COUNT(*) AS n FROM course_reviews GROUP BY course_id) rc ON rc.course_id = c.id
		LEFT JOIN (SELECT course_id, COUNT(*) AS n FROM purchases WHERE status = 'paid' GROUP BY course_id) pc ON pc.course_id = c.id`

	countQuery := "SELECT count(*) " + fromClause + " " + whereClause
	countArgs := args

	listArgs := append(append([]any{}, args...), pageSize, (page-1)*pageSize)
	listQuery := fmt.Sprintf(
		"SELECT c.id, c.slug, c.title, c.summary, c.description, c.level, c.duration, c.price_cents, c.image_url, "+
			"c.is_published, c.sort_order, c.is_paid, c.min_price_cents, c.content_key, c.content_object, c.rag_context, "+
			"c.tags, c.created_at, c.updated_at, COALESCE(rc.n, 0) AS review_count, COALESCE(pc.n, 0) + 1 AS enrolled_count "+
			"%s %s %s LIMIT $%d OFFSET $%d",
		fromClause, whereClause, orderClause, len(listArgs)-1, len(listArgs))

	// Independent queries (the total for the pager, the page of rows) run
	// concurrently rather than as a serial round-trip chain -- same
	// reasoning as ArticleRepo.ListPublishedFiltered (CLAUDE.md §9).
	var (
		total int
		out   []models.Course
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
			var c models.Course
			var tags []string
			if err := rows.Scan(&c.ID, &c.Slug, &c.Title, &c.Summary, &c.Description, &c.Level, &c.Duration,
				&c.PriceCents, &c.ImageURL, &c.IsPublished, &c.SortOrder, &c.IsPaid, &c.MinPriceCents,
				&c.ContentKey, &c.ContentObject, &c.RagContext, pq.Array(&tags), &c.CreatedAt, &c.UpdatedAt,
				&c.ReviewCount, &c.EnrolledCount); err != nil {
				return err
			}
			c.Tags = tags
			out = append(out, c)
		}
		return rows.Err()
	})
	if err := g.Wait(); err != nil {
		return CourseListResult{}, err
	}
	return CourseListResult{Courses: out, Total: total}, nil
}

// ListAllTags returns every distinct tag used by a published course,
// heaviest first, each with the number of published courses carrying it.
// Mirrors ArticleRepo.ListAllTags.
func (r *CourseRepo) ListAllTags(ctx context.Context) ([]models.CourseTag, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT tag, COUNT(*) AS n
		FROM (SELECT unnest(tags) AS tag FROM courses WHERE is_published = TRUE) t
		WHERE tag <> ''
		GROUP BY tag
		ORDER BY n DESC, tag`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.CourseTag
	for rows.Next() {
		var t models.CourseTag
		if err := rows.Scan(&t.Name, &t.Count); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *CourseRepo) GetBySlug(ctx context.Context, slug string) (models.Course, error) {
	c, err := scanCourse(r.db.QueryRowContext(ctx, `SELECT `+courseColumns+` FROM courses WHERE slug = $1`, slug))
	if errors.Is(err, sql.ErrNoRows) {
		return models.Course{}, ErrNotFound
	}
	return c, err
}

func (r *CourseRepo) GetByID(ctx context.Context, id int64) (models.Course, error) {
	c, err := scanCourse(r.db.QueryRowContext(ctx, `SELECT `+courseColumns+` FROM courses WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return models.Course{}, ErrNotFound
	}
	return c, err
}

func (r *CourseRepo) Create(ctx context.Context, c models.Course) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO courses (slug, title, summary, description, level, duration, price_cents, image_url, is_published, sort_order,
			is_paid, min_price_cents, rag_context, tags)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING id`,
		c.Slug, c.Title, c.Summary, c.Description, c.Level, c.Duration, c.PriceCents, c.ImageURL, c.IsPublished, c.SortOrder,
		c.IsPaid, c.MinPriceCents, c.RagContext, pq.Array(c.Tags),
	).Scan(&id)
	return id, err
}

func (r *CourseRepo) Update(ctx context.Context, c models.Course) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE courses SET slug=$1, title=$2, summary=$3, description=$4, level=$5, duration=$6,
			price_cents=$7, image_url=$8, is_published=$9, sort_order=$10,
			is_paid=$11, min_price_cents=$12, rag_context=$13, tags=$14, updated_at=now()
		WHERE id=$15`,
		c.Slug, c.Title, c.Summary, c.Description, c.Level, c.Duration, c.PriceCents, c.ImageURL, c.IsPublished, c.SortOrder,
		c.IsPaid, c.MinPriceCents, c.RagContext, pq.Array(c.Tags), c.ID,
	)
	return err
}

// UpdateContentObject records the private Storage path for a lesson.
// Called only after a new file was uploaded, so an edit without a new
// file leaves the existing object where it is.
func (r *CourseRepo) UpdateContentObject(ctx context.Context, id int64, objectPath string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE courses SET content_object = $1, updated_at = now() WHERE id = $2`, objectPath, id)
	return err
}

func (r *CourseRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM courses WHERE id = $1`, id)
	return err
}
