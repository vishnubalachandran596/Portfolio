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

// AdminContactMessageDelete removes a message (spam clean-up). The admin
// clicks it; nothing deletes messages automatically.
func (a *App) AdminContactMessageDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	if err := a.ContactMessages.Delete(r.Context(), id); err != nil {
		a.serverError(w, "delete contact message", err)
		return
	}
	http.Redirect(w, r, "/admin/contact-messages", http.StatusSeeOther)
}

// AdminContactMessagesBulkDelete removes every ticked message (spam
// clean-up). The admin ticks and confirms; nothing runs automatically.
func (a *App) AdminContactMessagesBulkDelete(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	var ids []int64
	for _, v := range r.Form["id"] {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil && id > 0 {
			ids = append(ids, id)
		}
	}
	if len(ids) > 500 {
		http.Error(w, "too many messages selected", http.StatusBadRequest)
		return
	}
	if _, err := a.ContactMessages.DeleteMany(r.Context(), ids); err != nil {
		a.serverError(w, "bulk delete contact messages", err)
		return
	}
	http.Redirect(w, r, "/admin/contact-messages", http.StatusSeeOther)
}
