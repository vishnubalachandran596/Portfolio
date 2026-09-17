package handlers

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/vishnubalachandran596/portfolio/core/authutil"
	"github.com/vishnubalachandran596/portfolio/core/repo"
)

// Visitor auth. One login form serves both a regular visitor (booking
// consultancy, buying a course) and the site owner: logging in with the
// email/password that matches the admins table also opens the admin
// dashboard, alongside a normal visitor session so the admin can browse
// the rest of the site exactly like any other logged-in user.

type LoginData struct {
	BaseData
	Error string
	Next  string
	Mode  string // "login" or "signup", which tab to show first
}

func safeNextValue(next string) string {
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		return "/"
	}
	return next
}

// safeNext reads the redirect target from wherever the current request
// carries it: the query string on a page load (GET /login?next=...), or
// the submitted form field on a POST (the login/signup form's hidden
// "next" input, which isn't part of the URL's own query string).
func safeNext(r *http.Request) string {
	if next := r.FormValue("next"); next != "" {
		return safeNextValue(next)
	}
	return safeNextValue(r.URL.Query().Get("next"))
}

func (a *App) LoginPage(w http.ResponseWriter, r *http.Request) {
	next := safeNext(r)
	if a.Auth.IsUserAuthenticated(r) || a.Auth.IsAuthenticated(r) {
		http.Redirect(w, r, next, http.StatusSeeOther)
		return
	}
	mode := r.URL.Query().Get("mode")
	if mode != "signup" {
		mode = "login"
	}
	a.View.Render(w, http.StatusOK, "login", LoginData{
		BaseData: a.base(r, "Log In — "+a.Cfg.SiteName, "Log in to book a consultancy session or access your purchased courses.", ""),
		Error:    r.URL.Query().Get("error"),
		Next:     next,
		Mode:     mode,
	})
}

func (a *App) LoginSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/login?error=Invalid+form+submission", http.StatusSeeOther)
		return
	}
	email := strings.TrimSpace(strings.ToLower(r.FormValue("email")))
	password := r.FormValue("password")
	next := safeNext(r)
	ctx := r.Context()

	// An email/password that matches an admin account opens both the
	// admin dashboard and a matching visitor session — auto-provisioning
	// the visitor account on first login so there's nothing separate to
	// set up.
	if admin, err := a.Admins.GetByEmail(ctx, email); err == nil && authutil.CheckPassword(admin.PasswordHash, password) {
		a.Auth.IssueSession(w, admin.ID)
		user, err := a.Users.GetByEmail(ctx, email)
		if err == repo.ErrNotFound {
			uid, cerr := a.Users.Create(ctx, email, admin.PasswordHash, admin.Name)
			if cerr != nil {
				a.serverError(w, "provision admin user account", cerr)
				return
			}
			a.Auth.IssueUserSession(w, uid)
		} else if err == nil {
			a.Auth.IssueUserSession(w, user.ID)
		}
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}

	user, err := a.Users.GetByEmail(ctx, email)
	if err != nil || !authutil.CheckPassword(user.PasswordHash, password) {
		http.Redirect(w, r, "/login?error=Invalid+email+or+password&next="+url.QueryEscape(next), http.StatusSeeOther)
		return
	}
	a.Auth.IssueUserSession(w, user.ID)
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (a *App) SignupSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/login?mode=signup&error=Invalid+form+submission", http.StatusSeeOther)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	email := strings.TrimSpace(strings.ToLower(r.FormValue("email")))
	password := r.FormValue("password")
	next := safeNext(r)

	if name == "" || !strings.Contains(email, "@") || len(password) < 8 {
		http.Redirect(w, r, "/login?mode=signup&error=Please+enter+your+name%2C+a+valid+email%2C+and+a+password+of+at+least+8+characters&next="+url.QueryEscape(next), http.StatusSeeOther)
		return
	}

	hash, err := authutil.HashPassword(password)
	if err != nil {
		a.serverError(w, "hash password", err)
		return
	}
	uid, err := a.Users.Create(r.Context(), email, hash, name)
	if err != nil {
		if err == repo.ErrEmailTaken {
			http.Redirect(w, r, "/login?mode=login&error=That+email+is+already+registered+%E2%80%94+log+in+instead&next="+url.QueryEscape(next), http.StatusSeeOther)
			return
		}
		a.serverError(w, "create user", err)
		return
	}
	a.Auth.IssueUserSession(w, uid)
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (a *App) UserLogout(w http.ResponseWriter, r *http.Request) {
	a.Auth.ClearUserSession(w)
	a.Auth.ClearSession(w)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
