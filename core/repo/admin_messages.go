package repo

import (
	"context"
	"database/sql"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

type AdminMessageRepo struct{ db *sql.DB }

func NewAdminMessageRepo(db *sql.DB) *AdminMessageRepo { return &AdminMessageRepo{db: db} }

const adminMessageColumns = `id, user_id, body, is_read, created_at`

func scanAdminMessage(row interface{ Scan(...any) error }) (models.AdminMessage, error) {
	var m models.AdminMessage
	err := row.Scan(&m.ID, &m.UserID, &m.Body, &m.IsRead, &m.CreatedAt)
	return m, err
}

func (r *AdminMessageRepo) Create(ctx context.Context, userID int64, body string) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO admin_messages (user_id, body) VALUES ($1,$2) RETURNING id`,
		userID, body,
	).Scan(&id)
	return id, err
}

// ListByUserID returns every message sent to a user, newest first.
func (r *AdminMessageRepo) ListByUserID(ctx context.Context, userID int64) ([]models.AdminMessage, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+adminMessageColumns+` FROM admin_messages WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.AdminMessage
	for rows.Next() {
		m, err := scanAdminMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *AdminMessageRepo) CountUnreadByUserID(ctx context.Context, userID int64) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM admin_messages WHERE user_id = $1 AND is_read = FALSE`, userID).Scan(&n)
	return n, err
}

// MarkAllReadByUserID marks every unread message for a user as read —
// called when they open /account, mirroring an inbox marking messages
// read on open.
func (r *AdminMessageRepo) MarkAllReadByUserID(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE admin_messages SET is_read = TRUE WHERE user_id = $1 AND is_read = FALSE`, userID)
	return err
}
