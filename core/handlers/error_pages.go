package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strings"
)

// errorPages turns an unhandled failure into the site's own error page and
// an alert, so a visitor never sees bare text or a dropped connection:
//
//   - a handler that answers 500 with http.Error (what serverError does) gets
//     the styled "Something went wrong" page instead of the plain text;
//   - a panic is recovered, logged, alerted, and answered with the same page.
//
// JSON endpoints and /internal/ routes keep their own error bodies. The page
// is built without any database call, so it still renders when the database
// is the thing that is down.
func (a *App) errorPages(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/internal/") || strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		ew := &errorPageWriter{ResponseWriter: w, app: a, req: r}
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			if rec == http.ErrAbortHandler {
				panic(rec)
			}
			log.Printf("panic %s %s: %v", r.Method, r.URL.Path, rec)
			a.alert(fmt.Sprintf("🚨 Crash (panic) on %s %s\n%s", r.Method, r.URL.Path, clipEmailErr(fmt.Errorf("%v", rec))))
			if !ew.wrote {
				ew.wrote = true
				a.writeErrorPage(w, r)
			}
		}()
		next.ServeHTTP(ew, r)
	})
}

type errorPageWriter struct {
	http.ResponseWriter
	app         *App
	req         *http.Request
	wrote       bool
	intercepted bool
}

func (e *errorPageWriter) Unwrap() http.ResponseWriter { return e.ResponseWriter }

func (e *errorPageWriter) WriteHeader(code int) {
	if e.wrote {
		return
	}
	e.wrote = true
	if code == http.StatusInternalServerError && strings.HasPrefix(e.Header().Get("Content-Type"), "text/plain") {
		e.intercepted = true
		e.app.writeErrorPage(e.ResponseWriter, e.req)
		return
	}
	e.ResponseWriter.WriteHeader(code)
}

func (e *errorPageWriter) Write(b []byte) (int, error) {
	if !e.wrote {
		e.WriteHeader(http.StatusOK)
	}
	if e.intercepted {
		return len(b), nil // the plain-text body is replaced by the page
	}
	return e.ResponseWriter.Write(b)
}

// writeErrorPage renders the 500 page. If rendering itself fails it falls
// back to one line of text, because the error path must never panic.
func (a *App) writeErrorPage(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("error page render: %v", rec)
			http.Error(w, "Something went wrong. Please try again.", http.StatusInternalServerError)
		}
	}()
	w.Header().Set("X-Robots-Tag", "noindex")
	w.Header().Set("Cache-Control", "no-store")
	base := a.baseFrom(r, &basePrefetch{}, "Something went wrong — "+a.Cfg.SiteName, "", "")
	base.NoCanonical = true
	base.RobotsMeta = "noindex, nofollow"
	a.View.Render(w, http.StatusInternalServerError, "server_error", base)
}
