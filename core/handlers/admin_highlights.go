package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
)

// Admin CRUD for the home page's stat strip (profile_highlights, migration
// 078). Same list/new/edit/save/delete shape as admin_credentials.go. The
// table ships empty: until it has a published row, Home falls back to the
// profile's years of experience so the strip is never invented.

type AdminHighlightsListData struct {
	BaseData
	Highlights []models.ProfileHighlight
}

func (a *App) AdminHighlightsList(w http.ResponseWriter, r *http.Request) {
	list, err := a.Highlights.ListAll(r.Context())
	if err != nil {
		a.serverError(w, "load highlights", err)
		return
	}
	a.View.Render(w, http.StatusOK, "admin_highlights_list", AdminHighlightsListData{
		BaseData:   a.base(r, "Home highlights — Admin", "", "admin"),
		Highlights: list,
	})
}

type AdminHighlightFormData struct {
	BaseData
	Highlight models.ProfileHighlight
	IsNew     bool
	Error     string
}

func (a *App) AdminHighlightNewPage(w http.ResponseWriter, r *http.Request) {
	a.View.Render(w, http.StatusOK, "admin_highlight_form", AdminHighlightFormData{
		BaseData:  a.base(r, "New highlight — Admin", "", "admin"),
		IsNew:     true,
		Highlight: models.ProfileHighlight{IsPublished: true},
	})
}

func (a *App) AdminHighlightEditPage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	h, err := a.Highlights.GetByID(r.Context(), id)
	if err != nil {
		if err == repo.ErrNotFound {
			a.NotFound(w, r)
			return
		}
		a.serverError(w, "load highlight", err)
		return
	}
	a.View.Render(w, http.StatusOK, "admin_highlight_form", AdminHighlightFormData{
		BaseData:  a.base(r, "Edit highlight — Admin", "", "admin"),
		Highlight: h,
	})
}

func (a *App) AdminHighlightSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	var id int64
	if idStr := r.PathValue("id"); idStr != "" {
		var err error
		if id, err = strconv.ParseInt(idStr, 10, 64); err != nil {
			a.NotFound(w, r)
			return
		}
	}
	sortOrder, _ := strconv.Atoi(r.FormValue("sort_order"))
	h := models.ProfileHighlight{
		ID:          id,
		Value:       strings.TrimSpace(r.FormValue("value")),
		Label:       strings.TrimSpace(r.FormValue("label")),
		SortOrder:   sortOrder,
		IsPublished: r.FormValue("is_published") == "on",
	}
	if h.Value == "" || h.Label == "" {
		a.View.Render(w, http.StatusUnprocessableEntity, "admin_highlight_form", AdminHighlightFormData{
			BaseData:  a.base(r, "Highlight — Admin", "", "admin"),
			Highlight: h,
			IsNew:     id == 0,
			Error:     "Both the value and its label are required.",
		})
		return
	}
	var err error
	if id == 0 {
		_, err = a.Highlights.Create(r.Context(), h)
	} else {
		err = a.Highlights.Update(r.Context(), h)
	}
	if err != nil {
		a.serverError(w, "save highlight", err)
		return
	}
	http.Redirect(w, r, "/admin/highlights", http.StatusSeeOther)
}

func (a *App) AdminHighlightDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	if err := a.Highlights.Delete(r.Context(), id); err != nil {
		a.serverError(w, "delete highlight", err)
		return
	}
	http.Redirect(w, r, "/admin/highlights", http.StatusSeeOther)
}
