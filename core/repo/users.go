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

const userColumns = `id, email, password_hash, name, is_guest, created_at`

func scanUser(row interface{ Scan(...any) error }) (models.User, error) {
	var u models.User
	err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.IsGuest, &u.CreatedAt)
	return u, err
}

func (r *UserRepo) GetByEmail(ctx context.Context, email string) (models.User, error) {
	u, err := scanUser(r.db.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE email = $1`, email))
	if errors.Is(err, sql.ErrNoRows) {
		return models.User{}, ErrNotFound
	}
	return u, err
}

func (r *UserRepo) GetByID(ctx context.Context, id int64) (models.User, error) {
	u, err := scanUser(r.db.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return models.User{}, ErrNotFound
	}
	return u, err
}

// ListAll returns every registered visitor account, newest first — for
// the admin Users list.
func (r *UserRepo) ListAll(ctx context.Context) ([]models.User, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+userColumns+` FROM users ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
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

// GetOrCreateGuest is Create's counterpart for an anonymous article
// thumbs-up that included an email: reuses the existing account if that
// email is already registered (guest or real — either way, the vote
// should dedupe against the same person), otherwise creates a new
// is_guest=true row. Atomic via ON CONFLICT, so a race between two
// requests for the same brand-new email can't create two rows.
func (r *UserRepo) GetOrCreateGuest(ctx context.Context, email, passwordHash, name string) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO users (email, password_hash, name, is_guest) VALUES ($1,$2,$3,true)
		ON CONFLICT (email) DO UPDATE SET name = CASE WHEN users.name = '' THEN EXCLUDED.name ELSE users.name END
		RETURNING id`,
		email, passwordHash, name,
	).Scan(&id)
	return id, err
}

// MarkSignedUp clears the is_guest flag once someone who was captured
// via the anonymous-vote form's email field actually signs in for real
// (Google OAuth) — their guest history (votes, etc.) stays on the same
// account, it's just no longer labeled "not signed up" in the admin view.
func (r *UserRepo) MarkSignedUp(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET is_guest = false WHERE id = $1 AND is_guest = true`, id)
	return err
}
