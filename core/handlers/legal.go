package handlers

import "net/http"

type LegalData struct {
	BaseData
	Slug         string
	PageTitle    string
	LastUpdated  string
	ContactName  string
	ContactLoc   string
	ContactEmail string
}

// LegalPage renders one of the static compliance pages (privacy, terms,
// refund, cookies). Content lives in the shared "legal" template, keyed by
// slug, so it's all in one place to keep in sync.
func (a *App) LegalPage(slug, title string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		profile, err := a.Profile.Get(r.Context())
		if err != nil {
			a.serverError(w, "load profile", err)
			return
		}
		a.View.Render(w, http.StatusOK, "legal", LegalData{
			BaseData:     a.base(r, title+" — "+a.Cfg.SiteName, title+" for "+a.Cfg.SiteName, "legal"),
			Slug:         slug,
			PageTitle:    title,
			LastUpdated:  "October 2026",
			ContactName:  profile.Name,
			ContactLoc:   profile.Location,
			ContactEmail: profile.Email,
		})
	}
}
