package handlers

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/notify"
)

const emailSendTimeout = 4 * time.Second

// emailOutboxStore is the delivery log. *repo.EmailOutboxRepo implements it.
type emailOutboxStore interface {
	TryEnqueue(ctx context.Context, kind, entityID, toEmail, provider string) (bool, error)
	Requeue(ctx context.Context, kind, entityID string) (bool, error)
	MarkSent(ctx context.Context, kind, entityID, providerID string) error
	MarkFailed(ctx context.Context, kind, entityID, lastError string) error
	MarkDisabled(ctx context.Context, kind, entityID string) error
	ListRetryable(ctx context.Context, limit int) ([]models.EmailOutbox, error)
	Get(ctx context.Context, kind, entityID string) (models.EmailOutbox, error)
	ListAll(ctx context.Context) ([]models.EmailOutbox, error)
	CountSince(ctx context.Context, kind string, since time.Time) (int, error)
}

// emailTemplateStore is the admin override table. A missing row means the
// built-in template is used.
type emailTemplateStore interface {
	Get(ctx context.Context, kind string) (models.EmailTemplate, bool, error)
	List(ctx context.Context) ([]models.EmailTemplate, error)
	Save(ctx context.Context, t models.EmailTemplate) error
	Delete(ctx context.Context, kind string) error
	Versions(ctx context.Context, kind string) ([]models.EmailTemplateVersion, error)
	Restore(ctx context.Context, kind string, versionID int64, by string) error
}

// emailJob is one send. TemplateKind overrides Kind when the outbox row
// is a send-test of some other template. The body is not stored.
type emailJob struct {
	Kind         string
	TemplateKind string
	EntityID     string
	To           string
	Data         models.EmailData
}

func (j emailJob) templateKind() string {
	if j.TemplateKind != "" {
		return j.TemplateKind
	}
	return j.Kind
}

// dispatchEmail records the outbox row and sends it before the handler
// returns. A goroutine after the response does not run: the Go runtime
// on Vercel freezes the process when the handler returns. The caller
// still succeeds when the provider fails. A duplicate (kind, entity_id)
// does not send again.
func (a *App) dispatchEmail(parent context.Context, job emailJob) string {
	switch a.enqueueEmail(parent, job) {
	case "queued":
		return a.sendQueued(parent, job)
	case "duplicate":
		return "duplicate"
	case "failed":
		return "failed"
	default:
		return "skipped"
	}
}

// enqueueEmail records one pending outbox row after the caller has
// committed. It does not call a provider. A duplicate (kind, entity_id)
// leaves the existing row alone. Enqueue trouble is logged and does not
// fail the purchase, booking, connect request, or subscription.
func (a *App) enqueueEmail(parent context.Context, job emailJob) string {
	if parent == nil {
		parent = context.Background()
	}
	ctx := context.WithoutCancel(parent)
	to := strings.TrimSpace(job.To)
	if to == "" || !strings.Contains(to, "@") {
		log.Printf("email skipped: %s %s missing recipient", job.Kind, job.EntityID)
		return "skipped"
	}
	if a.Outbox == nil {
		log.Printf("email skipped: %s %s no outbox", job.Kind, job.EntityID)
		return "skipped"
	}
	inserted, err := a.Outbox.TryEnqueue(ctx, job.Kind, job.EntityID, to, emailProviderName(job.templateKind()))
	if err != nil {
		log.Printf("email enqueue %s %s: %v", job.Kind, job.EntityID, err)
		a.alert("email failed: " + job.Kind + " " + job.EntityID)
		return "failed"
	}
	if !inserted {
		return "duplicate"
	}
	return "queued"
}

