package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

type AdminRepo struct{ db *sql.DB }

func NewAdminRepo(db *sql.DB) *AdminRepo { return &AdminRepo{db: db} }

func (r *AdminRepo) GetByEmail(ctx context.Context, email string) (models.Admin, error) {
	var a models.Admin
	err := r.db.QueryRowContext(ctx, `SELECT id, email, password_hash, name, created_at FROM admins WHERE email = $1`, email).
		Scan(&a.ID, &a.Email, &a.PasswordHash, &a.Name, &a.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return models.Admin{}, ErrNotFound
	}
	return a, err
}

func (r *AdminRepo) Count(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM admins`).Scan(&n)
	return n, err
}

func (r *AdminRepo) Create(ctx context.Context, email, passwordHash, name string) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO admins (email, password_hash, name) VALUES ($1,$2,$3) RETURNING id`,
		email, passwordHash, name,
	).Scan(&id)
	return id, err
}

func (r *AdminRepo) UpdatePassword(ctx context.Context, id int64, passwordHash string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE admins SET password_hash = $1 WHERE id = $2`, passwordHash, id)
	return err
}
