package handlers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/config"
	"github.com/vishnubalachandran596/portfolio/core/licence"
	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
)

// selfDeviceChangeLimit is how many times an owner may move a licence to a
// different computer per rolling year without asking the admin.
const (
	selfDeviceChangeLimit  = 2
	selfDeviceChangeWindow = 365 * 24 * time.Hour
)

// newLicenceSigner builds the signer from the environment secret. It
// returns nil (issuing disabled) when unset or invalid; the error text
// never includes key material.
func newLicenceSigner(cfg *config.Config) *licence.Signer {
	if strings.TrimSpace(cfg.LicenceSigningKeyB64) == "" || strings.TrimSpace(cfg.LicenceKID) == "" {
		return nil
	}
	s, err := licence.NewSigner(cfg.LicenceSigningKeyB64, cfg.LicenceKID)
	if err != nil {
		log.Printf("licence signer disabled: %v", err)
		return nil
	}
	return s
}

var (
	errLicenceOff = errors.New("Licence keys are not switched on yet. Please try again later.")
	errNoProduct  = errors.New("This software has no licence keys.")
)

// issueKey signs a key for lic and stores it. event/actor/detail feed the
// audit trail. The caller has already applied any owner-side limits.
func (a *App) issueKey(ctx context.Context, lic models.Licence, device, event, actor, detail string) error {
	if a.LicenceSigner == nil {
		return errLicenceOff
	}
	sw, err := a.Software.GetByID(ctx, lic.SoftwareID)
	if err != nil {
		return err
	}
	if sw.LicenceProduct == "" {
		return errNoProduct
	}
	dev, err := licence.NormaliseDevice(device)
	if err != nil {
		return err
	}
	iat := time.Now().UTC().Truncate(time.Second)
	key, err := a.LicenceSigner.Issue(sw.LicenceProduct, lic.Code, dev, iat)
	if err != nil {
		return err
	}
	return a.Licences.SetKey(ctx, lic.ID, dev, key, a.LicenceSigner.KID(), iat, event, actor, detail)
}

// userMessage turns an issue error into text safe to show: validation
// messages pass through, anything else becomes a generic line.
func userMessage(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, errLicenceOff), errors.Is(err, errNoProduct):
		return err.Error()
	}
	msg := err.Error()
	if strings.HasPrefix(msg, "Enter ") || strings.HasPrefix(msg, "The Device") ||
		strings.HasPrefix(msg, "That Device") {
		return msg
	}
	log.Printf("licence issue: %v", err)
	return "Could not create the key. Please try again."
}

