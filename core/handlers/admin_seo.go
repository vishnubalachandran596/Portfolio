package handlers

import (
	"context"
	"log"
	"net/http"
	"strings"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
)

// seoFieldsFromForm reads the four shared SEO inputs (see the
// admin_seo_fields partial). Values are trimmed and stored as typed:
// length limits are advice shown by a counter, not enforced.
func seoFieldsFromForm(r *http.Request) models.SEOFields {
	return models.SEOFields{
		SEOTitle:        strings.TrimSpace(r.FormValue("seo_title")),
		MetaDescription: strings.TrimSpace(r.FormValue("meta_description")),
		PrimaryKeyword:  strings.TrimSpace(r.FormValue("primary_keyword")),
		OGImage:         strings.TrimSpace(r.FormValue("og_image")),
	}
}

// saveSEOFields stores the SEO inputs of a course, article or software
// form on the row just saved. A failure is logged and never fails the
// save: the content itself is already written, and the fields can simply
// be submitted again.
func (a *App) saveSEOFields(ctx context.Context, kind repo.SEOKind, id int64, r *http.Request) {
	if id == 0 {
		return
	}
	if err := a.SEO.SetFields(ctx, kind, id, seoFieldsFromForm(r)); err != nil {
		log.Printf("save %s seo %d: %v", kind, id, err)
	}
}

type adminSEOPageRow struct {
	models.StaticSEOPage
	Fields      models.SEOFields
	Noindex     bool
	HasOverride bool
}

type AdminSEOData struct {
	BaseData
	Pages []adminSEOPageRow
	Saved string
	Error string
}

// AdminSEOPage lists every static page with its page_seo override (if
// any) as an edit form.
func (a *App) AdminSEOPage(w http.ResponseWriter, r *http.Request) {
	a.renderAdminSEO(w, r, r.URL.Query().Get("saved"), "")
}

func (a *App) renderAdminSEO(w http.ResponseWriter, r *http.Request, saved, errMsg string) {
	overrides, err := a.SEO.ListPages(r.Context())
	if err != nil {
		a.serverError(w, "load page seo", err)
		return
	}
	rows := make([]adminSEOPageRow, 0, len(models.StaticSEOPages))
	for _, p := range models.StaticSEOPages {
		row := adminSEOPageRow{StaticSEOPage: p}
		if o, ok := overrides[p.Path]; ok {
			row.Fields, row.Noindex, row.HasOverride = o.SEOFields, o.Noindex, true
		}
		rows = append(rows, row)
	}
	a.View.Render(w, http.StatusOK, "admin_seo", AdminSEOData{
		BaseData: a.base(r, "SEO — Admin", "", "admin"),
		Pages:    rows,
		Saved:    saved,
		Error:    errMsg,
	})
}

// AdminSEOSave upserts one page's override. A form with every field blank
// and noindex off is stored as no row at all (same as "Reset to default"),
// so page_seo only ever holds real overrides.
func (a *App) AdminSEOSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	path := r.FormValue("path")
	if !models.IsStaticSEOPath(path) {
		a.renderAdminSEO(w, r, "", "Unknown page.")
		return
	}
	f := seoFieldsFromForm(r)
	noindex := r.FormValue("noindex") == "1"
	var err error
	if r.FormValue("reset") == "1" || (f == models.SEOFields{} && !noindex) {
		err = a.SEO.DeletePage(r.Context(), path)
	} else {
		err = a.SEO.UpsertPage(r.Context(), models.PageSEO{Path: path, SEOFields: f, Noindex: noindex})
	}
	if err != nil {
		log.Printf("save page seo %s: %v", path, err)
		a.renderAdminSEO(w, r, "", "Could not save. Please try again.")
		return
	}
	http.Redirect(w, r, "/admin/seo?saved="+path, http.StatusSeeOther)
}
