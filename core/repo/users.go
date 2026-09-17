package repo

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

type UserRepo struct{ db *sql.DB }

func NewUserRepo(db *sql.DB) *UserRepo { return &UserRepo{db: db} }

func scanUser(row interface{ Scan(...any) error }) (models.User, error) {
	var u models.User
	err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.CreatedAt)
	return u, err
}

func (r *UserRepo) GetByEmail(ctx context.Context, email string) (models.User, error) {
	u, err := scanUser(r.db.QueryRowContext(ctx, `SELECT id, email, password_hash, name, created_at FROM users WHERE email = $1`, email))
	if errors.Is(err, sql.ErrNoRows) {
		return models.User{}, ErrNotFound
	}
	return u, err
}

func (r *UserRepo) GetByID(ctx context.Context, id int64) (models.User, error) {
	u, err := scanUser(r.db.QueryRowContext(ctx, `SELECT id, email, password_hash, name, created_at FROM users WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return models.User{}, ErrNotFound
	}
	return u, err
}

// ErrEmailTaken is returned by Create when the email is already registered.
var ErrEmailTaken = errors.New("email already registered")

func (r *UserRepo) Create(ctx context.Context, email, passwordHash, name string) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO users (email, password_hash, name) VALUES ($1,$2,$3) RETURNING id`,
		email, passwordHash, name,
	).Scan(&id)
	if err != nil && strings.Contains(err.Error(), "duplicate key value") {
		return 0, ErrEmailTaken
	}
	return id, err
}
