package handlers

import (
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/authutil"
	"github.com/vishnubalachandran596/portfolio/core/middleware"
	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/notify"
)

const emailPreviewCSP = "default-src 'none'; style-src 'unsafe-inline'; img-src https:; frame-src 'none'; script-src 'none'; base-uri 'none'; form-action 'none'"

type emailTemplateListRow struct {
	Kind     string
	Label    string
	From     string
	Provider string
	Custom   bool
	Updated  string
}

type AdminEmailTemplatesData struct {
	BaseData
	Rows   []emailTemplateListRow
	Notice string
}

type AdminEmailTemplateEditData struct {
	BaseData
	Kind     string
	Label    string
	From     string
	Provider string
	Custom   bool
	Subject  string
	HTML     string
	TextBody string
	Fields   []string
	Versions []models.EmailTemplateVersion
	Notice   string
	Error    string
	Warnings []string
	Updated  string
}

func (a *App) AdminEmailTemplatesList(w http.ResponseWriter, r *http.Request) {
	custom := map[string]models.EmailTemplate{}
	if a.EmailTemplates != nil {
		rows, err := a.EmailTemplates.List(r.Context())
		if err != nil {
			a.serverError(w, "load email templates", err)
			return
		}
		for _, row := range rows {
			custom[row.Kind] = row
		}
	}
	var list []emailTemplateListRow
	for _, kind := range models.EmailKinds {
		row := emailTemplateListRow{
			Kind: kind, Label: models.EmailKindLabel(kind),
			From: notify.FromFor(kind), Provider: a.mailerFor(kind).Name(),
			Updated: "Built-in default",
		}
		if c, ok := custom[kind]; ok {
			row.Custom = true
			if !c.UpdatedAt.IsZero() {
				row.Updated = c.UpdatedAt.In(a.emailLoc()).Format("Jan 2, 2006 · 3:04 PM")
			}
		}
		list = append(list, row)
	}
	a.View.Render(w, http.StatusOK, "admin_email_templates", AdminEmailTemplatesData{
		BaseData: a.base(r, "Email templates — Admin", "", "admin"),
		Rows:     list,
		Notice:   r.URL.Query().Get("notice"),
	})
}

func (a *App) AdminEmailTemplateEdit(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	if !knownCustomerKind(kind) {
		a.NotFound(w, r)
		return
	}
	a.renderEmailTemplateEdit(w, r, kind, r.URL.Query().Get("error"), nil)
}

func (a *App) renderEmailTemplateEdit(w http.ResponseWriter, r *http.Request, kind, formErr string, warnings []string) {
	subject := models.SubjectFor(kind)
	htmlSrc, textSrc := "", ""
	custom := false
	updated := "Built-in default"
	if a.View != nil {
		htmlSrc, textSrc, _ = a.View.EmailSource(kind)
	}
	if formErr != "" && r.PostForm != nil {
		if s := r.PostFormValue("subject"); s != "" {
			subject = s
		}
		if h := r.PostFormValue("html"); h != "" {
			htmlSrc = h
		}
		if t, ok := r.PostForm["text_body"]; ok && len(t) > 0 {
			textSrc = t[0]
		}
	}
	if warn := r.URL.Query().Get("warn"); warn != "" && len(warnings) == 0 {
		warnings = []string{warn}
	}
	if a.EmailTemplates != nil {
		row, ok, err := a.EmailTemplates.Get(r.Context(), kind)
		if err != nil {
			a.serverError(w, "load email template", err)
			return
		}
		if ok {
			custom = true
			subject, htmlSrc, textSrc = row.Subject, row.HTML, row.TextBody
			if !row.UpdatedAt.IsZero() {
				updated = row.UpdatedAt.In(a.emailLoc()).Format("Jan 2, 2006 · 3:04 PM")
			}
		}
	}
	var versions []models.EmailTemplateVersion
	if a.EmailTemplates != nil {
		var err error
		versions, err = a.EmailTemplates.Versions(r.Context(), kind)
		if err != nil {
			a.serverError(w, "load email template versions", err)
			return
		}
	}
	notice := r.URL.Query().Get("notice")
	a.View.Render(w, http.StatusOK, "admin_email_template_edit", AdminEmailTemplateEditData{
		BaseData: a.base(r, models.EmailKindLabel(kind)+" — Email", "", "admin"),
		Kind:     kind, Label: models.EmailKindLabel(kind),
		From: notify.FromFor(kind), Provider: a.mailerFor(kind).Name(),
		Custom: custom, Subject: subject, HTML: htmlSrc, TextBody: textSrc,
		Fields: models.EmailFields(kind), Versions: versions,
		Notice: notice, Error: formErr, Warnings: warnings, Updated: updated,
	})
}

