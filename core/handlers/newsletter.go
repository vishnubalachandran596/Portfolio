package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

// NewsletterSubscribe records an email-capture submission from Courses,
// Articles, or Home (POST /subscribe). There is no transactional email
// provider wired up anywhere in this codebase, so nothing automated is
// sent back — this is capture only, same scoping as Contact's own
// confirmation-email gap (see docs/FUNCTIONALITY.md). The redirect target
// is a fixed switch on the source label rather than anything client-
// supplied, so there is no open-redirect surface here.
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

	if email == "" || !strings.Contains(email, "@") {
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
