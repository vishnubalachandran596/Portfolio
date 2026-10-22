package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/authutil"
	"github.com/vishnubalachandran596/portfolio/core/repo"
)

// Password sign-up, login and security-question recovery. The site sends no
// email, so nothing here depends on one: the email address is only a login
// name, never treated as verified, and recovery is the question the visitor
// chose at sign-up. Google sign-in stays available alongside.

// securityQuestions is the fixed list offered at sign-up. A fixed list (not
// free text) lets an unknown email be given a believable question too, so
// the reset page cannot be used to find out who has an account.
var securityQuestions = []string{
	"What was the name of your first school?",
	"What was the name of your childhood best friend?",
	"In which town or city was your father born?",
	"In which town or city was your mother born?",
	"What was the name of your first pet?",
	"What was the make and model of your first vehicle?",
	"What was the name of your favourite teacher?",
	"What was the name of the first company you worked for?",
}

const (
	throttleLogin  = "login"
	throttleReset  = "reset"
	throttleSignup = "signup"
	throttleAdmin  = "admin"

	loginMaxFails  = 5
	resetMaxFails  = 5
	signupMaxPerIP = 10
)

func validQuestion(q string) bool {
	for _, s := range securityQuestions {
		if s == q {
			return true
		}
	}
	return false
}

type PasswordAuthData struct {
	BaseData
	Error     string
	Notice    string
	Next      string
	Email     string
	Name      string
	Question  string
	Questions []string
	Selected  string
	// GoogleEnabled lets the page offer Google as the alternative.
	GoogleEnabled bool
	// HasPassword is true when the signed-in account already has one.
	HasPassword bool
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
	return at > 0 && at < len(s)-3 && strings.Contains(s[at:], ".") && !strings.ContainsAny(s, " \t\r\n,;<>") && len(s) <= 254
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
	a.clearFailures(r, throttleLogin, ek, ik)
	a.Auth.IssueUserSession(w, user.ID, user.TokenVersion)
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (a *App) SignupPage(w http.ResponseWriter, r *http.Request) {
	next := safeNext(r)
	if a.Auth.IsUserAuthenticated(r) {
		http.Redirect(w, r, next, http.StatusSeeOther)
		return
	}
	a.View.Render(w, http.StatusOK, "signup", a.authPage(r, "Create account", PasswordAuthData{
		Next: next, Questions: securityQuestions, Error: r.URL.Query().Get("error"),
	}))
}

func (a *App) SignupSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form submission", http.StatusBadRequest)
		return
	}
	d := PasswordAuthData{
		Next: safeNext(r), Questions: securityQuestions,
		Email: cleanEmail(r.FormValue("email")), Name: strings.TrimSpace(r.FormValue("name")),
		Selected: r.FormValue("question"),
	}
	fail := func(msg string) {
		d.Error = msg
		a.View.Render(w, http.StatusOK, "signup", a.authPage(r, "Create account", d))
	}
	// Honeypot: real visitors never fill this hidden field.
	if r.FormValue("website") != "" {
		http.Redirect(w, r, d.Next, http.StatusSeeOther)
		return
	}
	ik := authutil.HashKey(authutil.ClientIP(r))
	if until, locked := a.throttled(r, throttleSignup, ik); locked {
		fail(lockedMessage(until))
		return
	}
	password, confirm := r.FormValue("password"), r.FormValue("confirm")
	answer := authutil.NormalizeAnswer(r.FormValue("answer"))
	switch {
	case !validEmail(d.Email):
		fail("Enter a valid email address.")
		return
	case len(d.Name) > 100:
		fail("Keep your name under 100 characters.")
		return
	case password != confirm:
		fail("The two passwords don't match.")
		return
	case authutil.ValidatePassword(password, d.Email) != "":
		fail(authutil.ValidatePassword(password, d.Email))
		return
	case !validQuestion(d.Selected):
		fail("Choose one of the security questions.")
		return
	case len(answer) < 3 || len(answer) > 100:
		fail("Your security answer must be between 3 and 100 characters.")
		return
	}

	pwHash, err := authutil.HashPassword(password)
	if err != nil {
		a.serverError(w, "hash password", err)
		return
	}
	ansHash, err := authutil.HashPassword(answer)
	if err != nil {
		a.serverError(w, "hash answer", err)
		return
	}
	a.recordFailure(r, throttleSignup, signupMaxPerIP, ik) // counts every sign-up, success or not
	ctx := r.Context()
	uid, err := a.Users.CreateWithPassword(ctx, d.Email, pwHash, d.Name, d.Selected, ansHash)
	if err == repo.ErrEmailTaken {
		existing, gerr := a.Users.GetByEmail(ctx, d.Email)
		claimed := false
		if gerr == nil && existing.IsGuest {
			claimed, _ = a.Users.ClaimGuest(ctx, existing.ID, pwHash, d.Name, d.Selected, ansHash)
			uid = existing.ID
		}
		if !claimed {
			fail("An account with this email already exists. Log in instead, or use Google if that's how you signed up.")
			return
		}
	} else if err != nil {
		a.serverError(w, "create user", err)
		return
	}
	tv, _ := a.Users.TokenVersion(ctx, uid)
	a.Auth.IssueUserSession(w, uid, tv)
	http.Redirect(w, r, d.Next, http.StatusSeeOther)
}

