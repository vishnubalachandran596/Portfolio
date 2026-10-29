package repo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

// AuthTokenRepo stores the emailed one-time links for sign-up activation
// and password reset. Only the hash of a token is ever stored.
type AuthTokenRepo struct{ db *sql.DB }

func NewAuthTokenRepo(db *sql.DB) *AuthTokenRepo { return &AuthTokenRepo{db: db} }

const authTokenColumns = `id, purpose, email, name, password_hash, next_path, expires_at`

func scanAuthToken(row interface{ Scan(...any) error }) (models.AuthToken, error) {
	var t models.AuthToken
	err := row.Scan(&t.ID, &t.Purpose, &t.Email, &t.Name, &t.PasswordHash, &t.NextPath, &t.ExpiresAt)
	return t, err
}

// Create stores a new token and drops any earlier unused token of the same
// purpose for the same email, so only the newest link works. Tokens that
// expired more than a week ago are cleaned up on the way.
func (r *AuthTokenRepo) Create(ctx context.Context, purpose, email, tokenHash, name, passwordHash, nextPath string, ttl time.Duration) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM auth_tokens WHERE (purpose=$1 AND email=$2 AND used_at IS NULL) OR expires_at < now() - interval '7 days'`, purpose, email); err != nil {
		return 0, err
	}
	var id int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO auth_tokens (purpose, email, token_hash, name, password_hash, next_path, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6, now() + ($7 * interval '1 second')) RETURNING id`,
		purpose, email, tokenHash, name, passwordHash, nextPath, int64(ttl.Seconds())).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

// Peek reads a live token without using it (the reset page shows its form
// first, so a mail scanner that opens the link cannot burn it).
func (r *AuthTokenRepo) Peek(ctx context.Context, purpose, tokenHash string) (models.AuthToken, error) {
	t, err := scanAuthToken(r.db.QueryRowContext(ctx, `
		SELECT `+authTokenColumns+` FROM auth_tokens
		WHERE token_hash=$1 AND purpose=$2 AND used_at IS NULL AND expires_at > now()`, tokenHash, purpose))
	if errors.Is(err, sql.ErrNoRows) {
		return models.AuthToken{}, ErrNotFound
	}
	return t, err
}

// Consume marks a live token used and returns it. A second call with the
// same token, or a call after expiry, returns ErrNotFound.
func (r *AuthTokenRepo) Consume(ctx context.Context, purpose, tokenHash string) (models.AuthToken, error) {
	t, err := scanAuthToken(r.db.QueryRowContext(ctx, `
		UPDATE auth_tokens SET used_at = now()
		WHERE token_hash=$1 AND purpose=$2 AND used_at IS NULL AND expires_at > now()
		RETURNING `+authTokenColumns, tokenHash, purpose))
	if errors.Is(err, sql.ErrNoRows) {
		return models.AuthToken{}, ErrNotFound
	}
	return t, err
}
