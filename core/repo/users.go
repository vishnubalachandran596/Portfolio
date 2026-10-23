package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

type UserRepo struct{ db *sql.DB }

func NewUserRepo(db *sql.DB) *UserRepo { return &UserRepo{db: db} }

const userColumns = `id, email, password_hash, name, is_guest, created_at, token_version, password_usable, google_verified, security_question, security_answer_hash`

func scanUser(row interface{ Scan(...any) error }) (models.User, error) {
	var u models.User
	err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.IsGuest, &u.CreatedAt,
		&u.TokenVersion, &u.PasswordUsable, &u.GoogleVerified, &u.SecurityQuestion, &u.SecurityAnswerHash)
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

// CreateWithPassword registers a visitor who chose their own password and
// security question. The email is not verified (the site sends no email).
func (r *UserRepo) CreateWithPassword(ctx context.Context, email, passwordHash, name, question, answerHash string) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO users (email, password_hash, name, password_usable, security_question, security_answer_hash)
		VALUES ($1,$2,$3,true,$4,$5) RETURNING id`,
		email, passwordHash, name, question, answerHash,
	).Scan(&id)
	if err != nil && strings.Contains(err.Error(), "duplicate key value") {
		return 0, ErrEmailTaken
	}
	return id, err
}

// ClaimGuest turns an anonymous thumbs-up guest row into a password account.
// The WHERE clause makes it a no-op for any real account, so a sign-up can
// never take over somebody else's row.
func (r *UserRepo) ClaimGuest(ctx context.Context, id int64, passwordHash, name, question, answerHash string) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE users SET password_hash=$2, name=CASE WHEN $3='' THEN name ELSE $3 END,
			password_usable=true, is_guest=false, security_question=$4, security_answer_hash=$5,
			token_version=token_version+1
		WHERE id=$1 AND is_guest=true AND password_usable=false AND google_verified=false`,
		id, passwordHash, name, question, answerHash)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// SetPassword replaces the password and signs the user out everywhere.
func (r *UserRepo) SetPassword(ctx context.Context, id int64, passwordHash string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE users SET password_hash=$2, password_usable=true, token_version=token_version+1 WHERE id=$1`,
		id, passwordHash)
	return err
}

// SetSecurity stores the security question and the hash of its answer.
func (r *UserRepo) SetSecurity(ctx context.Context, id int64, question, answerHash string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET security_question=$2, security_answer_hash=$3 WHERE id=$1`,
		id, question, answerHash)
	return err
}

// TokenVersion returns the current session version for a user, or false when
// the user no longer exists.
func (r *UserRepo) TokenVersion(ctx context.Context, id int64) (int, bool) {
	var tv int
	if err := r.db.QueryRowContext(ctx, `SELECT token_version FROM users WHERE id=$1`, id).Scan(&tv); err != nil {
		return 0, false
	}
	return tv, true
}

// MarkGoogleVerified records that Google proved control of the email. If the
// account was an unverified password account, the password and security
// answer are cleared (the real owner takes over) and old sessions die.
// Returns true when such a takeover happened.
func (r *UserRepo) MarkGoogleVerified(ctx context.Context, id int64) (bool, error) {
	var tookOver bool
	err := r.db.QueryRowContext(ctx, `
		WITH old AS (SELECT id, google_verified, password_usable FROM users WHERE id=$1 FOR UPDATE)
		UPDATE users u SET
			google_verified = true,
			password_usable = CASE WHEN old.google_verified THEN u.password_usable ELSE false END,
			security_question = CASE WHEN old.google_verified THEN u.security_question ELSE '' END,
			security_answer_hash = CASE WHEN old.google_verified THEN u.security_answer_hash ELSE '' END,
			token_version = CASE WHEN old.google_verified OR NOT old.password_usable THEN u.token_version ELSE u.token_version+1 END
		FROM old WHERE u.id = old.id
		RETURNING (NOT old.google_verified AND old.password_usable)`, id).Scan(&tookOver)
	return tookOver, err
}

// ErrDeleteBlocked means the account can't be deleted yet; the message is
// safe to show to the account holder.
var ErrDeleteBlocked = errors.New("account deletion blocked")

// DeleteAccount erases a visitor's personal data in one transaction. Rows
// that must be kept for accounting (payments, bookings, contributions,
// quiz attempts) are anonymised rather than removed; testimonials and
// thumbs-up are deleted; the user row is deleted last (remaining FKs are
// ON DELETE SET NULL / CASCADE). Admins and users with an upcoming booking
// are refused.
func (r *UserRepo) DeleteAccount(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var email string
	if err := tx.QueryRowContext(ctx, `SELECT email FROM users WHERE id = $1 FOR UPDATE`, id).Scan(&email); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	var isAdmin bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM admins WHERE lower(email) = lower($1))`, email).Scan(&isAdmin); err != nil {
		return err
	}
	if isAdmin {
		return ErrDeleteBlocked
	}
	var upcoming bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM bookings WHERE user_id = $1 AND status IN ('confirmed','pending_payment') AND slot_start > now())`, id).Scan(&upcoming); err != nil {
		return err
	}
	if upcoming {
		return ErrDeleteBlocked
	}

	anon := fmt.Sprintf("deleted-%d@deleted.invalid", id)
	stmts := []string{
		`UPDATE bookings SET name = 'Deleted user', email = $2, phone = '', manage_token_hash = md5(random()::text || clock_timestamp()::text) || md5(random()::text) WHERE user_id = $1`,
		`UPDATE purchases SET email = $2, access_token = 'scrubbed-' || md5(random()::text || clock_timestamp()::text), access_token_hash = md5(random()::text || clock_timestamp()::text) || md5(random()::text), claim_code = '' WHERE user_id = $1`,
		`UPDATE software_purchases SET email = $2, access_token = 'scrubbed-' || md5(random()::text || clock_timestamp()::text), access_token_hash = md5(random()::text || clock_timestamp()::text) || md5(random()::text), claim_code = '' WHERE user_id = $1`,
		`UPDATE article_contributions SET email = $2 WHERE user_id = $1`,
		`UPDATE course_contributions SET email = $2 WHERE user_id = $1`,
		`UPDATE quiz_attempts SET name = 'Deleted user', email = $2 WHERE user_id = $1`,
		`DELETE FROM testimonials WHERE user_id = $1`,
		`DELETE FROM article_thumbs_up WHERE user_id = $1`,
	}
	for _, s := range stmts {
		args := []any{id}
		if strings.Contains(s, "$2") {
			args = append(args, anon)
		}
		if _, err := tx.ExecContext(ctx, s, args...); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, id); err != nil {
		return err
	}
	return tx.Commit()
}
