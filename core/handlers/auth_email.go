package handlers

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/authutil"
	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
)

// Email-verified sign-up and email-link password reset.
//
// Sign-up never creates an account. It stores the name and the password
// hash with a one-time token and emails a link; the account is created (or,
// for an address that already signs in with Google, given a password too)
// only when the link is opened. Reset works the same way: the emailed link
// opens a form, and submitting it sets the new password. A token is 256
// random bits, stored only as a SHA-256 hash, valid for one hour, used once.
//
// The account emails are not rebuildable (the raw token is not kept), so the
// daily retry cron disables them rather than resending; the visitor asks
// again instead.

const (
	authLinkTTL      = time.Hour
	authLinkTTLLabel = "1 hour"

	throttleVerify = "verify" // sign-up emails sent to one address
	throttleForgot = "forgot" // reset requests

	verifyMaxPerEmail = 3
	forgotMaxPerIP    = 8
	forgotMaxPerEmail = 3
)

// errEmailNotRebuildable marks account emails: the retry cron disables them.
var errEmailNotRebuildable = errors.New("account email cannot be rebuilt")

// newAuthToken returns a fresh random token and the hash that is stored.
func newAuthToken() (raw, hash string, err error) {
	raw, err = authutil.RandomHex(32)
	if err != nil {
		return "", "", err
	}
	return raw, authutil.HashKey(raw), nil
}

// sendAuthLink emails one account link and reports whether it went out.
func (a *App) sendAuthLink(ctx context.Context, kind string, tokenID int64, to, name, path, raw string) bool {
	d := a.baseEmailData()
	if strings.TrimSpace(name) != "" {
		d.BuyerName = name
	}
	d.ActionURL = a.absURL(path + "?token=" + raw)
	d.ExpiresIn = authLinkTTLLabel
	return a.dispatchEmail(ctx, emailJob{
		Kind:     kind,
		EntityID: strconv.FormatInt(tokenID, 10),
		To:       to,
		Data:     d,
	}) == "sent"
}

func (a *App) authMessage(w http.ResponseWriter, r *http.Request, status int, title, heading, message string, links ...AuthLink) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	a.View.Render(w, status, "auth_message", a.authPage(r, title, PasswordAuthData{
		Heading: heading, Message: message, Links: links,
	}))
}

func (a *App) SignupPage(w http.ResponseWriter, r *http.Request) {
	next := safeNext(r)
	if a.Auth.IsUserAuthenticated(r) {
		http.Redirect(w, r, next, http.StatusSeeOther)
		return
	}
	a.View.Render(w, http.StatusOK, "signup", a.authPage(r, "Create account", PasswordAuthData{
		Next: next, Error: r.URL.Query().Get("error"),
	}))
}

// SignupSubmit validates the form and emails an activation link. Nothing is
// created yet. An address that already has a password account gets the same
// page and no email, so the form does not reveal who is registered.
func (a *App) SignupSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form submission", http.StatusBadRequest)
		return
	}
	d := PasswordAuthData{
		Next:  safeNext(r),
		Email: cleanEmail(r.FormValue("email")), Name: strings.TrimSpace(r.FormValue("name")),
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
	switch {
	case !validEmail(d.Email):
		fail("Enter a valid email address.")
		return
	case firstFieldError(fieldSpec{"Your name", fieldName, 0, true}, d.Name) != "":
		fail(firstFieldError(fieldSpec{"Your name", fieldName, 0, true}, d.Name))
		return
	case password != confirm:
		fail("The two passwords don't match.")
		return
	case authutil.ValidatePassword(password, d.Email) != "":
		fail(authutil.ValidatePassword(password, d.Email))
		return
	}
	if !a.humanVerified(r) {
		fail("We could not confirm you are human. Please complete the check and try again.")
		return
	}
	ek := authutil.HashKey(d.Email)
	if until, locked := a.throttled(r, throttleVerify, ek); locked {
		fail(lockedMessage(until))
		return
	}
	a.recordFailure(r, throttleSignup, signupMaxPerIP, ik) // counts every sign-up, success or not
	ctx := r.Context()

	sent := a.signupSentPage
	if existing, err := a.Users.GetByEmail(ctx, d.Email); err == nil && existing.PasswordUsable && existing.EmailVerified {
		sent(w, r, d.Email) // already registered: say nothing different, send nothing
		return
	} else if err != nil && !errors.Is(err, repo.ErrNotFound) {
		a.serverError(w, "load user", err)
		return
	}

	pwHash, err := authutil.HashPassword(password)
	if err != nil {
		a.serverError(w, "hash password", err)
		return
	}
	raw, hash, err := newAuthToken()
	if err != nil {
		a.serverError(w, "generate token", err)
		return
	}
	a.recordFailure(r, throttleVerify, verifyMaxPerEmail, ek)
	id, err := a.AuthTokens.Create(ctx, models.AuthTokenSignup, d.Email, hash, d.Name, pwHash, d.Next, authLinkTTL)
	if err != nil {
		a.serverError(w, "store activation token", err)
		return
	}
	if !a.sendAuthLink(ctx, models.EmailVerifyEmail, id, d.Email, d.Name, "/signup/verify", raw) {
		fail("We could not send the activation email right now. Please try again in a few minutes.")
		return
	}
	sent(w, r, d.Email)
}

