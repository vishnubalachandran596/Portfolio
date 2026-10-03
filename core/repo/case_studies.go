package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

// CaseStudyRepo follows the exact shape CertificationRepo/PublicationRepo
// established in credentials.go: ListPublished/ListAll/GetByID/Create/
// Update/Delete, with ListPublished returning a nil slice on no rows so
// About and Consultancy can drop the whole section rather than render an
// empty heading.
type CaseStudyRepo struct{ db *sql.DB }

func NewCaseStudyRepo(db *sql.DB) *CaseStudyRepo { return &CaseStudyRepo{db: db} }

const caseStudyColumns = `id, title, client_context, problem, approach, result, is_published, sort_order, created_at`

func scanCaseStudy(row interface{ Scan(...any) error }) (models.CaseStudy, error) {
	var c models.CaseStudy
	err := row.Scan(&c.ID, &c.Title, &c.ClientContext, &c.Problem, &c.Approach, &c.Result,
		&c.IsPublished, &c.SortOrder, &c.CreatedAt)
	return c, err
}

func (r *CaseStudyRepo) listWhere(ctx context.Context, where string) ([]models.CaseStudy, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+caseStudyColumns+` FROM case_studies `+where+` ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.CaseStudy
	for rows.Next() {
		c, err := scanCaseStudy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ListPublished feeds the About and Consultancy pages — the same query
// result is used for both rather than two separate lookups, since it's one
// canonical list of proof-of-work either page can draw on.
func (r *CaseStudyRepo) ListPublished(ctx context.Context) ([]models.CaseStudy, error) {
	return r.listWhere(ctx, `WHERE is_published = TRUE`)
}

// ListAll feeds the admin console, which needs to see hidden rows too.
func (r *CaseStudyRepo) ListAll(ctx context.Context) ([]models.CaseStudy, error) {
	return r.listWhere(ctx, ``)
}

func (r *CaseStudyRepo) GetByID(ctx context.Context, id int64) (models.CaseStudy, error) {
	c, err := scanCaseStudy(r.db.QueryRowContext(ctx,
		`SELECT `+caseStudyColumns+` FROM case_studies WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return models.CaseStudy{}, ErrNotFound
	}
	return c, err
}

func (r *CaseStudyRepo) Create(ctx context.Context, c models.CaseStudy) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO case_studies (title, client_context, problem, approach, result, is_published, sort_order)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		c.Title, c.ClientContext, c.Problem, c.Approach, c.Result, c.IsPublished, c.SortOrder,
	).Scan(&id)
	return id, err
}

func (r *CaseStudyRepo) Update(ctx context.Context, c models.CaseStudy) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE case_studies SET title=$1, client_context=$2, problem=$3, approach=$4,
		result=$5, is_published=$6, sort_order=$7 WHERE id=$8`,
		c.Title, c.ClientContext, c.Problem, c.Approach, c.Result, c.IsPublished, c.SortOrder, c.ID,
	)
	return err
}

func (r *CaseStudyRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM case_studies WHERE id = $1`, id)
	return err
}
