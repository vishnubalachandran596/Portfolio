package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
)

// Admin CRUD for Case Studies (migration 060), following the exact shape
// admin_credentials.go established for Certifications/Publications: list,
// new, edit, save (shared by new and edit), delete.
//
// Ships empty on purpose, same as Certifications/Publications — this is
// real engagement detail only Vishnu can supply accurately and with any
// client's consent (see the Consultancy FAQ's confidentiality answer), so
// there is no seeded example row anywhere.

type AdminCaseStudiesListData struct {
	BaseData
	CaseStudies []models.CaseStudy
}

func (a *App) AdminCaseStudiesList(w http.ResponseWriter, r *http.Request) {
	list, err := a.CaseStudies.ListAll(r.Context())
	if err != nil {
		a.serverError(w, "load case studies", err)
		return
	}
	a.View.Render(w, http.StatusOK, "admin_case_studies_list", AdminCaseStudiesListData{
		BaseData:    a.base(r, "Case Studies — Admin", "", "admin"),
		CaseStudies: list,
	})
}

type AdminCaseStudyFormData struct {
	BaseData
	CaseStudy models.CaseStudy
	IsNew     bool
	Error     string
}

func (a *App) AdminCaseStudyNewPage(w http.ResponseWriter, r *http.Request) {
	a.View.Render(w, http.StatusOK, "admin_case_study_form", AdminCaseStudyFormData{
		BaseData:  a.base(r, "New Case Study — Admin", "", "admin"),
		IsNew:     true,
		CaseStudy: models.CaseStudy{IsPublished: true},
	})
}

func (a *App) AdminCaseStudyEditPage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	c, err := a.CaseStudies.GetByID(r.Context(), id)
	if err != nil {
		if err == repo.ErrNotFound {
			a.NotFound(w, r)
			return
		}
		a.serverError(w, "load case study", err)
		return
	}
	a.View.Render(w, http.StatusOK, "admin_case_study_form", AdminCaseStudyFormData{
		BaseData:  a.base(r, "Edit Case Study — Admin", "", "admin"),
		CaseStudy: c,
	})
}

func (a *App) AdminCaseStudySave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	var id int64
	if idStr := r.PathValue("id"); idStr != "" {
		var err error
		id, err = strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			a.NotFound(w, r)
			return
		}
	}
	sortOrder, _ := strconv.Atoi(r.FormValue("sort_order"))
	c := models.CaseStudy{
		ID:            id,
		Title:         strings.TrimSpace(r.FormValue("title")),
		ClientContext: strings.TrimSpace(r.FormValue("client_context")),
		Problem:       strings.TrimSpace(r.FormValue("problem")),
		Approach:      strings.TrimSpace(r.FormValue("approach")),
		Result:        strings.TrimSpace(r.FormValue("result")),
		IsPublished:   r.FormValue("is_published") == "on",
		SortOrder:     sortOrder,
	}
	if c.Title == "" || c.Problem == "" || c.Approach == "" || c.Result == "" {
		a.View.Render(w, http.StatusUnprocessableEntity, "admin_case_study_form", AdminCaseStudyFormData{
			BaseData:  a.base(r, "Case Study — Admin", "", "admin"),
			CaseStudy: c,
			IsNew:     id == 0,
			Error:     "Title, Problem, Approach, and Result are all required.",
		})
		return
	}

	ctx := r.Context()
	var err error
	if id == 0 {
		_, err = a.CaseStudies.Create(ctx, c)
	} else {
		err = a.CaseStudies.Update(ctx, c)
	}
	if err != nil {
		a.serverError(w, "save case study", err)
		return
	}
	http.Redirect(w, r, "/admin/case-studies", http.StatusSeeOther)
}

func (a *App) AdminCaseStudyDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	if err := a.CaseStudies.Delete(r.Context(), id); err != nil {
		a.serverError(w, "delete case study", err)
		return
	}
	http.Redirect(w, r, "/admin/case-studies", http.StatusSeeOther)
}