// sendQueued delivers one outbox row. The customer request calls it
// immediately after enqueue. The daily cron and the admin resend call
// it again for anything still pending or failed. An empty provider key
// leaves the row pending.
func (a *App) sendQueued(parent context.Context, job emailJob) string {
	if parent == nil {
		parent = context.Background()
	}
	ctx := context.WithoutCancel(parent)
	kind := job.templateKind()
	job.Data.SupportReplyTo = notify.ReplyToFor(kind)
	to := strings.TrimSpace(job.To)
	if to == "" || !strings.Contains(to, "@") || a.Outbox == nil {
		return "skipped"
	}
	subject, htmlBody, textBody, fellBack, rerr := a.renderEmail(ctx, kind, job.Data)
	if rerr != nil {
		log.Printf("email render %s %s: %v", job.Kind, job.EntityID, rerr)
		_ = a.Outbox.MarkFailed(ctx, job.Kind, job.EntityID, clipEmailErr(rerr))
		a.alert("email failed: " + job.Kind + " " + job.EntityID)
		return "failed"
	}
	if fellBack {
		a.alert("email template fell back: " + kind + " " + job.EntityID)
	}
	admin := ""
	if a.Cfg != nil {
		admin = a.Cfg.AdminEmail
	}
	msg := notify.WithAdminBCC(notify.Message{
		From:           notify.FromFor(kind),
		To:             to,
		Subject:        subject,
		HTML:           htmlBody,
		Text:           textBody,
		ReplyTo:        notify.ReplyToFor(kind),
		IdempotencyKey: job.Kind + ":" + job.EntityID,
	}, admin)

	sendCtx, cancel := context.WithTimeout(ctx, emailSendTimeout)
	defer cancel()
	providerID, serr := a.mailerFor(kind).Send(sendCtx, msg)
	if errors.Is(serr, notify.ErrDisabled) {
		log.Printf("email waiting: %s %s provider disabled", job.Kind, job.EntityID)
		return "pending"
	}
	if serr != nil {
		log.Printf("email failed: %s %s: %s", job.Kind, job.EntityID, clipEmailErr(serr))
		_ = a.Outbox.MarkFailed(ctx, job.Kind, job.EntityID, clipEmailErr(serr))
		a.alert("email failed: " + job.Kind + " " + job.EntityID)
		return "failed"
	}
	if err := a.Outbox.MarkSent(ctx, job.Kind, job.EntityID, providerID); err != nil {
		log.Printf("email mark sent %s %s: %v", job.Kind, job.EntityID, err)
	}
	a.alert(emailTelegramCopy(msg, job.Kind, emailProviderName(kind), textBody, job.Data))
	return "sent"
}

// telegramMaxRunes keeps a copy under Telegram's 4096-character message cap.
const telegramMaxRunes = 3900

