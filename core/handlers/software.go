package handlers

import (
	"html/template"
	"net/http"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
)

type SoftwareListData struct {
	BaseData
	Software []models.Software
}

// SoftwareList is the public catalog of downloadable software products --
// the simplest of the catalog pages (no search/tag/sort/pagination, since
// there's currently only ever a handful of items, unlike courses/articles).
func (a *App) SoftwareList(w http.ResponseWriter, r *http.Request) {
	pf := a.startBase(r)
	list, err := a.Software.ListPublished(r.Context())
	if err != nil {
		a.serverError(w, "load software", err)
		return
	}
	base := a.baseFrom(r, pf, "Software — "+a.Cfg.SiteName, "Downloadable apps and tools, built and sold directly.", "software")
	a.View.Render(w, http.StatusOK, "software_list", SoftwareListData{BaseData: base, Software: list})
}

type SoftwareDetailData struct {
	BaseData
	Software         models.Software
	JSONLD           template.JS
	PaymentsEnabled  bool
	RazorpayKeyID    string
	PurchaseRequired bool
	HasAccess        bool
	UserEmail        string
}

func (a *App) SoftwareDetail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	slug := r.PathValue("slug")

	pf := a.startBase(r)
	sw, err := a.Software.GetBySlug(ctx, slug)
	if err != nil {
		if err == repo.ErrNotFound {
			a.NotFound(w, r)
			return
		}
		a.serverError(w, "load software", err)
		return
	}
	isAdmin := a.Auth.IsAuthenticated(r) && !a.isPreviewingAsVisitor(r)
	if !sw.IsPublished && !isAdmin {
		a.NotFound(w, r)
		return
	}

	userEmail := ""
	if uid, ok := a.Auth.UserID(r); ok {
		if u, err := a.Users.GetByID(ctx, uid); err == nil {
			userEmail = u.Email
		}
	}

	hasAccess := a.softwareAccess(w, r, sw)

	// Summary, not Description -- Description runs to ~1,000 chars and
	// Google truncates/ignores an over-long meta description (seen live:
	// the stats/listing audit flagged this page's snippet going stale/
	// inconsistent because of it). Fall back to Description only if a
	// software item predates the Summary field and has none set.
	metaDescription := sw.Summary
	if metaDescription == "" {
		metaDescription = sw.Description
	}
	base := a.baseFrom(r, pf, sw.Title+" — "+a.Cfg.SiteName, metaDescription, "software")

	a.View.Render(w, http.StatusOK, "software_detail", SoftwareDetailData{
		BaseData:         base,
		Software:         sw,
		JSONLD:           softwareApplicationJSONLD(sw, base.CanonicalURL),
		PaymentsEnabled:  a.Razorpay.Enabled(),
		RazorpayKeyID:    a.Razorpay.KeyID,
		PurchaseRequired: r.URL.Query().Get("purchase") == "required",
		HasAccess:        hasAccess,
		UserEmail:        userEmail,
	})
}