// resetQuestion returns the question to show for an email: the real one for
// a resettable account, otherwise a stable fake chosen from the same list.
func (a *App) resetQuestion(r *http.Request, email string) (question string, real bool) {
	if user, err := a.Users.GetByEmail(r.Context(), email); err == nil &&
		user.PasswordUsable && user.SecurityAnswerHash != "" && validQuestion(user.SecurityQuestion) {
		if _, aerr := a.Admins.GetByEmail(r.Context(), email); aerr != nil {
			return user.SecurityQuestion, true
		}
	}
	mac := hmac.New(sha256.New, a.Cfg.JWTSecret)
	mac.Write([]byte("reset-question|" + email))
	return securityQuestions[int(mac.Sum(nil)[0])%len(securityQuestions)], false
}

func (a *App) ForgotPage(w http.ResponseWriter, r *http.Request) {
	a.View.Render(w, http.StatusOK, "forgot", a.authPage(r, "Reset password", PasswordAuthData{
		Error: r.URL.Query().Get("error"),
	}))
}

// ForgotQuestion is step one: the email goes in, the security question
// comes back (a believable one even when the account can't be reset).
func (a *App) ForgotQuestion(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	email := cleanEmail(r.FormValue("email"))
	if !validEmail(email) {
		http.Redirect(w, r, "/forgot?error=Enter+a+valid+email+address", http.StatusSeeOther)
		return
	}
	q, _ := a.resetQuestion(r, email)
	a.View.Render(w, http.StatusOK, "forgot", a.authPage(r, "Reset password", PasswordAuthData{
		Email: email, Question: q,
	}))
}

// ForgotReset is step two: the answer and a new password. Every failure
// returns the same message and the same question, and counts against both
// the email and the caller's IP.
func (a *App) ForgotReset(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	email := cleanEmail(r.FormValue("email"))
	answer := authutil.NormalizeAnswer(r.FormValue("answer"))
	password, confirm := r.FormValue("password"), r.FormValue("confirm")
	q, real := a.resetQuestion(r, email)
	d := PasswordAuthData{Email: email, Question: q}
	show := func(msg string) {
		d.Error = msg
		a.View.Render(w, http.StatusOK, "forgot", a.authPage(r, "Reset password", d))
	}
	ek, ik := authutil.HashKey(email), authutil.HashKey(authutil.ClientIP(r))
	if until, locked := a.throttled(r, throttleReset, ek, ik); locked {
		show(lockedMessage(until))
		return
	}

	hash := authutil.DummyHash
	var user = struct{ ID int64 }{}
	if real {
		if u, err := a.Users.GetByEmail(r.Context(), email); err == nil {
			hash, user.ID = u.SecurityAnswerHash, u.ID
		}
	}
	answerOK := authutil.CheckPassword(hash, answer) && real && answer != ""
	if !answerOK {
		a.recordFailure(r, throttleReset, resetMaxFails, ek, ik)
		show("That answer isn't correct, or this account can't be reset here.")
		return
	}
	switch {
	case password != confirm:
		show("The two passwords don't match.")
		return
	case authutil.ValidatePassword(password, email) != "":
		show(authutil.ValidatePassword(password, email))
		return
	}
	pwHash, err := authutil.HashPassword(password)
	if err != nil {
		a.serverError(w, "hash password", err)
		return
	}
	if err := a.Users.SetPassword(r.Context(), user.ID, pwHash); err != nil {
		a.serverError(w, "reset password", err)
		return
	}
	a.clearFailures(r, throttleReset, ek, ik)
	http.Redirect(w, r, "/login?notice=Password+updated.+Log+in+with+your+new+password.", http.StatusSeeOther)
}

// AccountSecurityPage lets a signed-in visitor set a password and security
// question (Google accounts have neither) or change them.
func (a *App) AccountSecurityPage(w http.ResponseWriter, r *http.Request) {
	uid, _ := a.Auth.UserID(r)
	user, err := a.Users.GetByID(r.Context(), uid)
	if err != nil {
		a.serverError(w, "load user", err)
		return
	}
	a.View.Render(w, http.StatusOK, "account_security", a.authPage(r, "Password and security", PasswordAuthData{
		Questions: securityQuestions, Selected: user.SecurityQuestion,
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
	question := r.FormValue("question")
	answer := authutil.NormalizeAnswer(r.FormValue("answer"))
	switch {
	case password != confirm:
		back("error", "The two passwords don't match.")
		return
	case authutil.ValidatePassword(password, user.Email) != "":
		back("error", authutil.ValidatePassword(password, user.Email))
		return
	case !validQuestion(question):
		back("error", "Choose one of the security questions.")
		return
	case len(answer) < 3 || len(answer) > 100:
		back("error", "Your security answer must be between 3 and 100 characters.")
		return
	}
	pwHash, err1 := authutil.HashPassword(password)
	ansHash, err2 := authutil.HashPassword(answer)
	if err1 != nil || err2 != nil {
		a.serverError(w, "hash credentials", fmt.Errorf("%v %v", err1, err2))
		return
	}
	if err := a.Users.SetSecurity(r.Context(), uid, question, ansHash); err != nil {
		a.serverError(w, "save security", err)
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
