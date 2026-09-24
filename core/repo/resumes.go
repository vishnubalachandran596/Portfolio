package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

type RoleResumeRepo struct{ db *sql.DB }

func NewRoleResumeRepo(db *sql.DB) *RoleResumeRepo { return &RoleResumeRepo{db: db} }

func (r *RoleResumeRepo) List(ctx context.Context) ([]models.RoleResume, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT role_key, label, object_path, sort_order
		FROM role_resumes ORDER BY sort_order, role_key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.RoleResume
	for rows.Next() {
		var row models.RoleResume
		if err := rows.Scan(&row.RoleKey, &row.Label, &row.ObjectPath, &row.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *RoleResumeRepo) Get(ctx context.Context, roleKey string) (models.RoleResume, error) {
	var row models.RoleResume
	err := r.db.QueryRowContext(ctx, `
		SELECT role_key, label, object_path, sort_order
		FROM role_resumes WHERE role_key = $1`, roleKey).
		Scan(&row.RoleKey, &row.Label, &row.ObjectPath, &row.SortOrder)
	if errors.Is(err, sql.ErrNoRows) {
		return models.RoleResume{}, ErrNotFound
	}
	return row, err
}

func (r *RoleResumeRepo) Upsert(ctx context.Context, row models.RoleResume) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO role_resumes (role_key, label, object_path, sort_order)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (role_key) DO UPDATE SET
			label = EXCLUDED.label,
			object_path = EXCLUDED.object_path,
			sort_order = EXCLUDED.sort_order`,
		row.RoleKey, row.Label, row.ObjectPath, row.SortOrder,
	)
	return err
}

func (r *RoleResumeRepo) Delete(ctx context.Context, roleKey string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM role_resumes WHERE role_key = $1`, roleKey)
	return err
}