// emailTelegramCopy is the owner's Telegram copy of one sent email: the
// From, To, Bcc, Reply-To and Subject lines, then the plain-text body. Links
// and codes that grant access (course access link, private booking manage
// link, guest claim code, unsubscribe link) are replaced with a note, because
// Telegram chats are not end-to-end encrypted; the full email is in the
// admin BCC copy.
func emailTelegramCopy(msg notify.Message, kind, provider, textBody string, d models.EmailData) string {
	body := textBody
	for _, secret := range []string{d.AccessURL, d.ManageURL, d.ClaimCode, d.UnsubscribeURL} {
		if strings.TrimSpace(secret) != "" {
			body = strings.ReplaceAll(body, secret, "[hidden here, see the BCC copy]")
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "📧 Email sent: %s (%s)\n", kind, provider)
	fmt.Fprintf(&b, "From: %s\nTo: %s\n", msg.From, msg.To)
	if len(msg.Bcc) > 0 {
		fmt.Fprintf(&b, "Bcc: %s\n", strings.Join(msg.Bcc, ", "))
	}
	if msg.ReplyTo != "" {
		fmt.Fprintf(&b, "Reply-To: %s\n", msg.ReplyTo)
	}
	fmt.Fprintf(&b, "Subject: %s\n\n%s", msg.Subject, strings.TrimSpace(body))
	out := []rune(b.String())
	if len(out) > telegramMaxRunes {
		return string(out[:telegramMaxRunes]) + "\n[clipped]"
	}
	return string(out)
}

func emailProviderName(kind string) string {
	if models.EmailUsesResend(kind) {
		return "resend"
	}
	return "brevo"
}

func (a *App) mailerFor(kind string) notify.Mailer {
	var m notify.Mailer
	if models.EmailUsesResend(kind) {
		m = a.PaidMail
	} else {
		m = a.BrevoMail
	}
	if m == nil {
		return notify.Disabled{}
	}
	return m
}

func (a *App) renderEmail(ctx context.Context, kind string, data models.EmailData) (subject, htmlBody, textBody string, fellBack bool, err error) {
	subject = models.SubjectFor(kind)
	if a.EmailTemplates != nil {
		row, ok, gerr := a.EmailTemplates.Get(ctx, kind)
		if gerr != nil {
			log.Printf("email template lookup %s: %v", kind, gerr)
		} else if ok {
			if _, verr := notify.Validate(kind, row.Subject, row.HTML, row.TextBody); verr != nil {
				log.Printf("email template fallback %s: %v", kind, verr)
				fellBack = true
			} else {
				h, t, eerr := notify.Execute(kind, row.HTML, row.TextBody, data)
				if eerr != nil {
					log.Printf("email template fallback %s: %v", kind, eerr)
					fellBack = true
				} else {
					return row.Subject, h, t, false, nil
				}
			}
		}
	}
	if a.View != nil {
		h, t, rerr := a.View.RenderEmail(kind, data)
		if rerr != nil {
			return "", "", "", fellBack, rerr
		}
		return subject, h, t, fellBack, nil
	}
	plain := data.ItemTitle
	if plain == "" {
		plain = data.BookingCode
	}
	if plain == "" {
		plain = subject
	}
	return subject, "<p>" + template.HTMLEscapeString(plain) + "</p>", plain, fellBack, nil
}

func (a *App) alert(message string) {
	if a != nil && a.Telegram != nil {
		a.Telegram.Send(message)
	}
}

func clipEmailErr(err error) string {
	if err == nil {
		return ""
	}
	s := strings.Join(strings.Fields(err.Error()), " ")
	if len(s) > 400 {
		s = s[:400]
	}
	return s
}

func knownCustomerKind(kind string) bool {
	for _, k := range models.EmailKinds {
		if k == kind {
			return true
		}
	}
	return false
}

// EmailRetry is the daily fallback. Vercel Hobby cron calls it once a
// day. It sends pending rows and retries failed rows. The bearer is
// CHATBOT_REINDEX_TOKEN, which Vercel sends when CRON_SECRET is that
// same value. It is 404 when that token is unset. The send stays in
// this request; there is no background goroutine.
func (a *App) EmailRetry(w http.ResponseWriter, r *http.Request) {
	token := ""
	if a.Cfg != nil {
		token = a.Cfg.ChatbotReindexToken
	}
	if token == "" {
		http.NotFound(w, r)
		return
	}
	got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if a.Outbox == nil {
		http.Error(w, "outbox unavailable", http.StatusInternalServerError)
		return
	}
	rows, err := a.Outbox.ListRetryable(r.Context(), 8)
	if err != nil {
		log.Printf("email retry list: %v", err)
		http.Error(w, "retry failed", http.StatusInternalServerError)
		return
	}
	n := 0
	for _, row := range rows {
		job, rerr := a.rebuildEmailJob(r.Context(), row.Kind, row.EntityID)
		if rerr != nil {
			log.Printf("email retry rebuild %s %s: %v", row.Kind, row.EntityID, rerr)
			_ = a.Outbox.MarkFailed(r.Context(), row.Kind, row.EntityID, clipEmailErr(rerr))
			continue
		}
		if a.sendQueued(r.Context(), job) == "sent" {
			n++
		}
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "retried %d\n", n)
}

func (a *App) rebuildEmailJob(ctx context.Context, kind, entity string) (emailJob, error) {
	if a.emailRebuild != nil {
		return a.emailRebuild(ctx, kind, entity)
	}
	return a.rebuildEmail(ctx, kind, entity)
}
