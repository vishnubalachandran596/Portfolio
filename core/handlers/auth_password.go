package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/authutil"
	"github.com/vishnubalachandran596/portfolio/core/repo"
)

// Password login and account settings. Sign-up and password reset are in
// auth_email.go: both go through an emailed one-time link, so an address is
// only ever attached to a password after its owner clicked. Google sign-in
// stays available alongside; an account can have both.

const (
	throttleLogin  = "login"
	throttleReset  = "reset"
	throttleSignup = "signup"
	throttleAdmin  = "admin"

	loginMaxFails  = 5
	resetMaxFails  = 5
	signupMaxPerIP = 10
)

type PasswordAuthData struct {
	BaseData
	Error  string
	Notice string
	Next   string
	Email  string
	Name   string
	// GoogleEnabled lets the page offer Google as the alternative.
	GoogleEnabled bool
	// HasPassword is true when the signed-in account already has one.
	HasPassword bool
	// Heading, Message and Links drive the plain auth_message page.
	Heading string
	Message string
	Links   []AuthLink
	// Token is the one-time link token carried by the reset form.
	Token string
}

// AuthLink is one button on the auth_message page.
type AuthLink struct {
	Label   string
	Href    string
	Primary bool
}

func urlQuery(s string) string { return url.QueryEscape(s) }

// throttled reports whether any of the keys for this kind is locked, and
// when it unlocks. Both the account key and the caller's IP are checked.
func (a *App) throttled(r *http.Request, kind string, keys ...string) (time.Time, bool) {
	var latest time.Time
	for _, k := range keys {
		until, err := a.AuthThrottle.Locked(r.Context(), kind, k)
		if err == nil && until.After(latest) {
			latest = until
		}
	}
	return latest, !latest.IsZero()
}

func lockedMessage(until time.Time) string {
	mins := int(time.Until(until).Minutes()) + 1
	if mins < 1 {
		mins = 1
	}
	return fmt.Sprintf("Too many attempts. Please try again in about %d minute(s).", mins)
}

func (a *App) recordFailure(r *http.Request, kind string, max int, keys ...string) {
	for _, k := range keys {
		_ = a.AuthThrottle.Fail(r.Context(), kind, k, max)
	}
}

func (a *App) clearFailures(r *http.Request, kind string, keys ...string) {
	for _, k := range keys {
		_ = a.AuthThrottle.Clear(r.Context(), kind, k)
	}
}

func cleanEmail(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func validEmail(s string) bool {
	at := strings.LastIndex(s, "@")
	return asciiOnly(s) && at > 0 && at < len(s)-3 && strings.Contains(s[at:], ".") && !strings.ContainsAny(s, " \t\r\n,;<>") && len(s) <= 254
}

func (a *App) authPage(r *http.Request, title string, d PasswordAuthData) PasswordAuthData {
	d.BaseData = a.base(r, title+" — "+a.Cfg.SiteName, "", "")
	d.GoogleEnabled = a.Google.Enabled()
	return d
}

// PasswordLoginSubmit signs in with email + password. The password login
// never grants admin access: the admin console has its own login.
func (a *App) PasswordLoginSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/login?error=Invalid+form+submission", http.StatusSeeOther)
		return
	}
	email := cleanEmail(r.FormValue("email"))
	password := r.FormValue("password")
	next := safeNext(r)
	ek, ik := authutil.HashKey(email), authutil.HashKey(authutil.ClientIP(r))

	back := func(msg string) {
		http.Redirect(w, r, "/login?error="+urlQuery(msg)+"&next="+urlQuery(next), http.StatusSeeOther)
	}
	if until, locked := a.throttled(r, throttleLogin, ek, ik); locked {
		back(lockedMessage(until))
		return
	}

	user, err := a.Users.GetByEmail(r.Context(), email)
	hash := authutil.DummyHash
	if err == nil && user.PasswordUsable {
		hash = user.PasswordHash
	}
	passwordOK := authutil.CheckPassword(hash, password) && err == nil && user.PasswordUsable
	if !passwordOK {
		a.recordFailure(r, throttleLogin, loginMaxFails, ek, ik)
		back("Invalid email or password.")
		return
	}
	if !user.EmailVerified {
		// Right password, but the address was never confirmed (an account
		// from before emailed activation). The reset link confirms it.
		back("Confirm your email first: use Forgot password and we will email you a link.")
		return
	}
	a.clearFailures(r, throttleLogin, ek, ik)
	a.Auth.IssueUserSession(w, user.ID, user.TokenVersion)
	http.Redirect(w, r, next, http.StatusSeeOther)
}

