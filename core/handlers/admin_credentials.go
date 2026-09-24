package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
)

// Admin CRUD for the two About-page sections added in migration 046:
// Certifications, and Speaking & Publications. Both follow the shape the
// testimonials handlers in admin_content.go established — list, new, edit,
// save (shared by new and edit), delete — so there is one pattern to learn
// for every curated list in the console rather than one per section.
//
// Both tables ship empty on purpose. There is no seeded example row
// anywhere; these handlers are the only way content gets in, which is the
// point: the content is Vishnu's real credentials and talks, and nobody
// else can supply those honestly.

// parseOptionalDate reads a <input type="date"> value that is allowed to be
// blank. A nil return means "no date on record", which both models render
// differently from a zero date — see models.Certification.
func parseOptionalDate(raw string) *time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	d, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return nil
	}
	return &d
}

// --- Certifications ---------------------------------------------------

type AdminCertificationsListData struct {
	BaseData
	Certifications []models.Certification
}

func (a *App) AdminCertificationsList(w http.ResponseWriter, r *http.Request) {
	list, err := a.Certifications.ListAll(r.Context())
	if err != nil {
		a.serverError(w, "load certifications", err)
		return
	}
	a.View.Render(w, http.StatusOK, "admin_certifications_list", AdminCertificationsListData{
		BaseData:       a.base(r, "Certifications — Admin", "", "admin"),
		Certifications: list,
	})
}

type AdminCertificationFormData struct {
	BaseData
	Certification models.Certification
	IsNew         bool
	Error         string
}

func (a *App) AdminCertificationNewPage(w http.ResponseWriter, r *http.Request) {
	a.View.Render(w, http.StatusOK, "admin_certification_form", AdminCertificationFormData{
		BaseData:      a.base(r, "New Certification — Admin", "", "admin"),
		IsNew:         true,
		Certification: models.Certification{IsPublished: true},
	})
}

func (a *App) AdminCertificationEditPage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	c, err := a.Certifications.GetByID(r.Context(), id)
	if err != nil {
		if err == repo.ErrNotFound {
			a.NotFound(w, r)
			return
		}
		a.serverError(w, "load certification", err)
		return
	}
	a.View.Render(w, http.StatusOK, "admin_certification_form", AdminCertificationFormData{
		BaseData:      a.base(r, "Edit Certification — Admin", "", "admin"),
		Certification: c,
	})
}

func (a *App) AdminCertificationSave(w http.ResponseWriter, r *http.Request) {
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
	c := models.Certification{
		ID:            id,
		Name:          strings.TrimSpace(r.FormValue("name")),
		Issuer:        strings.TrimSpace(r.FormValue("issuer")),
		IssuedOn:      parseOptionalDate(r.FormValue("issued_on")),
		ExpiresOn:     parseOptionalDate(r.FormValue("expires_on")),
		CredentialURL: strings.TrimSpace(r.FormValue("credential_url")),
		IsPublished:   r.FormValue("is_published") == "on",
		SortOrder:     sortOrder,
	}
	if c.Name == "" {
		a.View.Render(w, http.StatusUnprocessableEntity, "admin_certification_form", AdminCertificationFormData{
			BaseData:      a.base(r, "Certification — Admin", "", "admin"),
			Certification: c,
			IsNew:         id == 0,
			Error:         "The certification name is required.",
		})
		return
	}

	ctx := r.Context()
	var err error
	if id == 0 {
		_, err = a.Certifications.Create(ctx, c)
	} else {
		err = a.Certifications.Update(ctx, c)
	}
	if err != nil {
		a.serverError(w, "save certification", err)
		return
	}
	http.Redirect(w, r, "/admin/certifications", http.StatusSeeOther)
}

func (a *App) AdminCertificationDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	if err := a.Certifications.Delete(r.Context(), id); err != nil {
		a.serverError(w, "delete certification", err)
		return
	}
	http.Redirect(w, r, "/admin/certifications", http.StatusSeeOther)
}

// --- Speaking & publications ------------------------------------------

type AdminPublicationsListData struct {
	BaseData
	Publications []models.Publication
}

func (a *App) AdminPublicationsList(w http.ResponseWriter, r *http.Request) {
	list, err := a.Publications.ListAll(r.Context())
	if err != nil {
		a.serverError(w, "load publications", err)
		return
	}
	a.View.Render(w, http.StatusOK, "admin_publications_list", AdminPublicationsListData{
		BaseData:     a.base(r, "Speaking & Publications — Admin", "", "admin"),
		Publications: list,
	})
}

type AdminPublicationFormData struct {
	BaseData
	Publication models.Publication
	IsNew       bool
	Error       string
}

func (a *App) AdminPublicationNewPage(w http.ResponseWriter, r *http.Request) {
	a.View.Render(w, http.StatusOK, "admin_publication_form", AdminPublicationFormData{
		BaseData:    a.base(r, "New Entry — Admin", "", "admin"),
		IsNew:       true,
		Publication: models.Publication{IsPublished: true},
	})
}

func (a *App) AdminPublicationEditPage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	p, err := a.Publications.GetByID(r.Context(), id)
	if err != nil {
		if err == repo.ErrNotFound {
			a.NotFound(w, r)
			return
		}
		a.serverError(w, "load publication", err)
		return
	}
	a.View.Render(w, http.StatusOK, "admin_publication_form", AdminPublicationFormData{
		BaseData:    a.base(r, "Edit Entry — Admin", "", "admin"),
		Publication: p,
	})
}

func (a *App) AdminPublicationSave(w http.ResponseWriter, r *http.Request) {
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
	p := models.Publication{
		ID:          id,
		Title:       strings.TrimSpace(r.FormValue("title")),
		Kind:        strings.TrimSpace(r.FormValue("kind")),
		Venue:       strings.TrimSpace(r.FormValue("venue")),
		URL:         strings.TrimSpace(r.FormValue("url")),
		PublishedOn: parseOptionalDate(r.FormValue("published_on")),
		Summary:     strings.TrimSpace(r.FormValue("summary")),
		IsPublished: r.FormValue("is_published") == "on",
		SortOrder:   sortOrder,
	}
	if p.Title == "" {
		a.View.Render(w, http.StatusUnprocessableEntity, "admin_publication_form", AdminPublicationFormData{
			BaseData:    a.base(r, "Entry — Admin", "", "admin"),
			Publication: p,
			IsNew:       id == 0,
			Error:       "The title is required.",
		})
		return
	}
	// A talk or article that lives somewhere else needs a usable link, and a
	// bare domain typed without a scheme is the common slip.
	if p.URL != "" && !strings.HasPrefix(p.URL, "http://") && !strings.HasPrefix(p.URL, "https://") {
		p.URL = "https://" + p.URL
	}

	ctx := r.Context()
	var err error
	if id == 0 {
		_, err = a.Publications.Create(ctx, p)
	} else {
		err = a.Publications.Update(ctx, p)
	}
	if err != nil {
		a.serverError(w, "save publication", err)
		return
	}
	http.Redirect(w, r, "/admin/publications", http.StatusSeeOther)
}

func (a *App) AdminPublicationDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	if err := a.Publications.Delete(r.Context(), id); err != nil {
		a.serverError(w, "delete publication", err)
		return
	}
	http.Redirect(w, r, "/admin/publications", http.StatusSeeOther)
}
