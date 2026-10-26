package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

// EmailTemplateRepo stores admin overrides and the last 20 versions of each.
type EmailTemplateRepo struct{ db *sql.DB }

func NewEmailTemplateRepo(db *sql.DB) *EmailTemplateRepo { return &EmailTemplateRepo{db: db} }

// Get returns ok=false when the kind still uses the built-in template.
func (r *EmailTemplateRepo) Get(ctx context.Context, kind string) (models.EmailTemplate, bool, error) {
	var t models.EmailTemplate
	err := r.db.QueryRowContext(ctx, `
		SELECT kind, subject, html, text_body, updated_at, updated_by
		FROM email_templates WHERE kind = $1`, kind).Scan(
		&t.Kind, &t.Subject, &t.HTML, &t.TextBody, &t.UpdatedAt, &t.UpdatedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return models.EmailTemplate{}, false, nil
	}
	if err != nil {
		return models.EmailTemplate{}, false, err
	}
	return t, true, nil
}

func (r *EmailTemplateRepo) List(ctx context.Context) ([]models.EmailTemplate, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT kind, subject, html, text_body, updated_at, updated_by
		FROM email_templates ORDER BY kind`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.EmailTemplate
	for rows.Next() {
		var t models.EmailTemplate
		if err := rows.Scan(&t.Kind, &t.Subject, &t.HTML, &t.TextBody, &t.UpdatedAt, &t.UpdatedBy); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Save snapshots the current row (when one exists) and keeps the newest 20
// versions, then upserts the template.
func (r *EmailTemplateRepo) Save(ctx context.Context, t models.EmailTemplate) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO email_template_versions (kind, subject, html, text_body, saved_by)
		SELECT kind, subject, html, text_body, updated_by
		FROM email_templates WHERE kind = $1`, t.Kind); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM email_template_versions
		WHERE kind = $1 AND id NOT IN (
			SELECT id FROM email_template_versions
			WHERE kind = $1
			ORDER BY saved_at DESC, id DESC
			LIMIT 20
		)`, t.Kind); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO email_templates (kind, subject, html, text_body, updated_by)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (kind) DO UPDATE SET
			subject = EXCLUDED.subject,
			html = EXCLUDED.html,
			text_body = EXCLUDED.text_body,
			updated_by = EXCLUDED.updated_by,
			updated_at = now()`,
		t.Kind, t.Subject, t.HTML, t.TextBody, t.UpdatedBy); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *EmailTemplateRepo) Delete(ctx context.Context, kind string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM email_templates WHERE kind = $1`, kind)
	return err
}

func (r *EmailTemplateRepo) Versions(ctx context.Context, kind string) ([]models.EmailTemplateVersion, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, kind, COALESCE(subject, ''), COALESCE(html, ''), COALESCE(text_body, ''), saved_at, saved_by
		FROM email_template_versions
		WHERE kind = $1
		ORDER BY saved_at DESC, id DESC`, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.EmailTemplateVersion
	for rows.Next() {
		var v models.EmailTemplateVersion
		if err := rows.Scan(&v.ID, &v.Kind, &v.Subject, &v.HTML, &v.TextBody, &v.SavedAt, &v.SavedBy); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Restore writes a previous version back through Save, which snapshots
// whatever is current first.
func (r *EmailTemplateRepo) Restore(ctx context.Context, kind string, versionID int64, by string) error {
	var v models.EmailTemplateVersion
	err := r.db.QueryRowContext(ctx, `
		SELECT id, kind, COALESCE(subject, ''), COALESCE(html, ''), COALESCE(text_body, ''), saved_at, saved_by
		FROM email_template_versions WHERE id = $1 AND kind = $2`, versionID, kind).Scan(
		&v.ID, &v.Kind, &v.Subject, &v.HTML, &v.TextBody, &v.SavedAt, &v.SavedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return r.Save(ctx, models.EmailTemplate{
		Kind: kind, Subject: v.Subject, HTML: v.HTML, TextBody: v.TextBody, UpdatedBy: by,
	})
}
