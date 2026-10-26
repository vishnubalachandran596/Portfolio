package db

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestEmailMigrationsAreIdempotentSchema(t *testing.T) {
	for _, name := range []string{"094_email_outbox.sql", "095_email_templates.sql"} {
		raw, err := migrationFiles.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		sqlText := string(raw)
		if !strings.Contains(sqlText, "CREATE TABLE IF NOT EXISTS") {
			t.Fatalf("%s: missing IF NOT EXISTS", name)
		}
		if !strings.Contains(sqlText, "ENABLE ROW LEVEL SECURITY") || !strings.Contains(sqlText, "to_regclass") {
			t.Fatalf("%s: RLS guard missing", name)
		}
		if !strings.Contains(sqlText, "unique_violation") || !strings.Contains(sqlText, "relkind") {
			t.Fatalf("%s: pg_type guard missing", name)
		}
		if strings.Contains(sqlText, "DROP TABLE") {
			t.Fatalf("%s: must not drop a table", name)
		}
		if strings.Contains(sqlText, "DROP TYPE") && !strings.Contains(sqlText, "rel_kind = 'c'") {
			t.Fatalf("%s: type drop is not limited to a non-table composite", name)
		}
		stripped := stripSQLComments(sqlText)
		if stmt := sqlDML.FindString(stripped); stmt != "" {
			t.Fatalf("%s: content statement %q", name, stmt)
		}
	}
}

var sqlDML = regexp.MustCompile(`(?i)(?:^|[;\n])\s*(INSERT|UPDATE|DELETE)\b`)

func stripSQLComments(sqlText string) string {
	sqlText = regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(sqlText, "")
	lines := strings.Split(sqlText, "\n")
	for i, line := range lines {
		if idx := strings.Index(line, "--"); idx >= 0 {
			lines[i] = line[:idx]
		}
	}
	return strings.Join(lines, "\n")
}

func TestEmailMigrationsApplyTwiceOnLocalDB(t *testing.T) {
	rawURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if rawURL == "" {
		t.Skip("DATABASE_URL unset")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Skip("DATABASE_URL unparsable")
	}
	switch strings.ToLower(u.Hostname()) {
	case "localhost", "127.0.0.1", "db", "::1":
	default:
		t.Skip("refusing non-local DATABASE_URL")
	}
	conn, err := Open(rawURL)
	if err != nil {
		t.Skipf("local postgres unavailable: %v", err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := conn.PingContext(ctx); err != nil {
		t.Skipf("local postgres unavailable: %v", err)
	}
	for _, name := range []string{"094_email_outbox.sql", "095_email_templates.sql"} {
		body, err := migrationFiles.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := conn.ExecContext(ctx, string(body)); err != nil {
			t.Fatalf("%s first apply: %v", name, err)
		}
		if _, err := conn.ExecContext(ctx, string(body)); err != nil {
			t.Fatalf("%s second apply: %v", name, err)
		}
	}
	_, _ = conn.ExecContext(ctx, `DELETE FROM email_outbox WHERE entity_id = 'migration-test'`)
	_, _ = conn.ExecContext(ctx, `DELETE FROM email_template_versions WHERE kind = 'migration_probe'`)
	_, _ = conn.ExecContext(ctx, `DELETE FROM email_templates WHERE kind = 'migration_probe'`)
	var first int64
	err = conn.QueryRowContext(ctx, `
		INSERT INTO email_outbox (kind, entity_id, to_email, provider, status)
		VALUES ('purchase_course', 'migration-test', 'buyer@example.com', 'resend', 'pending')
		ON CONFLICT (kind, entity_id) DO NOTHING
		RETURNING id`).Scan(&first)
	if err != nil || first == 0 {
		t.Fatalf("enqueue: id %d err %v", first, err)
	}
	var second int64
	err = conn.QueryRowContext(ctx, `
		INSERT INTO email_outbox (kind, entity_id, to_email, provider, status)
		VALUES ('purchase_course', 'migration-test', 'buyer@example.com', 'resend', 'pending')
		ON CONFLICT (kind, entity_id) DO NOTHING
		RETURNING id`).Scan(&second)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("second enqueue id %d err %v", second, err)
	}
	const probe = "migration_probe"
	for i := 0; i < 22; i++ {
		if _, err := conn.ExecContext(ctx, `
			INSERT INTO email_template_versions (kind, subject, html, text_body, saved_by)
			SELECT kind, subject, html, text_body, updated_by
			FROM email_templates WHERE kind = $1`, probe); err != nil {
			t.Fatal(err)
		}
		if _, err := conn.ExecContext(ctx, `
			DELETE FROM email_template_versions
			WHERE kind = $1 AND id NOT IN (
				SELECT id FROM email_template_versions
				WHERE kind = $1
				ORDER BY saved_at DESC, id DESC
				LIMIT 20
			)`, probe); err != nil {
			t.Fatal(err)
		}
		if _, err := conn.ExecContext(ctx, `
			INSERT INTO email_templates (kind, subject, html, text_body, updated_by)
			VALUES ($1, 'probe', '<p>migration-test</p>', 'probe', 'test')
			ON CONFLICT (kind) DO UPDATE SET
				subject = EXCLUDED.subject,
				html = EXCLUDED.html,
				text_body = EXCLUDED.text_body,
				updated_by = EXCLUDED.updated_by,
				updated_at = now()`, probe); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	if err := conn.QueryRowContext(ctx, `SELECT count(*) FROM email_template_versions WHERE kind = $1`, probe).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n > 20 {
		t.Fatalf("kept %d versions", n)
	}
	_, _ = conn.ExecContext(ctx, `DELETE FROM email_outbox WHERE entity_id = 'migration-test'`)
	_, _ = conn.ExecContext(ctx, `DELETE FROM email_template_versions WHERE kind = 'migration_probe'`)
	_, _ = conn.ExecContext(ctx, `DELETE FROM email_templates WHERE kind = 'migration_probe'`)
}