func (a *App) AdminEmailTemplateSave(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	if !knownCustomerKind(kind) {
		a.NotFound(w, r)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, notify.MaxTemplateBytes+8192)
	if err := r.ParseMultipartForm(notify.MaxTemplateBytes + 8192); err != nil {
		a.renderEmailTemplateEdit(w, r, kind, "That upload is too large or is not a form.", nil)
		return
	}
	subject := strings.TrimSpace(r.FormValue("subject"))
	htmlSrc := r.FormValue("html")
	if f, hdr, err := r.FormFile("file"); err == nil {
		defer f.Close()
		if hdr.Size > notify.MaxTemplateBytes {
			a.renderEmailTemplateEdit(w, r, kind, "HTML file must be at most 200 KB.", nil)
			return
		}
		raw, rerr := io.ReadAll(io.LimitReader(f, notify.MaxTemplateBytes+1))
		if rerr != nil || len(raw) > notify.MaxTemplateBytes {
			a.renderEmailTemplateEdit(w, r, kind, "HTML file must be at most 200 KB.", nil)
			return
		}
		if len(raw) > 0 {
			htmlSrc = string(raw)
		}
	}
	textBody := r.FormValue("text_body")
	if r.PostForm == nil {
		r.PostForm = url.Values{}
	}
	r.PostForm.Set("subject", subject)
	r.PostForm.Set("html", htmlSrc)
	r.PostForm.Set("text_body", textBody)
	warnings, err := notify.Validate(kind, subject, htmlSrc, textBody)
	if err != nil {
		a.renderEmailTemplateEdit(w, r, kind, err.Error(), nil)
		return
	}
	if strings.TrimSpace(textBody) == "" {
		textBody = notify.HTMLToText(htmlSrc)
	}
	if a.EmailTemplates == nil {
		a.renderEmailTemplateEdit(w, r, kind, "Templates are not available.", nil)
		return
	}
	if err := a.EmailTemplates.Save(r.Context(), models.EmailTemplate{
		Kind: kind, Subject: subject, HTML: htmlSrc, TextBody: textBody, UpdatedBy: a.adminActor(r),
	}); err != nil {
		a.serverError(w, "save email template", err)
		return
	}
	q := "?notice=saved"
	if len(warnings) > 0 {
		q += "&warn=" + url.QueryEscape(strings.Join(warnings, " "))
	}
	http.Redirect(w, r, "/admin/email-templates/"+kind+q, http.StatusSeeOther)
}

func (a *App) AdminEmailTemplateReset(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	if !knownCustomerKind(kind) {
		a.NotFound(w, r)
		return
	}
	if a.EmailTemplates != nil {
		if err := a.EmailTemplates.Delete(r.Context(), kind); err != nil {
			a.serverError(w, "reset email template", err)
			return
		}
	}
	http.Redirect(w, r, "/admin/email-templates/"+kind+"?notice=reset", http.StatusSeeOther)
}

