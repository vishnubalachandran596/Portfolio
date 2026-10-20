package repo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/authutil"
	"github.com/vishnubalachandran596/portfolio/core/models"
)

type LicenceRepo struct{ db *sql.DB }

func NewLicenceRepo(db *sql.DB) *LicenceRepo { return &LicenceRepo{db: db} }

const licenceSelect = `
	SELECT l.id, l.lic_code, l.software_id, s.title, COALESCE(l.purchase_id,0), COALESCE(l.user_id,0), COALESCE(u.email,''),
	       l.ver, l.device_hash, l.key_text, l.kid, l.iat, l.revoked_at, l.revoked_reason, l.note, l.added_by_admin, l.created_at
	FROM licences l
	JOIN software s ON s.id = l.software_id
	LEFT JOIN users u ON u.id = l.user_id `

func scanLicence(row interface{ Scan(...any) error }) (models.Licence, error) {
	var l models.Licence
	var iat, rev sql.NullTime
	err := row.Scan(&l.ID, &l.Code, &l.SoftwareID, &l.SoftwareTitle, &l.PurchaseID, &l.UserID, &l.UserEmail,
		&l.Ver, &l.DeviceHash, &l.Key, &l.KID, &iat, &rev, &l.RevokedReason, &l.Note, &l.AddedByAdmin, &l.CreatedAt)
	if iat.Valid {
		l.IssuedAt = &iat.Time
	}
	if rev.Valid {
		l.RevokedAt = &rev.Time
	}
	return l, err
}

func newLicenceCode() (string, error) {
	h, err := authutil.RandomHex(5)
	if err != nil {
		return "", err
	}
	h = strings.ToUpper(h)
	return "TAB-" + h[:5] + "-" + h[5:], nil
}

// EnsureForPurchase creates the single licence for a paid purchase. Safe to
// call repeatedly (the partial unique index on purchase_id makes a second
// call a no-op).
func (r *LicenceRepo) EnsureForPurchase(ctx context.Context, softwareID, purchaseID, userID int64) error {
	code, err := newLicenceCode()
	if err != nil {
		return err
	}
	var uid any
	if userID != 0 {
		uid = userID
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO licences (lic_code, software_id, purchase_id, user_id)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (purchase_id) WHERE purchase_id IS NOT NULL DO NOTHING`,
		code, softwareID, purchaseID, uid)
	return err
}

// AddByAdmin creates a licence with no order behind it.
func (r *LicenceRepo) AddByAdmin(ctx context.Context, softwareID, userID int64, note string) (int64, error) {
	code, err := newLicenceCode()
	if err != nil {
		return 0, err
	}
	var id int64
	err = r.db.QueryRowContext(ctx, `
		INSERT INTO licences (lic_code, software_id, user_id, note, added_by_admin)
		VALUES ($1,$2,$3,$4,TRUE) RETURNING id`, code, softwareID, userID, note).Scan(&id)
	if err == nil {
		_ = r.Event(ctx, id, "added", "admin", note)
	}
	return id, err
}

func (r *LicenceRepo) Get(ctx context.Context, id int64) (models.Licence, error) {
	l, err := scanLicence(r.db.QueryRowContext(ctx, licenceSelect+`WHERE l.id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return l, ErrNotFound
	}
	return l, err
}

// GetByPurchase returns the licence a purchase created (ErrNotFound when
// the item has no licence product or creation failed).
func (r *LicenceRepo) GetByPurchase(ctx context.Context, purchaseID int64) (models.Licence, error) {
	l, err := scanLicence(r.db.QueryRowContext(ctx, licenceSelect+`WHERE l.purchase_id = $1`, purchaseID))
	if errors.Is(err, sql.ErrNoRows) {
		return l, ErrNotFound
	}
	return l, err
}

func (r *LicenceRepo) ListByUser(ctx context.Context, userID int64) ([]models.Licence, error) {
	return r.list(ctx, licenceSelect+`WHERE l.user_id = $1 ORDER BY l.id DESC`, userID)
}

// Search lists licences newest first; q matches code, owner email or device.
func (r *LicenceRepo) Search(ctx context.Context, q string, limit int) ([]models.Licence, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return r.list(ctx, licenceSelect+`ORDER BY l.id DESC LIMIT $1`, limit)
	}
	like := "%" + strings.ToLower(q) + "%"
	return r.list(ctx, licenceSelect+`
		WHERE lower(l.lic_code) LIKE $1 OR lower(COALESCE(u.email,'')) LIKE $1 OR l.device_hash LIKE $1
		ORDER BY l.id DESC LIMIT $2`, like, limit)
}

func (r *LicenceRepo) list(ctx context.Context, query string, args ...any) ([]models.Licence, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Licence
	for rows.Next() {
		l, err := scanLicence(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// SetKey stores a freshly signed key for a licence and clears any revoke
// (a reissue is the only way back). The event row records who and why.
func (r *LicenceRepo) SetKey(ctx context.Context, id int64, ver, device, key, kid string, iat time.Time, event, actor, detail string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		UPDATE licences SET ver=$1, device_hash=$2, key_text=$3, kid=$4, iat=$5,
			revoked_at=NULL, revoked_reason='', updated_at=now()
		WHERE id=$6`, ver, device, key, kid, iat, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO licence_events (licence_id, event, actor, detail) VALUES ($1,$2,$3,$4)`,
		id, event, actor, detail); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *LicenceRepo) Revoke(ctx context.Context, id int64, reason string) error {
	if _, err := r.db.ExecContext(ctx, `UPDATE licences SET revoked_at=now(), revoked_reason=$1, updated_at=now() WHERE id=$2 AND revoked_at IS NULL`, reason, id); err != nil {
		return err
	}
	return r.Event(ctx, id, "revoked", "admin", reason)
}

func (r *LicenceRepo) Event(ctx context.Context, id int64, event, actor, detail string) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO licence_events (licence_id, event, actor, detail) VALUES ($1,$2,$3,$4)`, id, event, actor, detail)
	return err
}

// SelfDeviceChanges counts the owner's own device changes in the window.
func (r *LicenceRepo) SelfDeviceChanges(ctx context.Context, id int64, since time.Time) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM licence_events WHERE licence_id=$1 AND event='device_change' AND actor='user' AND created_at >= $2`, id, since).Scan(&n)
	return n, err
}
