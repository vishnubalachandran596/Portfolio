// Command server runs the portfolio site as a standalone HTTP server, for
// local development or any host that runs a long-lived Go process
// (Fly.io, Render, a VM, etc.). Vercel deployments instead use
// api/index.go, which wraps the same handlers.App as a single serverless
// function.
package main

import (
	"log"
	"net/http"
	"time"

	"github.com/vishnubalachandran596/portfolio/internal/app"
)

func main() {
	a, cleanup, err := app.Bootstrap()
	if err != nil {
		log.Fatalf("bootstrap: %v", err)
	}
	defer cleanup()

	srv := &http.Server{
		Addr:              ":" + a.Cfg.Port,
		Handler:           a.Routes(),
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("portfolio server listening on :%s (env=%s)", a.Cfg.Port, a.Cfg.Environment)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}