func (a *App) AdminEmailTemplateRestore(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	if !knownCustomerKind(kind) {
		a.NotFound(w, r)
		return
	}
	id, err := strconv.ParseInt(r.FormValue("version_id"), 10, 64)
	if err != nil || a.EmailTemplates == nil {
		http.Redirect(w, r, "/admin/email-templates/"+kind+"?error="+url.QueryEscape("That version could not be restored."), http.StatusSeeOther)
		return
	}
	if err := a.EmailTemplates.Restore(r.Context(), kind, id, a.adminActor(r)); err != nil {
		http.Redirect(w, r, "/admin/email-templates/"+kind+"?error="+url.QueryEscape("That version could not be restored."), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/admin/email-templates/"+kind+"?notice=restored", http.StatusSeeOther)
}

// sendTestRecipient is always the configured admin address. The form
// cannot choose another recipient.
func (a *App) sendTestRecipient() string {
	if a.Cfg == nil {
		return ""
	}
	return strings.TrimSpace(a.Cfg.AdminEmail)
}

func (a *App) AdminEmailTemplateSendTest(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	if !knownCustomerKind(kind) {
		a.NotFound(w, r)
		return
	}
	to := a.sendTestRecipient()
	if to == "" {
		http.Redirect(w, r, "/admin/email-templates/"+kind+"?error="+url.QueryEscape("Admin address is not configured."), http.StatusSeeOther)
		return
	}
	if a.Outbox != nil {
		n, err := a.Outbox.CountSince(r.Context(), models.EmailSendTest, time.Now().Add(-time.Hour))
		if err != nil {
			a.serverError(w, "count send tests", err)
			return
		}
		if n >= 5 {
			http.Redirect(w, r, "/admin/email-templates/"+kind+"?error="+url.QueryEscape("Send test is limited to 5 per hour."), http.StatusSeeOther)
			return
		}
	}
	entity, err := authutil.RandomHex(8)
	if err != nil {
		entity = strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	job := emailJob{
		Kind:         models.EmailSendTest,
		TemplateKind: kind,
		EntityID:     entity,
		To:           to,
		Data:         notify.SampleData(kind),
	}
	if a.enqueueEmail(r.Context(), job) == "failed" {
		http.Redirect(w, r, "/admin/email-templates/"+kind+"?error="+url.QueryEscape("The test email could not be sent."), http.StatusSeeOther)
		return
	}
	switch a.sendQueued(r.Context(), job) {
	case "sent":
		http.Redirect(w, r, "/admin/email-templates/"+kind+"?notice=test-sent", http.StatusSeeOther)
	case "pending":
		http.Redirect(w, r, "/admin/email-templates/"+kind+"?error="+url.QueryEscape("That provider is off until its API key is set."), http.StatusSeeOther)
	default:
		http.Redirect(w, r, "/admin/email-templates/"+kind+"?error="+url.QueryEscape("The test email could not be sent."), http.StatusSeeOther)
	}
}

// AdminEmailTemplatePreview is a sandboxed preview. It does not send mail
// and does not read customer rows.
func (a *App) AdminEmailTemplatePreview(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	if !knownCustomerKind(kind) {
		a.NotFound(w, r)
		return
	}
	htmlBody, textBody, _, _, err := a.renderEmail(r.Context(), kind, notify.SampleData(kind))
	if err != nil {
		http.Error(w, "preview failed", http.StatusInternalServerError)
		return
	}
	theme := "light"
	if r.URL.Query().Get("theme") == "dark" {
		theme = "dark"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", emailPreviewCSP)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	bg, fg := "#f4f1ea", "#1c1917"
	if theme == "dark" {
		bg, fg = "#1c1917", "#f5f5f4"
	}
	doc := "<!DOCTYPE html><html lang=\"en\"><head><meta charset=\"utf-8\"><title>Preview</title>" +
		"<style>body{margin:0;background:" + bg + ";color:" + fg + ";font-family:Arial,sans-serif}" +
		"main{padding:16px}iframe{border:1px solid #d6d3d1;background:#fff;display:block;margin:0 0 16px}" +
		"pre{white-space:pre-wrap;font-family:ui-monospace,monospace}</style></head><body><main>" +
		"<p>Sample data only. Nothing is sent.</p>"
	for _, width := range []int{390, 768, 1100} {
		doc += `<iframe sandbox srcdoc="` + html.EscapeString(htmlBody) + `" width="` + strconv.Itoa(width) + `" height="640" title="Email preview ` + strconv.Itoa(width) + `"></iframe>`
	}
	doc += "<h2>Plain text</h2><pre>" + html.EscapeString(textBody) + "</pre></main></body></html>"
	_, _ = io.WriteString(w, doc)
}

func (a *App) AdminEmailResend(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	next := safeEmailNext(r.FormValue("next"))
	kind := r.FormValue("kind")
	entity := r.FormValue("entity_id")
	if !knownCustomerKind(kind) || strings.TrimSpace(entity) == "" {
		http.Redirect(w, r, next+"?email=failed", http.StatusSeeOther)
		return
	}
	if a.Outbox == nil {
		http.Redirect(w, r, next+"?email=failed", http.StatusSeeOther)
		return
	}
	ok, err := a.Outbox.Requeue(r.Context(), kind, entity)
	if err != nil {
		http.Redirect(w, r, next+"?email=failed", http.StatusSeeOther)
		return
	}
	if !ok {
		job, rerr := a.rebuildEmailJob(r.Context(), kind, entity)
		if rerr != nil || a.enqueueEmail(r.Context(), job) == "failed" {
			http.Redirect(w, r, next+"?email=failed", http.StatusSeeOther)
			return
		}
	}
	// Requeue only puts the row back to pending. The scheduled worker is
	// what used to send it, and a pending row is already in that state, so
	// this request sends as well. An empty provider key leaves it pending.
	job, rerr := a.rebuildEmailJob(r.Context(), kind, entity)
	if rerr != nil {
		log.Printf("email resend rebuild %s %s: %v", kind, entity, rerr)
		_ = a.Outbox.MarkFailed(r.Context(), kind, entity, clipEmailErr(rerr))
		http.Redirect(w, r, next+"?email=failed", http.StatusSeeOther)
		return
	}
	switch a.sendQueued(r.Context(), job) {
	case "sent":
		http.Redirect(w, r, next+"?email=sent", http.StatusSeeOther)
	case "pending":
		http.Redirect(w, r, next+"?email=queued", http.StatusSeeOther)
	default:
		http.Redirect(w, r, next+"?email=failed", http.StatusSeeOther)
	}
}

func safeEmailNext(next string) string {
	switch next {
	case "/admin/payments", "/admin/bookings":
		return next
	default:
		return "/admin/payments"
	}
}

func (a *App) adminActor(r *http.Request) string {
	if a.Admins == nil {
		return "admin"
	}
	id, ok := middleware.AdminIDFromContext(r.Context())
	if !ok {
		return "admin"
	}
	adm, err := a.Admins.GetByID(r.Context(), id)
	if err != nil || strings.TrimSpace(adm.Email) == "" {
		return "admin"
	}
	return adm.Email
}

func (a *App) emailLoc() *time.Location {
	if a.Timezone != nil {
		return a.Timezone
	}
	return time.UTC
}