func (a *App) signupSentPage(w http.ResponseWriter, r *http.Request, email string) {
	a.authMessage(w, r, http.StatusOK, "Check your email", "Check your email",
		"If "+email+" can be registered, we have sent an activation link to it. Open the link within "+authLinkTTLLabel+" to activate your account. No account exists until you do. If you already have an account, log in or reset your password instead.",
		AuthLink{Label: "Log in", Href: "/login", Primary: true}, AuthLink{Label: "Reset password", Href: "/forgot"})
}

// SignupVerify opens the emailed activation link: it creates the account (or
// adds a password to the existing Google/guest account for that address),
// signs the visitor in, and sends them on. The address is now proven, so
// purchases made as a guest with it are attached.
func (a *App) SignupVerify(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimSpace(r.URL.Query().Get("token"))
	invalid := func() {
		a.authMessage(w, r, http.StatusOK, "Link not valid", "This link is not valid",
			"It has expired, or it was already used. If you already activated your account, log in. Otherwise sign up again and we will send a new link.",
			AuthLink{Label: "Log in", Href: "/login", Primary: true}, AuthLink{Label: "Sign up", Href: "/signup"})
	}
	if len(raw) < 32 || len(raw) > 128 {
		invalid()
		return
	}
	ctx := r.Context()
	t, err := a.AuthTokens.Consume(ctx, models.AuthTokenSignup, authutil.HashKey(raw))
	if errors.Is(err, repo.ErrNotFound) {
		invalid()
		return
	} else if err != nil {
		a.serverError(w, "use activation token", err)
		return
	}
	uid, err := a.Users.ActivatePassword(ctx, t.Email, t.PasswordHash, t.Name)
	if err != nil {
		a.serverError(w, "activate account", err)
		return
	}
	if _, aerr := a.Claims.AttachByEmail(ctx, uid, t.Email); aerr != nil {
		log.Printf("attach guest purchases by verified email: %v", aerr)
	}
	tv, _ := a.Users.TokenVersion(ctx, uid)
	a.Auth.IssueUserSession(w, uid, tv)
	next := safeNextValue(t.NextPath)
	if next == "/" {
		next = "/account"
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (a *App) ForgotPage(w http.ResponseWriter, r *http.Request) {
	a.View.Render(w, http.StatusOK, "forgot", a.authPage(r, "Reset password", PasswordAuthData{
		Error: r.URL.Query().Get("error"),
	}))
}

// ForgotSubmit emails a reset link to a password account. An address that
// signs in only with Google is told so on the page (there is no password to
// reset), and an unknown address gets the same "sent" page as a real one.
func (a *App) ForgotSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form submission", http.StatusBadRequest)
		return
	}
	email := cleanEmail(r.FormValue("email"))
	again := func(msg string) {
		a.View.Render(w, http.StatusOK, "forgot", a.authPage(r, "Reset password", PasswordAuthData{Email: email, Error: msg}))
	}
	if r.FormValue("website") != "" {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if !validEmail(email) {
		again("Enter a valid email address.")
		return
	}
	ik, ek := authutil.HashKey(authutil.ClientIP(r)), authutil.HashKey(email)
	if until, locked := a.throttled(r, throttleForgot, ik, ek); locked {
		again(lockedMessage(until))
		return
	}
	if !a.humanVerified(r) {
		again("We could not confirm you are human. Please complete the check and try again.")
		return
	}
	a.recordFailure(r, throttleForgot, forgotMaxPerIP, ik)
	a.recordFailure(r, throttleForgot, forgotMaxPerEmail, ek)

	ctx := r.Context()
	user, err := a.Users.GetByEmail(ctx, email)
	switch {
	case errors.Is(err, repo.ErrNotFound):
		// Unknown address: same page as a real one, nothing sent.
	case err != nil:
		a.serverError(w, "load user", err)
		return
	case !user.PasswordUsable:
		links := []AuthLink{}
		if a.Google.Enabled() {
			links = append(links, AuthLink{Label: "Continue with Google", Href: "/login/google", Primary: true})
		}
		links = append(links, AuthLink{Label: "Sign up with a password", Href: "/signup"}, AuthLink{Label: "Back to log in", Href: "/login"})
		a.authMessage(w, r, http.StatusOK, "Use Google to log in", "This email signs in with Google",
			"There is no password to reset for "+email+": that account signs in with Google. Use Continue with Google. If you also want a password, sign up with the same email; after you confirm it from the email we send, you can log in either way.",
			links...)
		return
	default:
		raw, hash, terr := newAuthToken()
		if terr != nil {
			a.serverError(w, "generate token", terr)
			return
		}
		id, cerr := a.AuthTokens.Create(ctx, models.AuthTokenReset, email, hash, "", "", "", authLinkTTL)
		if cerr != nil {
			a.serverError(w, "store reset token", cerr)
			return
		}
		if !a.sendAuthLink(ctx, models.EmailPasswordReset, id, email, user.Name, "/forgot/reset", raw) {
			again("We could not send the reset email right now. Please try again in a few minutes.")
			return
		}
	}
	a.authMessage(w, r, http.StatusOK, "Check your email", "Check your email",
		"If "+email+" has a password account, we have sent a reset link to it. It works once and expires in "+authLinkTTLLabel+".",
		AuthLink{Label: "Back to log in", Href: "/login", Primary: true})
}

func (a *App) resetLinkInvalid(w http.ResponseWriter, r *http.Request) {
	a.authMessage(w, r, http.StatusOK, "Link not valid", "This reset link is not valid",
		"It has expired, or it was already used. Ask for a new one.",
		AuthLink{Label: "Reset password", Href: "/forgot", Primary: true})
}

// ResetPage shows the new-password form for a live token. Opening the link
// does not use it, so a mail scanner that fetches the URL cannot burn it.
func (a *App) ResetPage(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimSpace(r.URL.Query().Get("token"))
	if len(raw) < 32 || len(raw) > 128 {
		a.resetLinkInvalid(w, r)
		return
	}
	t, err := a.AuthTokens.Peek(r.Context(), models.AuthTokenReset, authutil.HashKey(raw))
	if errors.Is(err, repo.ErrNotFound) {
		a.resetLinkInvalid(w, r)
		return
	} else if err != nil {
		a.serverError(w, "load reset token", err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	a.View.Render(w, http.StatusOK, "reset_password", a.authPage(r, "Choose a new password", PasswordAuthData{
		Token: raw, Email: t.Email,
	}))
}

// ResetSubmit sets the new password. The token is only used once the
// passwords check out, so a typo does not cost the visitor their link.
func (a *App) ResetSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form submission", http.StatusBadRequest)
		return
	}
	raw := strings.TrimSpace(r.FormValue("token"))
	if len(raw) < 32 || len(raw) > 128 {
		a.resetLinkInvalid(w, r)
		return
	}
	ctx := r.Context()
	hash := authutil.HashKey(raw)
	ik := authutil.HashKey(authutil.ClientIP(r))
	if until, locked := a.throttled(r, throttleReset, ik); locked {
		a.authMessage(w, r, http.StatusOK, "Try again later", "Try again later", lockedMessage(until))
		return
	}
	t, err := a.AuthTokens.Peek(ctx, models.AuthTokenReset, hash)
	if errors.Is(err, repo.ErrNotFound) {
		a.recordFailure(r, throttleReset, resetMaxFails, ik)
		a.resetLinkInvalid(w, r)
		return
	} else if err != nil {
		a.serverError(w, "load reset token", err)
		return
	}
	password, confirm := r.FormValue("password"), r.FormValue("confirm")
	show := func(msg string) {
		a.View.Render(w, http.StatusOK, "reset_password", a.authPage(r, "Choose a new password", PasswordAuthData{
			Token: raw, Email: t.Email, Error: msg,
		}))
	}
	switch {
	case password != confirm:
		show("The two passwords don't match.")
		return
	case authutil.ValidatePassword(password, t.Email) != "":
		show(authutil.ValidatePassword(password, t.Email))
		return
	}
	pwHash, err := authutil.HashPassword(password)
	if err != nil {
		a.serverError(w, "hash password", err)
		return
	}
	if _, err := a.AuthTokens.Consume(ctx, models.AuthTokenReset, hash); err != nil {
		a.resetLinkInvalid(w, r) // used by another request in between
		return
	}
	user, err := a.Users.GetByEmail(ctx, t.Email)
	if err != nil {
		a.resetLinkInvalid(w, r) // account deleted since the link was sent
		return
	}
	if err := a.Users.ResetPassword(ctx, user.ID, pwHash); err != nil {
		a.serverError(w, "reset password", err)
		return
	}
	http.Redirect(w, r, "/login?notice=Password+updated.+Log+in+with+your+new+password.", http.StatusSeeOther)
}
