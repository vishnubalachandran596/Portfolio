// Package db manages the Postgres connection pool and schema migrations.
package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Open connects to Postgres (including Supabase's pooled/direct connection
// strings) and configures sane pool limits for a serverless environment.
func Open(databaseURL string) (*sql.DB, error) {
	cfg, err := pgx.ParseConfig(stripUnrecognizedParams(databaseURL))
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	// Supabase's pooled connection string (Supavisor/PgBouncer in
	// transaction-pooling mode, port 6543 — what this app is deployed
	// against on Vercel) hands out a different backend connection per
	// transaction, so server-side prepared statements don't survive
	// between queries. pgx defaults to using them; force the simple
	// query protocol instead so every query is self-contained. This is a
	// no-op on a direct (non-pooled) connection.
	cfg.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol

	conn := stdlib.OpenDB(*cfg)
	conn.SetMaxOpenConns(10)
	conn.SetMaxIdleConns(5)
	conn.SetConnMaxLifetime(30 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := conn.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return conn, nil
}

// stripUnrecognizedParams removes query parameters that are meaningful to
// other ecosystems' pooled-connection conventions (e.g. Prisma's
// "pgbouncer=true" hint, which Supabase's dashboard includes in its
// generated connection strings) but aren't real Postgres or pgx settings.
// Left in place, pgx would forward them to the server as unknown runtime
// parameters and the connection would fail outright.
func stripUnrecognizedParams(databaseURL string) string {
	u, err := url.Parse(databaseURL)
	if err != nil {
		return databaseURL
	}
	q := u.Query()
	q.Del("pgbouncer")
	u.RawQuery = q.Encode()
	return u.String()
}

// Migrate applies every *.sql file under core/db/migrations that hasn't
// already been recorded in schema_migrations, in filename order.
func Migrate(ctx context.Context, conn *sql.DB) error {
	if _, err := conn.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		var applied int
		if err := conn.QueryRowContext(ctx,
			`SELECT count(*) FROM schema_migrations WHERE version = $1`, name,
		).Scan(&applied); err != nil {
			return fmt.Errorf("check migration %s: %w", name, err)
		}
		if applied > 0 {
			continue
		}

		contents, err := migrationFiles.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin tx for %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, string(contents)); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations (version) VALUES ($1)`, name,
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", name, err)
		}
	}
	return nil
}
