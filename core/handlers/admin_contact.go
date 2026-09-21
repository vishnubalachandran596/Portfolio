package handlers

import (
	"net/http"
	"strconv"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

type AdminContactMessagesData struct {
	BaseData
	Messages []models.ContactMessage
}

// AdminContactMessagesList shows every /contact form submission,
// newest first — the admin's view into the one non-scheduling way a
// visitor can reach out on this site.
func (a *App) AdminContactMessagesList(w http.ResponseWriter, r *http.Request) {
	messages, err := a.ContactMessages.ListAll(r.Context())
	if err != nil {
		a.serverError(w, "load contact messages", err)
		return
	}
	a.View.Render(w, http.StatusOK, "admin_contact_messages", AdminContactMessagesData{
		BaseData: a.base(r, "Contact Messages — Admin", "", "admin"),
		Messages: messages,
	})
}

func (a *App) AdminContactMessageMarkRead(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	if err := a.ContactMessages.MarkRead(r.Context(), id); err != nil {
		a.serverError(w, "mark contact message read", err)
		return
	}
	http.Redirect(w, r, "/admin/contact-messages", http.StatusSeeOther)
}
