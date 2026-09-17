// Package handler is the Vercel Go serverless entrypoint. Vercel's Go
// runtime treats every file under /api as its own function; this single
// catch-all function (routed here by vercel.json's rewrite rule) serves
// the entire app, reusing one Bootstrap()'d handlers.App across warm
// invocations of the same instance.
package handler

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/vishnubalachandran596/portfolio/internal/app"
)

var (
	once    sync.Once
	routes  http.Handler
	initErr error
)

// Handler is Vercel's entrypoint signature for a Go function.
func Handler(w http.ResponseWriter, r *http.Request) {
	once.Do(func() {
		a, _, err := app.Bootstrap()
		if err != nil {
			initErr = err
			return
		}
		// Deliberately not deferring cleanup: the DB pool is reused across
		// warm invocations of this same serverless instance.
		routes = a.Routes()
	})
	if initErr != nil {
		http.Error(w, fmt.Sprintf("service unavailable: %v", initErr), http.StatusInternalServerError)
		return
	}
	routes.ServeHTTP(w, r)
}
