package handlers

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/view"
)

func renderDL(t *testing.T, name string, data any) string {
	t.Helper()
	r, err := view.New()
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	r.Render(rec, 200, name, data)
	body := rec.Body.String()
	if strings.Contains(body, "<!-- render error:") {
		t.Fatalf("%s: %s", name, body)
	}
	return body
}

func TestDownloadsBlock(t *testing.T) {
	base := BaseData{SiteName: "S", Nav: NavLinks{Home: "/", TempleAccountsPrivacy: "/temple-accounts/privacy"}}
	sw := models.Software{ID: 1, Slug: "temple", Title: "Temple", LicenceProduct: "temple-accounts", DownloadLink: "https://drive.example.test/folder"}

	// No store URL: Coming soon. Old exe link is gone.
	body := renderDL(t, "software_detail", SoftwareDetailData{BaseData: base, Software: sw, HasAccess: true, OwnsSoftware: true})
	for _, want := range []string{
		"Coming soon", "Download from Microsoft Store", "No security warnings, automatic updates.",
		"Open the app and copy the Device ID from the activation screen", `href="/temple-accounts/privacy"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	if !strings.Contains(body, `href="https://drive.example.test/folder"`) || !strings.Contains(body, "Download from Google Drive") {
		t.Error("Drive card missing beside Store")
	}
	for _, gone := range []string{"TempleAccounts-windows.zip", "TempleAccounts.app.zip", "TempleAccounts-linux.deb"} {
		if strings.Contains(body, gone) {
			t.Errorf("hidden build %q still shown", gone)
		}
	}

	// Store URL set: link instead of Coming soon.
	sw.MicrosoftStoreURL = "https://apps.microsoft.com/detail/test"
	body = renderDL(t, "software_detail", SoftwareDetailData{BaseData: base, Software: sw, HasAccess: true, OwnsSoftware: true})
	if !strings.Contains(body, `href="https://apps.microsoft.com/detail/test"`) || strings.Contains(body, "Coming soon") {
		t.Error("store URL not used")
	}

	// Not licensed: no block. Not entitled: no block.
	plain := models.Software{ID: 2, Slug: "x", Title: "X", DownloadLink: "https://example.test/d"}
	if strings.Contains(renderDL(t, "software_detail", SoftwareDetailData{BaseData: base, Software: plain, HasAccess: true}), "TempleAccounts-windows.zip") {
		t.Error("block shown for unlicensed item")
	}
	if strings.Contains(renderDL(t, "software_detail", SoftwareDetailData{BaseData: base, Software: sw}), "TempleAccounts-windows.zip") {
		t.Error("block shown without access")
	}

	// Confirmation page.
	body = renderDL(t, "purchase_success", PurchaseSuccessData{BaseData: base, Kind: "software", Title: "Temple", Attached: true, Licence: true, NextURL: "/software/temple", NextText: "Go"})
	if !strings.Contains(body, "Coming soon") || !strings.Contains(body, "Paste the key into the app.") {
		t.Error("confirmation page missing downloads or steps")
	}
}

func TestAccountLicencesAndAdminFormRender(t *testing.T) {
	base := BaseData{SiteName: "S", Nav: NavLinks{Home: "/"}}
	l := models.Licence{ID: 3, Code: "TAB-1", SoftwareTitle: "Temple", SoftwareSlug: "temple", Key: "VBL1-x.y", DeviceHash: "abc"}
	body := renderDL(t, "account_licences", AccountLicencesData{BaseData: base, Ready: true, Items: []licenceRow{{Licence: l, ChangesLeft: 2}}})
	if strings.Contains(body, `name="ver"`) || strings.Contains(body, "Registered for version") || !strings.Contains(body, "Paste the key into the app.") || !strings.Contains(body, "/account/licences/3/download") {
		t.Error("account licences page wrong")
	}
	form := renderDL(t, "admin_software_form", AdminSoftwareFormData{BaseData: base, Software: models.Software{MicrosoftStoreURL: "https://s.test/x"}})
	if !strings.Contains(form, `name="microsoft_store_url"`) || strings.Contains(form, "licence_versions") {
		t.Error("admin form fields wrong")
	}
}

func TestTempleAccountsPrivacyRender(t *testing.T) {
	body := renderDL(t, "legal", LegalData{BaseData: BaseData{SiteName: "S", Nav: NavLinks{Privacy: "/privacy"}}, Slug: "temple-accounts-privacy", PageTitle: "Temple Accounts Privacy Policy", ContactName: "V"})
	if !strings.Contains(body, "sends no data") || !strings.Contains(body, "Device ID") {
		t.Error("privacy content missing")
	}
}

func TestPromoBannerInfoIcon(t *testing.T) {
	scoped := models.PromoCode{ID: 7, Code: "WELCOMEVB", BannerText: "First course free using promocode WELCOMEVB on signup",
		ScopeItems: []models.PromoScopeItem{{Title: "Java Basics", URL: "/courses/java"}, {Title: "Temple App", URL: "/software/temple"}}}
	open := models.PromoCode{ID: 8, Code: "ALL", DiscountPercent: 10}
	base := BaseData{SiteName: "S", Nav: NavLinks{Home: "/"}, PromoCodes: []models.PromoCode{scoped, open}}
	body := renderDL(t, "software_list", SoftwareListData{BaseData: base})
	for _, want := range []string{`aria-describedby="promo-tip-7"`, `role="tooltip"`, `id="promo-tip-7"`, `href="/courses/java"`, "Temple App", `aria-label="Where code WELCOMEVB applies"`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(body, "promo-tip-8") || strings.Count(body, `class="promo-info-btn"`) != 1 {
		t.Error("unscoped code must have no info icon")
	}
}

func TestAccountDeleteRender(t *testing.T) {
	base := BaseData{SiteName: "S", Nav: NavLinks{Home: "/"}}
	body := renderDL(t, "account_delete", PasswordAuthData{BaseData: base, Email: "a@b.test", HasPassword: true})
	for _, want := range []string{`action="/account/delete"`, `name="confirm"`, `name="current"`, "Delete my account"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	body = renderDL(t, "account_delete", PasswordAuthData{BaseData: base, Email: "a@b.test"})
	if strings.Contains(body, `name="current"`) {
		t.Error("google account must not be asked for a password")
	}
}
