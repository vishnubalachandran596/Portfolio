// Package app wires configuration, the database, and the view renderer
// into a ready-to-serve handlers.App. Both the local server (cmd/server)
// and the Vercel serverless entrypoint (api/index.go) call Bootstrap so
// there is exactly one place that assembles the application.
package app

import (
	"context"
	"database/sql"
	"log"
	"strings"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/config"
	"github.com/vishnubalachandran596/portfolio/core/db"
	"github.com/vishnubalachandran596/portfolio/core/handlers"
	"github.com/vishnubalachandran596/portfolio/core/view"
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

	if err := bootstrapAdmin(ctx, conn, cfg.AdminEmail); err != nil {
		log.Printf("admin bootstrap: %v", err)
	}

	return a, cleanup, nil
}

// bootstrapAdmin makes sure ADMIN_BOOTSTRAP_EMAIL is on the admin list on
// every start, so the owner is always an admin and always has every course.
// No password is stored: admin sign-in compares the typed password with
// ADMIN_BOOTSTRAP_PASSWORD (a Vercel secret) at login, so changing the
// variable and redeploying changes the password.
func bootstrapAdmin(ctx context.Context, conn *sql.DB, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil
	}
	res, err := conn.ExecContext(ctx, `INSERT INTO admins (email, name) VALUES ($1, '') ON CONFLICT (email) DO NOTHING`, email)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		log.Printf("added %s to the admin list", email)
	}
	return nil
}
