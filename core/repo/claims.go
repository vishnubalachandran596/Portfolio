package repo

import (
	"context"
	"database/sql"
	"strings"
)

// ClaimedItem is one purchase just attached to an account.
type ClaimedItem struct {
	Kind  string // "course" or "software"
	Title string
	Slug  string
}

// ClaimRepo attaches guest purchases (user_id NULL, claim_code set) to an
// account. Both paths are atomic and single-use: a purchase is only ever
// updated while its user_id is still NULL, and its claim code is wiped in
// the same statement, so two concurrent requests cannot both win.
type ClaimRepo struct{ db *sql.DB }

func NewClaimRepo(db *sql.DB) *ClaimRepo { return &ClaimRepo{db: db} }

// ClaimByCode attaches the (course or software) purchase whose claim code is
// code (already normalised with authutil.NormalizeClaimCode) to userID.
// It returns ErrNotFound when no unclaimed paid purchase has that code.
func (r *ClaimRepo) ClaimByCode(ctx context.Context, userID int64, code string) (ClaimedItem, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return ClaimedItem{}, err
	}
	defer tx.Rollback()

	items, err := claimWhere(ctx, tx, userID, "claim_code = $2", code)
	if err != nil {
		return ClaimedItem{}, err
	}
	if len(items) == 0 {
		return ClaimedItem{}, ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return ClaimedItem{}, err
	}
	return items[0], nil
}

// AttachByEmail attaches every unclaimed paid purchase made with email to
// userID. Callers must ONLY pass an email that Google has just verified; a
// typed or password-signup email proves nothing and must never reach here.
func (r *ClaimRepo) AttachByEmail(ctx context.Context, userID int64, email string) ([]ClaimedItem, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	items, err := claimWhere(ctx, tx, userID, "lower(email) = $2", email)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return items, nil
}

// claimWhere runs the shared attach logic for both product lines. cond is a
// trusted fragment using $2 for the match value ($1 is the user id).
func claimWhere(ctx context.Context, tx *sql.Tx, userID int64, cond, val string) ([]ClaimedItem, error) {
	var out []ClaimedItem

	rows, err := tx.QueryContext(ctx, `
		WITH u AS (
			UPDATE purchases SET user_id = $1, claimed_at = now(), claim_code = ''
			WHERE `+cond+` AND user_id IS NULL AND claim_code <> '' AND status = 'paid'
			RETURNING id, course_id)
		SELECT 'course', c.title, c.slug FROM u JOIN courses c ON c.id = u.course_id`, userID, val)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var it ClaimedItem
		if err := rows.Scan(&it.Kind, &it.Title, &it.Slug); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Software: attach the purchase and the licence created for it. The
	// licence UPDATE joins on the purchases just claimed (user_id = $1 and
	// claimed in this statement chain), so it only touches their licences.
	rows, err = tx.QueryContext(ctx, `
		WITH u AS (
			UPDATE software_purchases SET user_id = $1, claimed_at = now(), claim_code = ''
			WHERE `+cond+` AND user_id IS NULL AND claim_code <> '' AND status = 'paid'
			RETURNING id, software_id),
		l AS (
			UPDATE licences SET user_id = $1, updated_at = now()
			WHERE user_id IS NULL AND purchase_id IN (SELECT id FROM u)
			RETURNING id)
		SELECT 'software', s.title, s.slug FROM u JOIN software s ON s.id = u.software_id`, userID, val)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var it ClaimedItem
		if err := rows.Scan(&it.Kind, &it.Title, &it.Slug); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, it)
	}
	rows.Close()
	return out, rows.Err()
}
