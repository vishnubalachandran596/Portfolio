package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/vishnubalachandran596/portfolio/core/authutil"
	"github.com/vishnubalachandran596/portfolio/core/repo"
)

// AccountDeleteData drives the "Delete my account" confirmation page.
type AccountDeleteData struct {
	BaseData
	Error       string
	Email       string
	HasPassword bool
	Summary     repo.AccountDeletionSummary
}

const deleteConfirmWord = "DELETE"

// AccountDeletePage explains exactly what deleting the account removes (every
// purchase, licence and certificate is lost, with no refund) and asks for
// confirmation. It deletes nothing.
func (a *App) AccountDeletePage(w http.ResponseWriter, r *http.Request) {
	uid, _ := a.Auth.UserID(r)
	user, err := a.Users.GetByID(r.Context(), uid)
	if err != nil {
		a.serverError(w, "load user", err)
		return
	}
	sum, err := a.Users.DeletionSummary(r.Context(), uid)
	if err != nil {
		a.serverError(w, "deletion summary", err)
		return
	}
	a.View.Render(w, http.StatusOK, "account_delete", AccountDeleteData{
		BaseData:    a.base(r, "Delete my account — "+a.Cfg.SiteName, "", ""),
		Error:       r.URL.Query().Get("error"),
		Email:       user.Email,
		HasPassword: user.PasswordUsable,
		Summary:     sum,
	})
}

// AccountDeleteSubmit permanently deletes the signed-in visitor's account.
// A password account must re-enter its password; a Google account must type
// its email address. Both must type DELETE. Wrong attempts share the login
// throttle so the page cannot be used to guess a password.
func (a *App) AccountDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid, _ := a.Auth.UserID(r)
	_ = r.ParseForm()
	user, err := a.Users.GetByID(ctx, uid)
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
	if strings.TrimSpace(r.FormValue("confirm")) != deleteConfirmWord {
		back("Type DELETE in capital letters to confirm.")
		return
	}
	if user.PasswordUsable {
		if !authutil.CheckPassword(user.PasswordHash, r.FormValue("password")) {
			a.recordFailure(r, throttleLogin, loginMaxFails, ek, ik)
			back("Your password isn't correct.")
			return
		}
	} else if !strings.EqualFold(strings.TrimSpace(r.FormValue("email")), user.Email) {
		a.recordFailure(r, throttleLogin, loginMaxFails, ek, ik)
		back("Type your account email address exactly to confirm.")
		return
	}
	if _, err := a.Admins.GetByEmail(ctx, user.Email); err == nil {
		back("The site administrator account can't be deleted from here.")
		return
	} else if !errors.Is(err, repo.ErrNotFound) {
		a.serverError(w, "admin check", err)
		return
	}
	sum, err := a.Users.DeletionSummary(ctx, uid)
	if err != nil {
		a.serverError(w, "deletion summary", err)
		return
	}
	if sum.UpcomingSlots > 0 {
		back(fmt.Sprintf("You have %d upcoming consultancy booking(s). Cancel or complete them first, or email us, then delete your account.", sum.UpcomingSlots))
		return
	}
	if err := a.Users.DeleteAccount(ctx, uid); err != nil {
		a.serverError(w, "delete account", err)
		return
	}
	a.clearFailures(r, throttleLogin, ek, ik)
	a.Auth.ClearUserSession(w)
	a.Auth.ClearSession(w)
	http.Redirect(w, r, "/login?notice=Your+account+has+been+deleted.", http.StatusSeeOther)
}
