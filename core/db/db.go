// Package db manages the Postgres connection pool and schema migrations.
package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log"
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

	// A Vercel Go instance serves one request at a time, so the pool only
	// has to cover the widest concurrent fan-out inside a single request —
	// not the request rate. That widest point is the About page: seven
	// page lookups in its errgroup plus the two the shared chrome runs for
	// a signed-in visitor, nine in one wave. The pool must not be smaller
	// than that: the surplus goroutines would block waiting for a
	// connection and one concurrent wave would quietly become two, which
	// is a latency bug that looks like nothing in the code. If the fan-out
	// grows again, this has to grow with it.
	const widestRequestFanOut = 9
	conn.SetMaxOpenConns(widestRequestFanOut + 1)

	// Idle connections are the difference between a fast warm invocation
	// and a slow one, and they matter far more here than the ceiling does.
	// Postgres is in ap-south-1 while the function runs in iad1, so
	// establishing a connection costs a TCP handshake, a TLS handshake and
	// the startup/auth exchange across ~200 ms of round-trip latency —
	// several hundred milliseconds before the first query can even be
	// sent. Keeping fewer idle connections than the widest fan-out means
	// every single render of the widest page re-establishes the shortfall
	// from scratch, which is exactly why /about never got faster when
	// warm. Matching the ceiling means a warm instance re-uses what it
	// already has.
	conn.SetMaxIdleConns(widestRequestFanOut + 1)

	// Neither of these is a latency lever; they bound how long a mostly
	// idle instance keeps holding a client slot on the pooler.
	conn.SetConnMaxIdleTime(5 * time.Minute)
	conn.SetConnMaxLifetime(30 * time.Minute)

	warnIfNotPooled(cfg.Host)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := conn.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return conn, nil
}

// warnIfNotPooled logs a warning when DATABASE_URL looks like Supabase's
// direct connection rather than the transaction pooler. Nothing verified
// this before, even though the whole app is built on the assumption (see
// the QueryExecModeSimpleProtocol comment above): a fleet of serverless
// instances against the direct connection exhausts Postgres' connection
// limit, and the failure arrives as 500s on pages the database is
// perfectly healthy to serve. A warning rather than a hard failure, so a
// plain local Postgres or a non-Supabase host still boots.
func warnIfNotPooled(host string) {
	if !strings.HasSuffix(host, ".supabase.co") && !strings.HasSuffix(host, ".supabase.com") {
		return
	}
	if strings.Contains(host, "pooler") {
		return
	}
	log.Printf("warning: DATABASE_URL host %q is not Supabase's transaction pooler; "+
		"a serverless fleet on the direct connection will exhaust Postgres' connection limit", host)
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

	// One read of the whole table instead of one round trip per file. This
	// runs on every cold start, and the per-file version of it grew by a
	// round trip with every migration added — at ~200 ms to ap-south-1 and
	// 48 files it was most of a second of cold-start latency spent asking
	// whether each file had already run.
	applied, err := appliedMigrations(ctx, conn)
	if err != nil {
		return err
	}

	for _, name := range names {
		if applied[name] {
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
		applied[name] = true
	}
	return nil
}

func appliedMigrations(ctx context.Context, conn *sql.DB) (map[string]bool, error) {
	rows, err := conn.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("read schema_migrations: %w", err)
	}
	defer rows.Close()
	applied := make(map[string]bool)
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("scan schema_migrations: %w", err)
		}
		applied[version] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read schema_migrations: %w", err)
	}
	return applied, nil
}
