package handlers

import (
	"log"
	"net/http"
	"net/mail"
	"strconv"
	"strings"

	"github.com/vishnubalachandran596/portfolio/core/authutil"
	"github.com/vishnubalachandran596/portfolio/core/models"
)

// Throttle for POST /subscribe: a confirmation email goes to whatever address
// is typed, so one IP must not be able to send it to many addresses.
const (
	throttleSubscribe = "subscribe"
	subscribeMaxPerIP = 5
)

// NewsletterSubscribe records an email-capture submission from Courses,
// Articles, or Home (POST /subscribe) and sends a confirmation when Brevo
// is configured. Campaign blasts are not sent from here. The redirect
// target is a fixed switch on the source label rather than anything
// client-supplied, so there is no open-redirect surface here.
func (a *App) NewsletterSubscribe(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	source := r.FormValue("source")
	email := strings.TrimSpace(r.FormValue("email"))

	redirectTo := "/"
	switch source {
	case "courses":
		redirectTo = "/courses"
	case "articles":
		redirectTo = "/articles"
	case "home":
		redirectTo = "/"
	default:
		source = "other"
	}

	// Honeypot: real visitors never fill this hidden field.
	if r.FormValue("website") != "" {
		http.Redirect(w, r, redirectTo+"?subscribed=1", http.StatusSeeOther)
		return
	}
	email = strings.ToLower(email)
	if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email || len(email) > 254 || !asciiOnly(email) {
		http.Redirect(w, r, redirectTo, http.StatusSeeOther)
		return
	}
	ik := authutil.HashKey(authutil.ClientIP(r))
	if _, locked := a.throttled(r, throttleSubscribe, ik); locked {
		http.Redirect(w, r, redirectTo, http.StatusSeeOther)
		return
	}
	a.recordFailure(r, throttleSubscribe, subscribeMaxPerIP, ik)
	if !a.humanVerified(r) {
		http.Redirect(w, r, redirectTo, http.StatusSeeOther)
		return
	}

	created, err := a.Subscribers.Create(r.Context(), email, source)
	if err != nil {
		a.serverError(w, "record subscriber", err)
		return
	}
	if created {
		a.Telegram.Send("📬 New subscriber (" + source + "): " + email)
		sub, serr := a.Subscribers.GetByEmail(r.Context(), email)
		if serr != nil {
			log.Printf("newsletter confirm lookup: %v", serr)
		} else {
			a.emailNewsletterConfirm(r.Context(), sub.ID, sub.Email)
		}
	}

	http.Redirect(w, r, redirectTo+"?subscribed=1", http.StatusSeeOther)
}

type AdminSubscribersListData struct {
	BaseData
	Subscribers []models.Subscriber
}

// AdminSubscribersList is a read-only inbox, newest first, mirroring the
// admin_contact.go Contact Messages pattern — the admin follows up
// manually from here, there's no reply-from-admin feature.
func (a *App) AdminSubscribersList(w http.ResponseWriter, r *http.Request) {
	list, err := a.Subscribers.ListAll(r.Context())
	if err != nil {
		a.serverError(w, "load subscribers", err)
		return
	}
	a.View.Render(w, http.StatusOK, "admin_subscribers_list", AdminSubscribersListData{
		BaseData:    a.base(r, "Subscribers — Admin", "", "admin"),
		Subscribers: list,
	})
}

func (a *App) AdminSubscriberDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	if err := a.Subscribers.Delete(r.Context(), id); err != nil {
		a.serverError(w, "delete subscriber", err)
		return
	}
	http.Redirect(w, r, "/admin/subscribers", http.StatusSeeOther)
}
