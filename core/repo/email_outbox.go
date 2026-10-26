package repo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

// EmailOutboxRepo stores delivery metadata only. Bodies are never written.
type EmailOutboxRepo struct{ db *sql.DB }

func NewEmailOutboxRepo(db *sql.DB) *EmailOutboxRepo { return &EmailOutboxRepo{db: db} }

const emailOutboxColumns = `id, kind, entity_id, to_email, provider, status, attempts, last_error, provider_id, created_at, updated_at, sent_at`

func scanEmailOutbox(row interface{ Scan(...any) error }) (models.EmailOutbox, error) {
	var e models.EmailOutbox
	var sent sql.NullTime
	err := row.Scan(&e.ID, &e.Kind, &e.EntityID, &e.ToEmail, &e.Provider, &e.Status, &e.Attempts,
		&e.LastError, &e.ProviderID, &e.CreatedAt, &e.UpdatedAt, &sent)
	if sent.Valid {
		e.SentAt = &sent.Time
	}
	return e, err
}

// TryEnqueue inserts a pending row. A conflict on (kind, entity_id) returns
// false and does not change the existing row, so a repeated verify sends once.
func (r *EmailOutboxRepo) TryEnqueue(ctx context.Context, kind, entityID, toEmail, provider string) (bool, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO email_outbox (kind, entity_id, to_email, provider, status)
		VALUES ($1, $2, $3, $4, 'pending')
		ON CONFLICT (kind, entity_id) DO NOTHING
		RETURNING id`, kind, entityID, toEmail, provider).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (r *EmailOutboxRepo) MarkSent(ctx context.Context, kind, entityID, providerID string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE email_outbox
		SET status = 'sent', provider_id = $3, sent_at = now(), updated_at = now(), last_error = ''
		WHERE kind = $1 AND entity_id = $2`, kind, entityID, providerID)
	return err
}

func (r *EmailOutboxRepo) MarkFailed(ctx context.Context, kind, entityID, lastError string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE email_outbox
		SET status = 'failed', attempts = attempts + 1, last_error = $3, updated_at = now()
		WHERE kind = $1 AND entity_id = $2`, kind, entityID, lastError)
	return err
}

func (r *EmailOutboxRepo) MarkDisabled(ctx context.Context, kind, entityID string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE email_outbox
		SET status = 'disabled', updated_at = now(), last_error = ''
		WHERE kind = $1 AND entity_id = $2`, kind, entityID)
	return err
}

// Requeue puts a row back to pending so the worker sends it. Attempts
// reset, including a row that already hit the cap. Already-pending rows
// stay pending.
func (r *EmailOutboxRepo) Requeue(ctx context.Context, kind, entityID string) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE email_outbox
		SET status = 'pending', attempts = 0, last_error = '', provider_id = '',
		    sent_at = NULL, updated_at = now()
		WHERE kind = $1 AND entity_id = $2`, kind, entityID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// ListRetryable is the worker queue: pending rows, then failed rows
// under the attempt cap. Send-test is sent in the admin request and is
// excluded. There is no age delay — a new pending row is eligible on
// the next worker run.
func (r *EmailOutboxRepo) ListRetryable(ctx context.Context, limit int) ([]models.EmailOutbox, error) {
	if limit <= 0 {
		limit = 8
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+emailOutboxColumns+`
		FROM email_outbox
		WHERE status IN ('failed', 'pending')
		  AND attempts < 5
		  AND kind <> $1
		ORDER BY CASE WHEN status = 'failed' THEN 0 ELSE 1 END, created_at
		LIMIT $2`, models.EmailSendTest, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.EmailOutbox
	for rows.Next() {
		e, err := scanEmailOutbox(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *EmailOutboxRepo) Get(ctx context.Context, kind, entityID string) (models.EmailOutbox, error) {
	e, err := scanEmailOutbox(r.db.QueryRowContext(ctx, `
		SELECT `+emailOutboxColumns+` FROM email_outbox WHERE kind = $1 AND entity_id = $2`, kind, entityID))
	if errors.Is(err, sql.ErrNoRows) {
		return models.EmailOutbox{}, ErrNotFound
	}
	return e, err
}

func (r *EmailOutboxRepo) ListAll(ctx context.Context) ([]models.EmailOutbox, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+emailOutboxColumns+` FROM email_outbox ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.EmailOutbox
	for rows.Next() {
		e, err := scanEmailOutbox(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *EmailOutboxRepo) CountSince(ctx context.Context, kind string, since time.Time) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM email_outbox WHERE kind = $1 AND created_at >= $2`, kind, since).Scan(&n)
	return n, err
}
