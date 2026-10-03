package repo

import (
	"context"
	"database/sql"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

type SubscriberRepo struct{ db *sql.DB }

func NewSubscriberRepo(db *sql.DB) *SubscriberRepo { return &SubscriberRepo{db: db} }

// Create records a new subscriber, reusing isUniqueViolation (bookings.go)
// rather than a second error-classification helper, same as promo_codes.go
// already does. A repeat email is not an error from the
// caller's point of view — the visitor is already on the list, which reads
// to them exactly like success — so this returns (false, nil) rather than
// a distinguishable error the handler would have to swallow anyway.
func (r *SubscriberRepo) Create(ctx context.Context, email, sourcePage string) (created bool, err error) {
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO subscribers (email, source_page) VALUES ($1, $2)`, email, sourcePage)
	if err != nil {
		if isUniqueViolation(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// ListAll feeds the admin console, newest first.
func (r *SubscriberRepo) ListAll(ctx context.Context) ([]models.Subscriber, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, email, source_page, created_at FROM subscribers ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Subscriber
	for rows.Next() {
		var s models.Subscriber
		if err := rows.Scan(&s.ID, &s.Email, &s.SourcePage, &s.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *SubscriberRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM subscribers WHERE id = $1`, id)
	return err
}