func serveLicFile(w http.ResponseWriter, l models.Licence) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="temple-accounts-%s.lic"`, l.Code))
	fmt.Fprint(w, l.Key)
}

// ----- Visitor: My licences ------------------------------------------

type licenceRow struct {
	models.Licence
	ChangesLeft int
}

// licenceBuy is one "Get a new licence" box: a published licensed item the
// account has already paid for, offered again at its fixed price.
type licenceBuy struct {
	Slug  string
	Title string
	Price string
}

type AccountLicencesData struct {
	BaseData
	Items           []licenceRow
	Buy             []licenceBuy
	PaymentsEnabled bool
	Ready           bool
	Notice          string
	Error           string
	ErrID           int64
}

func (a *App) loadLicenceRows(ctx context.Context, list []models.Licence) []licenceRow {
	rows := make([]licenceRow, 0, len(list))
	since := time.Now().Add(-selfDeviceChangeWindow)
	for _, l := range list {
		left := selfDeviceChangeLimit
		if l.Issued() {
			if n, err := a.Licences.SelfDeviceChanges(ctx, l.ID, since); err == nil {
				left = selfDeviceChangeLimit - n
			}
			if left < 0 {
				left = 0
			}
		}
		rows = append(rows, licenceRow{Licence: l, ChangesLeft: left})
	}
	return rows
}

// loadLicenceBuy lists the items this account may buy another licence for:
// published, licensed, and already paid for at least once.
func (a *App) loadLicenceBuy(ctx context.Context, uid int64) []licenceBuy {
	paid, err := a.SoftwarePurchases.ListPaidByUserID(ctx, uid)
	if err != nil {
		return nil
	}
	seen := map[int64]bool{}
	var out []licenceBuy
	for i := len(paid) - 1; i >= 0; i-- { // oldest purchase first
		id := paid[i].SoftwareID
		if seen[id] {
			continue
		}
		seen[id] = true
		sw, err := a.Software.GetByID(ctx, id)
		if err != nil || !sw.IsPublished || sw.LicenceProduct == "" {
			continue
		}
		out = append(out, licenceBuy{Slug: sw.Slug, Title: sw.Title, Price: sw.PriceDisplay()})
	}
	return out
}

func (a *App) renderAccountLicences(w http.ResponseWriter, r *http.Request, uid int64, status int, notice, errMsg string, errID int64) {
	list, err := a.Licences.ListByUser(r.Context(), uid)
	if err != nil {
		a.serverError(w, "load licences", err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	a.View.Render(w, status, "account_licences", AccountLicencesData{
		BaseData:        a.base(r, "My licences", "Your software licence keys.", "account"),
		Items:           a.loadLicenceRows(r.Context(), list),
		Buy:             a.loadLicenceBuy(r.Context(), uid),
		PaymentsEnabled: a.Razorpay.Enabled(),
		Ready:           a.LicenceSigner != nil,
		Notice:          notice,
		Error:           errMsg,
		ErrID:           errID,
	})
}

func (a *App) AccountLicences(w http.ResponseWriter, r *http.Request) {
	uid, _ := a.Auth.UserID(r)
	notice := ""
	if r.URL.Query().Get("done") == "1" {
		notice = "Your key is ready below."
	}
	if r.URL.Query().Get("new") == "1" {
		notice = "Payment received. Your new licence is below: choose the app version and enter the Device ID of the other computer."
	}
	a.renderAccountLicences(w, r, uid, http.StatusOK, notice, "", 0)
}

func (a *App) ownedLicence(w http.ResponseWriter, r *http.Request) (models.Licence, int64, bool) {
	uid, _ := a.Auth.UserID(r)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return models.Licence{}, 0, false
	}
	l, err := a.Licences.Get(r.Context(), id)
	if err != nil || l.UserID != uid {
		a.NotFound(w, r)
		return models.Licence{}, 0, false
	}
	return l, uid, true
}

// AccountLicenceIssue creates a key from a Device ID (first time) or moves
// the licence to another computer (limited per year; the admin can override).
func (a *App) AccountLicenceIssue(w http.ResponseWriter, r *http.Request) {
	l, uid, ok := a.ownedLicence(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	fail := func(msg string) { a.renderAccountLicences(w, r, uid, http.StatusUnprocessableEntity, "", msg, l.ID) }
	if l.Revoked() {
		fail("This licence has been revoked. Please contact support.")
		return
	}
	event, detail := "issued", ""
	if l.Issued() {
		dev, err := licence.NormaliseDevice(r.FormValue("device"))
		if err != nil {
			fail(userMessage(err))
			return
		}
		if dev == l.DeviceHash {
			http.Redirect(w, r, "/account/licences?done=1#lic-"+strconv.FormatInt(l.ID, 10), http.StatusSeeOther)
			return
		}
		// Same Device ID: the key already on the page is the right one.
		// A different one moves the licence to another computer.
		n, err := a.Licences.SelfDeviceChanges(r.Context(), l.ID, time.Now().Add(-selfDeviceChangeWindow))
		if err != nil {
			a.serverError(w, "count device changes", err)
			return
		}
		if n >= selfDeviceChangeLimit {
			fail("You have used both computer changes for this year. Please contact support to move this licence.")
			return
		}
		event, detail = "device_change", "from "+l.DeviceHash
	}
	if err := a.issueKey(r.Context(), l, r.FormValue("device"), event, "user", detail); err != nil {
		fail(userMessage(err))
		return
	}
	http.Redirect(w, r, "/account/licences?done=1#lic-"+strconv.FormatInt(l.ID, 10), http.StatusSeeOther)
}

func (a *App) AccountLicenceDownload(w http.ResponseWriter, r *http.Request) {
	l, _, ok := a.ownedLicence(w, r)
	if !ok {
		return
	}
	if !l.Issued() || l.Revoked() {
		a.NotFound(w, r)
		return
	}
	serveLicFile(w, l)
}

// ----- Admin ---------------------------------------------------------

type AdminLicencesData struct {
	BaseData
	Items     []models.Licence
	Software  []models.Software
	Q         string
	Ready     bool
	KID       string
	PublicKey string
	Notice    string
	Error     string
}

func (a *App) renderAdminLicences(w http.ResponseWriter, r *http.Request, status int, notice, errMsg string) {
	ctx := r.Context()
	q := r.URL.Query().Get("q")
	items, err := a.Licences.Search(ctx, q, 200)
	if err != nil {
		a.serverError(w, "load licences", err)
		return
	}
	sw, err := a.Software.ListAll(ctx)
	if err != nil {
		a.serverError(w, "load software", err)
		return
	}
	var licensed []models.Software
	for _, s := range sw {
		if s.LicenceProduct != "" {
			licensed = append(licensed, s)
		}
	}
	d := AdminLicencesData{
		BaseData: a.base(r, "Licences — Admin", "", "admin"),
		Items:    items, Software: licensed, Q: q, Ready: a.LicenceSigner != nil,
		Notice: notice, Error: errMsg,
	}
	if a.LicenceSigner != nil {
		d.KID, d.PublicKey = a.LicenceSigner.KID(), a.LicenceSigner.PublicKeyB64()
	}
	w.Header().Set("Cache-Control", "no-store")
	a.View.Render(w, status, "admin_licences", d)
}

func (a *App) AdminLicences(w http.ResponseWriter, r *http.Request) {
	a.renderAdminLicences(w, r, http.StatusOK, r.URL.Query().Get("notice"), "")
}

// AdminLicenceAdd gives a user a licence with no order behind it.
func (a *App) AdminLicenceAdd(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	swID, _ := strconv.ParseInt(r.FormValue("software_id"), 10, 64)
	sw, err := a.Software.GetByID(ctx, swID)
	if err != nil || sw.LicenceProduct == "" {
		a.renderAdminLicences(w, r, http.StatusUnprocessableEntity, "", "Pick a software item that has a licence product id.")
		return
	}
	u, err := a.Users.GetByEmail(ctx, cleanEmail(r.FormValue("email")))
	if err != nil {
		a.renderAdminLicences(w, r, http.StatusUnprocessableEntity, "", "No user has that email. They need to sign up first.")
		return
	}
	if _, err := a.Licences.AddByAdmin(ctx, sw.ID, u.ID, strings.TrimSpace(r.FormValue("note"))); err != nil {
		a.serverError(w, "add licence", err)
		return
	}
	http.Redirect(w, r, "/admin/licences?notice=Licence+added", http.StatusSeeOther)
}

func (a *App) adminLicenceFromPath(w http.ResponseWriter, r *http.Request) (models.Licence, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return models.Licence{}, false
	}
	l, err := a.Licences.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			a.NotFound(w, r)
		} else {
			a.serverError(w, "load licence", err)
		}
		return models.Licence{}, false
	}
	return l, true
}

// AdminLicenceReissue signs a key for any device, bypassing the owner's
// yearly limit, and un-revokes the licence.
func (a *App) AdminLicenceReissue(w http.ResponseWriter, r *http.Request) {
	l, ok := a.adminLicenceFromPath(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	if err := a.issueKey(r.Context(), l, r.FormValue("device"), "admin_reissue", "admin", strings.TrimSpace(r.FormValue("reason"))); err != nil {
		a.renderAdminLicences(w, r, http.StatusUnprocessableEntity, "", userMessage(err))
		return
	}
	http.Redirect(w, r, "/admin/licences?notice=Key+reissued", http.StatusSeeOther)
}

func (a *App) AdminLicenceRevoke(w http.ResponseWriter, r *http.Request) {
	l, ok := a.adminLicenceFromPath(w, r)
	if !ok {
		return
	}
	_ = r.ParseForm()
	if err := a.Licences.Revoke(r.Context(), l.ID, strings.TrimSpace(r.FormValue("reason"))); err != nil {
		a.serverError(w, "revoke licence", err)
		return
	}
	http.Redirect(w, r, "/admin/licences?notice=Licence+revoked", http.StatusSeeOther)
}

func (a *App) AdminLicenceDownload(w http.ResponseWriter, r *http.Request) {
	l, ok := a.adminLicenceFromPath(w, r)
	if !ok || !l.Issued() {
		if ok {
			a.NotFound(w, r)
		}
		return
	}
	serveLicFile(w, l)
}

// ----- Admin: key-pair generator -------------------------------------

type AdminKeygenData struct {
	BaseData
	Generated  bool
	KID        string
	PublicKey  string
	PrivateKey string
	ConfigJSON string
	Error      string
	ServerKID  string
	ServerPub  string
}

func (a *App) keygenBase(r *http.Request) AdminKeygenData {
	d := AdminKeygenData{BaseData: a.base(r, "Licence key pair — Admin", "", "admin")}
	if a.LicenceSigner != nil {
		d.ServerKID, d.ServerPub = a.LicenceSigner.KID(), a.LicenceSigner.PublicKeyB64()
	}
	return d
}

func (a *App) AdminLicenceKeygenPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	a.View.Render(w, http.StatusOK, "admin_licence_keygen", a.keygenBase(r))
}

// AdminLicenceKeygen makes a new Ed25519 pair and shows it once. The
// private key is never stored, logged or cached: the response is
// no-store and only a POST (same-origin enforced) creates it.
func (a *App) AdminLicenceKeygen(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store, private")
	w.Header().Set("Pragma", "no-cache")
	_ = r.ParseForm()
	d := a.keygenBase(r)
	kid := strings.TrimSpace(r.FormValue("kid"))
	if kid == "" {
		kid = "vb-" + strconv.Itoa(time.Now().Year()) + "-1"
	}
	if len(kid) > 40 || strings.ContainsAny(kid, " \"\\<>&\n\r\t") {
		d.Error = "Use a short key id without spaces or quotes, e.g. vb-2026-1."
		a.View.Render(w, http.StatusUnprocessableEntity, "admin_licence_keygen", d)
		return
	}
	priv, pub, err := licence.GenerateKeyPair()
	if err != nil {
		a.serverError(w, "generate licence key pair", err)
		return
	}
	d.Generated, d.KID, d.PublicKey, d.PrivateKey = true, kid, pub, priv
	d.ConfigJSON = fmt.Sprintf("{\n  \"keys\": {\n    %q: %q\n  }\n}\n", kid, pub)
	a.View.Render(w, http.StatusOK, "admin_licence_keygen", d)
}
