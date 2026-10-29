package handlers

import (
	"log"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/vishnubalachandran596/portfolio/core/authutil"
	"github.com/vishnubalachandran596/portfolio/core/repo"
)

// Visitor auth is Google-only: one "Sign in with Google" button serves
// both a regular visitor (booking consultancy, buying a course) and the
// site owner — a Google account whose email matches the admins table
// also opens the admin dashboard, alongside a normal visitor session so
// the admin can browse the rest of the site exactly like any other
// logged-in user. There's no separate password to set up or lose.

type LoginData struct {
	BaseData
	Error         string
	Notice        string
	Next          string
	GoogleEnabled bool
}

func safeNextValue(next string) string {
	// Only a plain same-site path is allowed. Browsers read "\" as "/" and
	// ignore tabs and line breaks inside a URL, so "/\\evil.com" and "/\t/evil.com"
	// would both leave the site; reject those along with absolute URLs.
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") ||
		strings.ContainsAny(next, "\\\t\r\n") || strings.ContainsFunc(next, unicode.IsControl) {
		return "/"
	}
	return next
}

// safeNext reads the redirect target from wherever the current request
// carries it: the query string on a page load, or the "next" form field
// this package's own POST endpoints pass along.
func safeNext(r *http.Request) string {
	if next := r.FormValue("next"); next != "" {
		return safeNextValue(next)
	}
	return safeNextValue(r.URL.Query().Get("next"))
}

const oauthStateCookieName = "portfolio_oauth_state"

func (a *App) LoginPage(w http.ResponseWriter, r *http.Request) {
	next := safeNext(r)
	if a.Auth.IsUserAuthenticated(r) || a.Auth.IsAuthenticated(r) {
		http.Redirect(w, r, next, http.StatusSeeOther)
		return
	}
	a.View.Render(w, http.StatusOK, "login", LoginData{
		BaseData:      a.base(r, "Log In — "+a.Cfg.SiteName, "Log in to access your purchased courses and account.", ""),
		Error:         r.URL.Query().Get("error"),
		Notice:        r.URL.Query().Get("notice"),
		Next:          next,
		GoogleEnabled: a.Google.Enabled(),
	})
}

// GoogleLoginStart sends the visitor to Google's consent screen, after
// stashing a random CSRF state value (and the post-login redirect
// target) in a short-lived cookie only this request/callback pair reads.
func (a *App) GoogleLoginStart(w http.ResponseWriter, r *http.Request) {
	if !a.Google.Enabled() {
		http.Redirect(w, r, "/login?error=Google+sign-in+isn%27t+configured+yet", http.StatusSeeOther)
		return
	}
	state, err := authutil.RandomHex(24)
	if err != nil {
		a.serverError(w, "generate oauth state", err)
		return
	}
	next := safeNext(r)

	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookieName,
		Value:    state + "|" + next,
		Path:     "/",
		HttpOnly: true,
		Secure:   a.Cfg.IsProduction(),
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(10 * time.Minute),
	})
	http.Redirect(w, r, a.Google.AuthURL(state), http.StatusSeeOther)
}

// GoogleLoginCallback completes the flow: verify the state matches what
// we stashed (CSRF protection), exchange the code, fetch the verified
// email, then find-or-create the visitor account and sign them in.
func (a *App) GoogleLoginCallback(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(oauthStateCookieName)
	if err != nil {
		http.Redirect(w, r, "/login?error=Sign-in+session+expired%2C+please+try+again", http.StatusSeeOther)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: oauthStateCookieName, Value: "", Path: "/", MaxAge: -1, Expires: time.Unix(0, 0),
	})
	wantState, next, ok := strings.Cut(cookie.Value, "|")
	if !ok {
		next = "/"
	}
	if r.URL.Query().Get("error") != "" || r.URL.Query().Get("state") != wantState {
		http.Redirect(w, r, "/login?error=Sign-in+was+cancelled+or+could+not+be+verified", http.StatusSeeOther)
		return
	}

	code := r.URL.Query().Get("code")
	accessToken, err := a.Google.Exchange(code)
	if err != nil {
		a.serverError(w, "google oauth exchange", err)
		return
	}
	info, err := a.Google.FetchUserInfo(accessToken)
	if err != nil {
		a.serverError(w, "google oauth userinfo", err)
		return
	}
	if !info.EmailVerified {
		http.Redirect(w, r, "/login?error=Your+Google+email+isn%27t+verified", http.StatusSeeOther)
		return
	}
	email := strings.ToLower(strings.TrimSpace(info.Email))
	ctx := r.Context()

	user, err := a.Users.GetByEmail(ctx, email)
	if err == repo.ErrNotFound {
		// A Google sign-in never sets a usable password — the column is
		// NOT NULL, so store a random value nobody could ever type in.
		randomPassword, rerr := authutil.RandomHex(32)
		if rerr != nil {
			a.serverError(w, "provision google user", rerr)
			return
		}
		hash, herr := authutil.HashPassword(randomPassword)
		if herr != nil {
			a.serverError(w, "provision google user", herr)
			return
		}
		uid, cerr := a.Users.Create(ctx, email, hash, info.Name)
		if cerr != nil {
			a.serverError(w, "provision google user", cerr)
			return
		}
		user.ID = uid
	} else if err != nil {
		a.serverError(w, "load user", err)
		return
	} else if user.IsGuest {
		// This email was previously captured via the anonymous article
		// thumbs-up form (never actually signed in) — now that they're
		// signing in for real, the account (and its vote history) stays
		// the same row, it's just no longer "not signed up" in the admin
		// view. Best-effort: never block a real login over this.
		if merr := a.Users.MarkSignedUp(ctx, user.ID); merr != nil {
			log.Printf("mark guest user signed up: %v", merr)
		}
	}
	// Google has now proven control of this email. If it was an unverified
	// password account, its password and security answer are cleared.
	if _, verr := a.Users.MarkGoogleVerified(ctx, user.ID); verr != nil {
		a.serverError(w, "mark google verified", verr)
		return
	}
	// Only now - Google has just proven this email - are guest purchases
	// made with it attached to the account. A typed or password-signup email
	// is never trusted for this. Best-effort: never block a login over it.
	if _, aerr := a.Claims.AttachByEmail(ctx, user.ID, email); aerr != nil {
		log.Printf("attach guest purchases by verified email: %v", aerr)
	}
	tv, _ := a.Users.TokenVersion(ctx, user.ID)
	a.Auth.IssueUserSession(w, user.ID, tv)

	// Same email as the configured admin account also opens the admin
	// dashboard, alongside the visitor session just issued.
	if admin, aerr := a.Admins.GetByEmail(ctx, email); aerr == nil {
		a.Auth.IssueSession(w, admin.ID)
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (a *App) UserLogout(w http.ResponseWriter, r *http.Request) {
	a.Auth.ClearUserSession(w)
	a.Auth.ClearSession(w)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