// AccountSecurityPage lets a signed-in visitor set a password (a Google
// account has none) or change it.
func (a *App) AccountSecurityPage(w http.ResponseWriter, r *http.Request) {
	uid, _ := a.Auth.UserID(r)
	user, err := a.Users.GetByID(r.Context(), uid)
	if err != nil {
		a.serverError(w, "load user", err)
		return
	}
	a.View.Render(w, http.StatusOK, "account_security", a.authPage(r, "Password", PasswordAuthData{
		Error: r.URL.Query().Get("error"), Notice: r.URL.Query().Get("notice"),
		Email: user.Email, HasPassword: user.PasswordUsable,
	}))
}

func (a *App) AccountSecuritySubmit(w http.ResponseWriter, r *http.Request) {
	uid, _ := a.Auth.UserID(r)
	_ = r.ParseForm()
	user, err := a.Users.GetByID(r.Context(), uid)
	if err != nil {
		a.serverError(w, "load user", err)
		return
	}
	back := func(kind, msg string) {
		http.Redirect(w, r, "/account/security?"+kind+"="+urlQuery(msg), http.StatusSeeOther)
	}
	ik := authutil.HashKey(authutil.ClientIP(r))
	ek := authutil.HashKey(user.Email)
	if until, locked := a.throttled(r, throttleLogin, ek, ik); locked {
		back("error", lockedMessage(until))
		return
	}
	// A password account must prove the current password; a Google account
	// has none and is already signed in through Google.
	if user.PasswordUsable && !authutil.CheckPassword(user.PasswordHash, r.FormValue("current")) {
		a.recordFailure(r, throttleLogin, loginMaxFails, ek, ik)
		back("error", "Your current password isn't correct.")
		return
	}
	password, confirm := r.FormValue("password"), r.FormValue("confirm")
	switch {
	case password != confirm:
		back("error", "The two passwords don't match.")
		return
	case authutil.ValidatePassword(password, user.Email) != "":
		back("error", authutil.ValidatePassword(password, user.Email))
		return
	}
	pwHash, err := authutil.HashPassword(password)
	if err != nil {
		a.serverError(w, "hash password", err)
		return
	}
	if err := a.Users.SetPassword(r.Context(), uid, pwHash); err != nil {
		a.serverError(w, "save password", err)
		return
	}
	a.clearFailures(r, throttleLogin, ek, ik)
	// SetPassword bumped the token version, so re-issue this session.
	tv, _ := a.Users.TokenVersion(r.Context(), uid)
	a.Auth.IssueUserSession(w, uid, tv)
	back("notice", "Saved. Other devices have been signed out.")
}

// AccountDeletePage shows the delete-my-account confirmation.
func (a *App) AccountDeletePage(w http.ResponseWriter, r *http.Request) {
	uid, _ := a.Auth.UserID(r)
	user, err := a.Users.GetByID(r.Context(), uid)
	if err != nil {
		a.serverError(w, "load user", err)
		return
	}
	a.View.Render(w, http.StatusOK, "account_delete", a.authPage(r, "Delete my account", PasswordAuthData{
		Error: r.URL.Query().Get("error"), Email: user.Email, HasPassword: user.PasswordUsable,
	}))
}

// AccountDeleteSubmit erases the signed-in user's personal data (DPDP
// right to erasure). Password accounts re-enter their password; everyone
// types DELETE. Payment and booking records are anonymised, not kept with
// the person's details (repo.UserRepo.DeleteAccount).
func (a *App) AccountDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	uid, _ := a.Auth.UserID(r)
	_ = r.ParseForm()
	user, err := a.Users.GetByID(r.Context(), uid)
	if err != nil {
		a.serverError(w, "load user", err)
		return
	}
	back := func(msg string) {
		http.Redirect(w, r, "/account/delete?error="+urlQuery(msg), http.StatusSeeOther)
	}
	ik := authutil.HashKey(authutil.ClientIP(r))
	ek := authutil.HashKey(user.Email)
	if until, locked := a.throttled(r, throttleLogin, ek, ik); locked {
		back(lockedMessage(until))
		return
	}
	if strings.TrimSpace(r.FormValue("confirm")) != "DELETE" {
		back("Type DELETE in capitals to confirm.")
		return
	}
	if user.PasswordUsable && !authutil.CheckPassword(user.PasswordHash, r.FormValue("current")) {
		a.recordFailure(r, throttleLogin, loginMaxFails, ek, ik)
		back("Your current password isn't correct.")
		return
	}
	if err := a.Users.DeleteAccount(r.Context(), uid); err != nil {
		if errors.Is(err, repo.ErrDeleteBlocked) {
			back("This account can't be deleted online right now (it may have an upcoming booking, or it is an administrator account). Email the privacy contact and we will handle it.")
			return
		}
		a.serverError(w, "delete account", err)
		return
	}
	a.Auth.ClearUserSession(w)
	a.Auth.ClearSession(w)
	http.Redirect(w, r, "/login?notice="+urlQuery("Your account and personal details have been deleted."), http.StatusSeeOther)
}
