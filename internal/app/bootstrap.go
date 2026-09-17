// Package app wires configuration, the database, and the view renderer
// into a ready-to-serve handlers.App. Both the local server (cmd/server)
// and the Vercel serverless entrypoint (api/index.go) call Bootstrap so
// there is exactly one place that assembles the application.
package app

import (
	"context"
	"database/sql"
	"log"
	"time"

	"github.com/vishnubalachandran596/portfolio/internal/authutil"
	"github.com/vishnubalachandran596/portfolio/internal/config"
	"github.com/vishnubalachandran596/portfolio/internal/db"
	"github.com/vishnubalachandran596/portfolio/internal/handlers"
	"github.com/vishnubalachandran596/portfolio/internal/view"
)

// Bootstrap loads config, connects to Postgres, applies migrations,
// optionally seeds the bootstrap admin account, and returns a fully wired
// App. The returned cleanup func closes the database pool and should be
// deferred by the caller.
func Bootstrap() (*handlers.App, func(), error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, err
	}

	conn, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { _ = conn.Close() }

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.Migrate(ctx, conn); err != nil {
		cleanup()
		return nil, nil, err
	}

	renderer, err := view.New()
	if err != nil {
		cleanup()
		return nil, nil, err
	}

	a := handlers.New(cfg, conn, renderer)

	if err := bootstrapAdmin(ctx, conn, cfg.AdminEmail, cfg.AdminPassword); err != nil {
		log.Printf("admin bootstrap: %v", err)
	}

	return a, cleanup, nil
}

// bootstrapAdmin creates the first admin account from
// ADMIN_BOOTSTRAP_EMAIL/ADMIN_BOOTSTRAP_PASSWORD when no admin exists yet.
// It is a no-op once at least one admin has been created, so it's safe to
// leave the env vars set permanently.
func bootstrapAdmin(ctx context.Context, conn *sql.DB, email, password string) error {
	if email == "" || password == "" {
		return nil
	}
	var count int
	if err := conn.QueryRowContext(ctx, `SELECT count(*) FROM admins`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	hash, err := authutil.HashPassword(password)
	if err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, `INSERT INTO admins (email, password_hash, name) VALUES ($1, $2, '') ON CONFLICT (email) DO NOTHING`, email, hash)
	if err == nil {
		log.Printf("bootstrapped admin account for %s", email)
	}
	return err
}
