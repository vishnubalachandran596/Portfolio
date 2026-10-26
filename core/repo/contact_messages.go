package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

type ContactMessageRepo struct{ db *sql.DB }

func NewContactMessageRepo(db *sql.DB) *ContactMessageRepo { return &ContactMessageRepo{db: db} }

const contactMessageColumns = `id, name, email, phone, message, is_read, created_at`

func scanContactMessage(row interface{ Scan(...any) error }) (models.ContactMessage, error) {
	var m models.ContactMessage
	err := row.Scan(&m.ID, &m.Name, &m.Email, &m.Phone, &m.Message, &m.IsRead, &m.CreatedAt)
	return m, err
}

// Create records a new /contact form submission.
func (r *ContactMessageRepo) Create(ctx context.Context, m models.ContactMessage) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO contact_messages (name, email, phone, message) VALUES ($1,$2,$3,$4) RETURNING id`,
		m.Name, m.Email, m.Phone, m.Message,
	).Scan(&id)
	return id, err
}

// GetByID loads one submission so a queued confirmation can be rebuilt
// without storing the rendered email.
func (r *ContactMessageRepo) GetByID(ctx context.Context, id int64) (models.ContactMessage, error) {
	m, err := scanContactMessage(r.db.QueryRowContext(ctx, `SELECT `+contactMessageColumns+` FROM contact_messages WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return models.ContactMessage{}, ErrNotFound
	}
	return m, err
}

// ListAll returns every contact message, newest first, for the admin inbox.
func (r *ContactMessageRepo) ListAll(ctx context.Context) ([]models.ContactMessage, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+contactMessageColumns+` FROM contact_messages ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ContactMessage
	for rows.Next() {
		m, err := scanContactMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// MarkRead flips a message to read — the admin list's "mark as read" action.
func (r *ContactMessageRepo) MarkRead(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE contact_messages SET is_read = true WHERE id = $1`, id)
	return err
}
