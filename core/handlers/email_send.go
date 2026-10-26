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

// enqueueEmail records one pending outbox row after the caller has
// committed. It does not call a provider. A duplicate (kind, entity_id)
// leaves the existing row alone. Enqueue trouble is logged and does not
// fail the purchase, booking, or subscription.
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

// sendQueued delivers one outbox row. The customer request never calls
// this. The retry worker does, and the admin send-test does. An empty
// provider key leaves the row pending so a later worker run can send it.
func (a *App) sendQueued(parent context.Context, job emailJob) string {
	if parent == nil {
		parent = context.Background()
	}
	ctx := context.WithoutCancel(parent)
	kind := job.templateKind()
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
		ReplyTo:        notify.ReplyTo,
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
	return "sent"
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

// EmailRetry is the queue worker. It sends pending rows and retries
// failed rows. The bearer is the existing CHATBOT_REINDEX_TOKEN. It is
// 404 when that token is unset. The send stays in this request; there
// is no background goroutine.
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
