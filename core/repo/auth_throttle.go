package repo

import (
	"context"
	"database/sql"
	"time"
)

// AuthThrottleRepo keeps failed-attempt counters in Postgres so that login,
// password-reset and admin-login limits hold across serverless instances.
type AuthThrottleRepo struct{ db *sql.DB }

func NewAuthThrottleRepo(db *sql.DB) *AuthThrottleRepo { return &AuthThrottleRepo{db: db} }

// Locked returns when the key is unlocked again, or the zero time when it is
// not currently locked.
func (r *AuthThrottleRepo) Locked(ctx context.Context, kind, key string) (time.Time, error) {
	var until sql.NullTime
	err := r.db.QueryRowContext(ctx,
		`SELECT locked_until FROM auth_throttle WHERE kind=$1 AND key=$2`, kind, key).Scan(&until)
	if err == sql.ErrNoRows {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	if until.Valid && until.Time.After(time.Now()) {
		return until.Time, nil
	}
	return time.Time{}, nil
}

// Fail records one failed attempt. Counts older than an hour start again.
// After maxFails failures the key is locked for 1 minute, doubling with
// every further failure up to 60 minutes.
func (r *AuthThrottleRepo) Fail(ctx context.Context, kind, key string, maxFails int) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO auth_throttle (kind, key, fail_count, last_fail_at, locked_until)
		VALUES ($1, $2, 1, now(), NULL)
		ON CONFLICT (kind, key) DO UPDATE SET
			fail_count   = CASE WHEN auth_throttle.last_fail_at < now() - interval '1 hour'
			                    THEN 1 ELSE auth_throttle.fail_count + 1 END,
			last_fail_at = now(),
			locked_until = CASE
				WHEN (CASE WHEN auth_throttle.last_fail_at < now() - interval '1 hour'
				           THEN 1 ELSE auth_throttle.fail_count + 1 END) >= $3
				THEN now() + LEAST(interval '60 minutes',
				     interval '1 minute' * power(2, LEAST(10, (CASE WHEN auth_throttle.last_fail_at < now() - interval '1 hour'
				           THEN 1 ELSE auth_throttle.fail_count + 1 END) - $3)))
				ELSE NULL END`,
		kind, key, maxFails)
	return err
}

// Clear forgets a key after a successful attempt.
func (r *AuthThrottleRepo) Clear(ctx context.Context, kind, key string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM auth_throttle WHERE kind=$1 AND key=$2`, kind, key)
	return err
}
