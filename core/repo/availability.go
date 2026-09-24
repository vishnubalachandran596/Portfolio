package repo

import (
	"context"
	"database/sql"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

type AvailabilityRepo struct{ db *sql.DB }

func NewAvailabilityRepo(db *sql.DB) *AvailabilityRepo { return &AvailabilityRepo{db: db} }

func (r *AvailabilityRepo) ListActive(ctx context.Context) ([]models.AvailabilityRule, error) {
	return r.list(ctx, `SELECT id, weekday, start_time::text, end_time::text, slot_minutes, active FROM availability_rules WHERE active = TRUE ORDER BY weekday`)
}

func (r *AvailabilityRepo) ListAll(ctx context.Context) ([]models.AvailabilityRule, error) {
	return r.list(ctx, `SELECT id, weekday, start_time::text, end_time::text, slot_minutes, active FROM availability_rules ORDER BY weekday`)
}

func (r *AvailabilityRepo) list(ctx context.Context, query string) ([]models.AvailabilityRule, error) {
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.AvailabilityRule
	for rows.Next() {
		var a models.AvailabilityRule
		var weekday int
		if err := rows.Scan(&a.ID, &weekday, &a.StartTime, &a.EndTime, &a.SlotMinutes, &a.Active); err != nil {
			return nil, err
		}
		a.Weekday = time.Weekday(weekday)
		// Postgres TIME comes back as "HH:MM:SS"; trim to "HH:MM".
		if len(a.StartTime) >= 5 {
			a.StartTime = a.StartTime[:5]
		}
		if len(a.EndTime) >= 5 {
			a.EndTime = a.EndTime[:5]
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *AvailabilityRepo) Create(ctx context.Context, a models.AvailabilityRule) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO availability_rules (weekday, start_time, end_time, slot_minutes, active)
		VALUES ($1,$2,$3,$4,$5) RETURNING id`,
		int(a.Weekday), a.StartTime, a.EndTime, a.SlotMinutes, a.Active,
	).Scan(&id)
	return id, err
}

func (r *AvailabilityRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM availability_rules WHERE id = $1`, id)
	return err
}

func (r *AvailabilityRepo) SetActive(ctx context.Context, id int64, active bool) error {
	_, err := r.db.ExecContext(ctx, `UPDATE availability_rules SET active = $1 WHERE id = $2`, active, id)
	return err
}
