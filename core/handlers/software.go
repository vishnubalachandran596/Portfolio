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
	base.BreadcrumbJSONLD = breadcrumbJSONLD(a.trail(r, crumb{"Software", a.pageURL(r, "/software")}))
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
	OwnsSoftware     bool // signed-in visitor with a paid purchase of this item
	UserEmail        string
}

func (a *App) SoftwareDetail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	slug := r.PathValue("slug")
	seoWait := a.seoFieldsAsync(ctx, repo.SEOSoftware, slug)

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
	ownsSoftware := false
	if uid, ok := a.Auth.UserID(r); ok {
		if u, err := a.Users.GetByID(ctx, uid); err == nil {
			userEmail = u.Email
		}
		// A signed-in buyer who already paid for this item is pointed at
		// their licence instead of being offered checkout again.
		if paid, err := a.SoftwarePurchases.ListPaidByUserID(ctx, uid); err == nil {
			for _, p := range paid {
				if p.SoftwareID == sw.ID {
					ownsSoftware = true
					break
				}
			}
		}
	}

	hasAccess := a.softwareAccess(w, r, sw)

	// Fallback chain: meta_description, then Summary, then a truncated
	// Description -- never the full ~1,000-char Description, which Google
	// truncates or ignores (seen live: the listing audit flagged this
	// page's snippet going stale because of it).
	seo := seoWait()
	description := seo.Description(sw.Summary, sw.Description)
	base := a.baseFrom(r, pf, seo.Title(sw.Title+" — "+a.Cfg.SiteName), description, "software")
	base.OGImageURL = ogImageFor(a.baseURL(r), seo, "")
	base.BreadcrumbJSONLD = breadcrumbJSONLD(a.trail(r,
		crumb{"Software", a.pageURL(r, "/software")}, crumb{sw.Title, base.CanonicalURL}))

	a.View.Render(w, http.StatusOK, "software_detail", SoftwareDetailData{
		BaseData:         base,
		Software:         sw,
		JSONLD:           softwareApplicationJSONLD(sw, base.CanonicalURL, description, base.OGImageURL),
		PaymentsEnabled:  a.Razorpay.Enabled(),
		RazorpayKeyID:    a.Razorpay.KeyID,
		PurchaseRequired: r.URL.Query().Get("purchase") == "required",
		HasAccess:        hasAccess || ownsSoftware,
		OwnsSoftware:     ownsSoftware,
		UserEmail:        userEmail,
	})
}
