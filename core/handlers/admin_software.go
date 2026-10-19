package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
)

// Admin CRUD for Software, following the exact list/new/edit/save/delete
// shape admin_case_studies.go established — simpler than course CRUD
// since there's no file upload step (the download is an external Drive
// link, not a Storage object).

type AdminSoftwareListData struct {
	BaseData
	Software []models.Software
}

func (a *App) AdminSoftwareList(w http.ResponseWriter, r *http.Request) {
	list, err := a.Software.ListAll(r.Context())
	if err != nil {
		a.serverError(w, "load software", err)
		return
	}
	a.View.Render(w, http.StatusOK, "admin_software_list", AdminSoftwareListData{
		BaseData: a.base(r, "Software — Admin", "", "admin"),
		Software: list,
	})
}

type AdminSoftwareFormData struct {
	BaseData
	Software models.Software
	IsNew    bool
	Error    string
	SEO      models.SEOFields
}

func (a *App) AdminSoftwareNewPage(w http.ResponseWriter, r *http.Request) {
	a.View.Render(w, http.StatusOK, "admin_software_form", AdminSoftwareFormData{
		BaseData: a.base(r, "New Software — Admin", "", "admin"),
		IsNew:    true,
	})
}

func (a *App) AdminSoftwareEditPage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	sw, err := a.Software.GetByID(r.Context(), id)
	if err != nil {
		if err == repo.ErrNotFound {
			a.NotFound(w, r)
			return
		}
		a.serverError(w, "load software", err)
		return
	}
	seo, _ := a.SEO.GetFields(r.Context(), repo.SEOSoftware, id)
	a.View.Render(w, http.StatusOK, "admin_software_form", AdminSoftwareFormData{
		BaseData: a.base(r, "Edit Software — Admin", "", "admin"),
		Software: sw,
		SEO:      seo,
	})
}

func (a *App) AdminSoftwareSave(w http.ResponseWriter, r *http.Request) {
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
	priceRupees, _ := strconv.ParseFloat(r.FormValue("price"), 64)
	sortOrder, _ := strconv.Atoi(r.FormValue("sort_order"))
	slug := strings.TrimSpace(r.FormValue("slug"))
	title := strings.TrimSpace(r.FormValue("title"))
	if slug == "" {
		slug = slugify(title)
	}
	sw := models.Software{
		ID:                       id,
		Slug:                     slug,
		Title:                    title,
		Version:                  strings.TrimSpace(r.FormValue("version")),
		Summary:                  strings.TrimSpace(r.FormValue("summary")),
		Description:              strings.TrimSpace(r.FormValue("description")),
		RagContext:               strings.TrimSpace(r.FormValue("rag_context")),
		DownloadLink:             strings.TrimSpace(r.FormValue("download_link")),
		PriceCents:               int(priceRupees * 100),
		LicenseTerms:             strings.TrimSpace(r.FormValue("license_terms")),
		PostPurchaseInstructions: strings.TrimSpace(r.FormValue("post_purchase_instructions")),
		IsPublished:              r.FormValue("is_published") == "on",
		SortOrder:                sortOrder,
		LicenceProduct:           strings.TrimSpace(r.FormValue("licence_product")),
		LicenceVersions:          strings.TrimSpace(r.FormValue("licence_versions")),
	}
	if sw.LicenceVersions == "" {
		sw.LicenceVersions = "1"
	}
	if sw.Title == "" || sw.Slug == "" {
		a.View.Render(w, http.StatusUnprocessableEntity, "admin_software_form", AdminSoftwareFormData{
			BaseData: a.base(r, "Software — Admin", "", "admin"),
			Software: sw,
			SEO:      seoFieldsFromForm(r),
			IsNew:    id == 0,
			Error:    "Title is required.",
		})
		return
	}

	ctx := r.Context()
	var err error
	if id == 0 {
		id, err = a.Software.Create(ctx, sw)
	} else {
		err = a.Software.Update(ctx, sw)
	}
	if err != nil {
		a.serverError(w, "save software", err)
		return
	}
	a.saveSEOFields(ctx, repo.SEOSoftware, id, r)
	a.markChatbotReindexPending(ctx)
	http.Redirect(w, r, "/admin/software", http.StatusSeeOther)
}

func (a *App) AdminSoftwareDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	if err := a.Software.Delete(r.Context(), id); err != nil {
		a.serverError(w, "delete software", err)
		return
	}
	a.markChatbotReindexPending(r.Context())
	http.Redirect(w, r, "/admin/software", http.StatusSeeOther)
}
