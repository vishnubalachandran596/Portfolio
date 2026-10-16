package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

// HighlightRepo backs the home page's stat strip (profile_highlights,
// migration 078). Same list/get/create/update/delete shape as the other
// small curated lists (certifications, publications).
type HighlightRepo struct{ db *sql.DB }

func NewHighlightRepo(db *sql.DB) *HighlightRepo { return &HighlightRepo{db: db} }

const highlightColumns = `id, value, label, sort_order, is_published`

func scanHighlight(row interface{ Scan(...any) error }) (models.ProfileHighlight, error) {
	var h models.ProfileHighlight
	err := row.Scan(&h.ID, &h.Value, &h.Label, &h.SortOrder, &h.IsPublished)
	return h, err
}

func (r *HighlightRepo) list(ctx context.Context, where string) ([]models.ProfileHighlight, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+highlightColumns+` FROM profile_highlights `+where+` ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ProfileHighlight
	for rows.Next() {
		h, err := scanHighlight(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (r *HighlightRepo) ListPublished(ctx context.Context) ([]models.ProfileHighlight, error) {
	return r.list(ctx, `WHERE is_published = TRUE`)
}

func (r *HighlightRepo) ListAll(ctx context.Context) ([]models.ProfileHighlight, error) {
	return r.list(ctx, ``)
}

func (r *HighlightRepo) GetByID(ctx context.Context, id int64) (models.ProfileHighlight, error) {
	h, err := scanHighlight(r.db.QueryRowContext(ctx, `SELECT `+highlightColumns+` FROM profile_highlights WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return models.ProfileHighlight{}, ErrNotFound
	}
	return h, err
}

func (r *HighlightRepo) Create(ctx context.Context, h models.ProfileHighlight) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO profile_highlights (value, label, sort_order, is_published) VALUES ($1, $2, $3, $4) RETURNING id`,
		h.Value, h.Label, h.SortOrder, h.IsPublished).Scan(&id)
	return id, err
}

func (r *HighlightRepo) Update(ctx context.Context, h models.ProfileHighlight) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE profile_highlights SET value = $1, label = $2, sort_order = $3, is_published = $4 WHERE id = $5`,
		h.Value, h.Label, h.SortOrder, h.IsPublished, h.ID)
	return err
}

func (r *HighlightRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM profile_highlights WHERE id = $1`, id)
	return err
}
