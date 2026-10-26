package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/ai"
	"github.com/vishnubalachandran596/portfolio/core/config"
	"github.com/vishnubalachandran596/portfolio/core/middleware"
	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/notify"
	"github.com/vishnubalachandran596/portfolio/core/view"
)

type fakeMailer struct {
	name string
	err  error
	mu   sync.Mutex
	sent []notify.Message
}

func (f *fakeMailer) Name() string { return f.name }

func (f *fakeMailer) Send(_ context.Context, m notify.Message) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, m)
	if f.err != nil {
		return "", f.err
	}
	return "prov-" + m.IdempotencyKey, nil
}

func (f *fakeMailer) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sent)
}

func (f *fakeMailer) last() notify.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sent[len(f.sent)-1]
}

type spyTelegram struct {
	mu   sync.Mutex
	msgs []string
}

func (s *spyTelegram) Send(m string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.msgs = append(s.msgs, m)
}

func (s *spyTelegram) joined() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Join(s.msgs, "\n")
}

type memOutbox struct {
	mu   sync.Mutex
	rows map[string]models.EmailOutbox
	next int64
}

func newMemOutbox() *memOutbox {
	return &memOutbox{rows: map[string]models.EmailOutbox{}}
}

func outboxKey(kind, entity string) string { return kind + "\x00" + entity }

func (m *memOutbox) TryEnqueue(_ context.Context, kind, entityID, toEmail, provider string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := outboxKey(kind, entityID)
	if _, ok := m.rows[key]; ok {
		return false, nil
	}
	m.next++
	now := time.Now()
	m.rows[key] = models.EmailOutbox{
		ID: m.next, Kind: kind, EntityID: entityID, ToEmail: toEmail, Provider: provider,
		Status: models.EmailStatusPending, CreatedAt: now, UpdatedAt: now,
	}
	return true, nil
}

func (m *memOutbox) Requeue(_ context.Context, kind, entityID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := outboxKey(kind, entityID)
	row, ok := m.rows[key]
	if !ok {
		return false, nil
	}
	row.Status = models.EmailStatusPending
	row.Attempts = 0
	row.LastError = ""
	row.ProviderID = ""
	row.SentAt = nil
	row.UpdatedAt = time.Now()
	m.rows[key] = row
	return true, nil
}

func (m *memOutbox) MarkSent(_ context.Context, kind, entityID, providerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	row := m.rows[outboxKey(kind, entityID)]
	now := time.Now()
	row.Status = models.EmailStatusSent
	row.ProviderID = providerID
	row.LastError = ""
	row.UpdatedAt = now
	row.SentAt = &now
	m.rows[outboxKey(kind, entityID)] = row
	return nil
}

func (m *memOutbox) MarkFailed(_ context.Context, kind, entityID, lastError string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	row := m.rows[outboxKey(kind, entityID)]
	row.Status = models.EmailStatusFailed
	row.Attempts++
	row.LastError = lastError
	row.UpdatedAt = time.Now()
	m.rows[outboxKey(kind, entityID)] = row
	return nil
}

func (m *memOutbox) MarkDisabled(_ context.Context, kind, entityID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	row := m.rows[outboxKey(kind, entityID)]
	row.Status = models.EmailStatusDisabled
	row.LastError = ""
	row.UpdatedAt = time.Now()
	m.rows[outboxKey(kind, entityID)] = row
	return nil
}

func (m *memOutbox) ListRetryable(_ context.Context, limit int) ([]models.EmailOutbox, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if limit <= 0 {
		limit = 8
	}
	var out []models.EmailOutbox
	for _, row := range m.rows {
		if row.Kind == models.EmailSendTest || row.Attempts >= 5 {
			continue
		}
		if row.Status != models.EmailStatusFailed && row.Status != models.EmailStatusPending {
			continue
		}
		out = append(out, row)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (m *memOutbox) Get(_ context.Context, kind, entityID string) (models.EmailOutbox, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.rows[outboxKey(kind, entityID)]
	if !ok {
		return models.EmailOutbox{}, errors.New("not found")
	}
	return row, nil
}

func (m *memOutbox) ListAll(context.Context) ([]models.EmailOutbox, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]models.EmailOutbox, 0, len(m.rows))
	for _, row := range m.rows {
		out = append(out, row)
	}
	return out, nil
}

func (m *memOutbox) CountSince(_ context.Context, kind string, since time.Time) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, row := range m.rows {
		if row.Kind == kind && !row.CreatedAt.Before(since) {
			n++
		}
	}
	return n, nil
}

func (m *memOutbox) put(row models.EmailOutbox) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows[outboxKey(row.Kind, row.EntityID)] = row
}

type memTemplates struct {
	mu       sync.Mutex
	rows     map[string]models.EmailTemplate
	versions []models.EmailTemplateVersion
	next     int64
}

func newMemTemplates() *memTemplates {
	return &memTemplates{rows: map[string]models.EmailTemplate{}}
}

func (m *memTemplates) Get(_ context.Context, kind string) (models.EmailTemplate, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.rows[kind]
	return row, ok, nil
}

func (m *memTemplates) List(context.Context) ([]models.EmailTemplate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]models.EmailTemplate, 0, len(m.rows))
	for _, row := range m.rows {
		out = append(out, row)
	}
	return out, nil
}

func (m *memTemplates) Save(_ context.Context, t models.EmailTemplate) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if prev, ok := m.rows[t.Kind]; ok {
		m.next++
		m.versions = append(m.versions, models.EmailTemplateVersion{
			ID: m.next, Kind: prev.Kind, Subject: prev.Subject, HTML: prev.HTML, TextBody: prev.TextBody,
			SavedAt: time.Now(), SavedBy: prev.UpdatedBy,
		})
		var kept []models.EmailTemplateVersion
		n := 0
		for i := len(m.versions) - 1; i >= 0; i-- {
			if m.versions[i].Kind != t.Kind {
				kept = append(kept, m.versions[i])
				continue
			}
			if n < 20 {
				kept = append(kept, m.versions[i])
				n++
			}
		}
		m.versions = kept
	}
	t.UpdatedAt = time.Now()
	m.rows[t.Kind] = t
	return nil
}

func (m *memTemplates) Delete(_ context.Context, kind string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.rows, kind)
	return nil
}

func (m *memTemplates) Versions(_ context.Context, kind string) ([]models.EmailTemplateVersion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []models.EmailTemplateVersion
	for _, v := range m.versions {
		if v.Kind == kind {
			out = append(out, v)
		}
	}
	return out, nil
}

func (m *memTemplates) Restore(ctx context.Context, kind string, versionID int64, by string) error {
	m.mu.Lock()
	var found models.EmailTemplateVersion
	ok := false
	for _, v := range m.versions {
		if v.ID == versionID && v.Kind == kind {
			found = v
			ok = true
			break
		}
	}
	m.mu.Unlock()
	if !ok {
		return errors.New("missing version")
	}
	return m.Save(ctx, models.EmailTemplate{
		Kind: kind, Subject: found.Subject, HTML: found.HTML, TextBody: found.TextBody, UpdatedBy: by,
	})
}

type emailFixture struct {
	app   *App
	paid  *fakeMailer
	brevo *fakeMailer
	box   *memOutbox
	tpl   *memTemplates
	tg    *spyTelegram
}

func newEmailFixture(t *testing.T) *emailFixture {
	t.Helper()
	renderer, err := view.New()
	if err != nil {
		t.Fatal(err)
	}
	paid := &fakeMailer{name: "resend"}
	brevo := &fakeMailer{name: "brevo"}
	box := newMemOutbox()
	tpl := newMemTemplates()
	tg := &spyTelegram{}
	app := &App{
		Cfg: &config.Config{
			SiteURL:             "https://www.vishnubalachandran.com",
			SiteName:            "Vishnu Balachandran",
			AdminEmail:          "owner@example.com",
			ChatbotReindexToken: "retry-secret",
			JWTSecret:           []byte("test-secret-at-least-32-bytes-long"),
			SessionTTL:          time.Hour,
		},
		View:           renderer,
		Auth:           middleware.NewAuth([]byte("test-secret-at-least-32-bytes-long"), time.Hour, false),
		PaidMail:       paid,
		BrevoMail:      brevo,
		Outbox:         box,
		EmailTemplates: tpl,
		Telegram:       tg,
		Nvidia:         ai.New(""),
		Timezone:       time.UTC,
	}
	return &emailFixture{app: app, paid: paid, brevo: brevo, box: box, tpl: tpl, tg: tg}
}

func adminCookie(t *testing.T, auth *middleware.Auth) *http.Cookie {
	t.Helper()
	rec := httptest.NewRecorder()
	auth.IssueSession(rec, 1)
	for _, c := range rec.Result().Cookies() {
		if c.Name == middleware.SessionCookieName {
			return c
		}
	}
	t.Fatal("admin cookie missing")
	return nil
}

func userCookie(t *testing.T, auth *middleware.Auth) *http.Cookie {
	t.Helper()
	rec := httptest.NewRecorder()
	auth.IssueUserSession(rec, 9, 1)
	for _, c := range rec.Result().Cookies() {
		if c.Name == middleware.UserSessionCookieName {
			return c
		}
	}
	t.Fatal("user cookie missing")
	return nil
}

func TestEnqueueIsIdempotentAndWorkerSends(t *testing.T) {
	fx := newEmailFixture(t)
	p := models.Purchase{ID: 42, Email: "buyer@example.com", AmountCents: 149900, Currency: "INR", ClaimCode: "abc"}
	course := models.Course{Title: "Sample course", Slug: "sample"}
	fx.app.emailCoursePurchase(context.Background(), p, course)
	fx.app.emailCoursePurchase(context.Background(), p, course)
	if fx.paid.count() != 0 {
		t.Fatalf("checkout called the provider %d times", fx.paid.count())
	}
	row, err := fx.box.Get(context.Background(), models.EmailPurchaseCourse, "42")
	if err != nil || row.Status != models.EmailStatusPending {
		t.Fatalf("row %+v err %v", row, err)
	}
	fx.app.emailRebuild = func(context.Context, string, string) (emailJob, error) {
		return fx.app.courseEmailJob(p, course), nil
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/internal/email/retry", nil)
	req.Header.Set("Authorization", "Bearer retry-secret")
	fx.app.Routes().ServeHTTP(rec, req)
	if fx.paid.count() != 1 {
		t.Fatalf("worker sends %d", fx.paid.count())
	}
	msg := fx.paid.last()
	if len(msg.Bcc) != 1 || msg.Bcc[0] != "owner@example.com" {
		t.Fatalf("bcc %#v", msg.Bcc)
	}
	if msg.IdempotencyKey != "purchase_course:42" {
		t.Fatal(msg.IdempotencyKey)
	}
	if !strings.Contains(msg.HTML, "₹1,499") {
		t.Fatalf("formatted amount missing: %s", msg.HTML)
	}
	row, err = fx.box.Get(context.Background(), models.EmailPurchaseCourse, "42")
	if err != nil || row.Status != models.EmailStatusSent {
		t.Fatalf("row %+v err %v", row, err)
	}
	fx.app.Routes().ServeHTTP(httptest.NewRecorder(), req)
	if fx.paid.count() != 1 {
		t.Fatalf("second worker send %d", fx.paid.count())
	}
}

func TestZeroAmountPurchaseStillEnqueues(t *testing.T) {
	fx := newEmailFixture(t)
	fx.app.emailCoursePurchase(context.Background(), models.Purchase{
		ID: 11, Email: "buyer@example.com", AmountCents: 0, Currency: "INR",
	}, models.Course{Title: "Free course", Slug: "free"})
	fx.app.emailSoftwarePurchase(context.Background(), models.SoftwarePurchase{
		ID: 12, Email: "buyer@example.com", AmountCents: 0, Currency: "INR",
	}, models.Software{Title: "Tool", Version: "1"})
	fx.app.emailConnectRequest(context.Background(), models.ContactMessage{
		ID: 13, Name: "Ada", Email: "ada@example.com",
	})
	if fx.paid.count() != 0 || fx.brevo.count() != 0 {
		t.Fatal("enqueue called a provider")
	}
	course, err := fx.box.Get(context.Background(), models.EmailPurchaseCourse, "11")
	if err != nil || course.Status != models.EmailStatusPending || course.Provider != "resend" || course.ToEmail != "buyer@example.com" {
		t.Fatalf("course %+v %v", course, err)
	}
	software, err := fx.box.Get(context.Background(), models.EmailPurchaseSoftware, "12")
	if err != nil || software.Status != models.EmailStatusPending || software.Provider != "resend" {
		t.Fatalf("software %+v %v", software, err)
	}
	connect, err := fx.box.Get(context.Background(), models.EmailConnectRequest, "13")
	if err != nil || connect.Status != models.EmailStatusPending || connect.Provider != "brevo" || connect.ToEmail != "ada@example.com" {
		t.Fatalf("connect %+v %v", connect, err)
	}
}

func TestEmailFailureDoesNotFailCheckoutOrBooking(t *testing.T) {
	fx := newEmailFixture(t)
	fx.paid.err = errors.New("provider down")
	fx.brevo.err = errors.New("provider down")
	paid := false
	fx.app.emailCoursePurchase(context.Background(), models.Purchase{
		ID: 7, Email: "buyer@example.com", AmountCents: 100, Currency: "INR",
	}, models.Course{Title: "Course", Slug: "course"})
	paid = true
	booked := false
	start := time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC)
	fx.app.emailBooking(context.Background(), models.EmailBookingNew, models.Booking{
		ID: 3, Email: "guest@example.com", Name: "Ada", Code: "BK-123456",
	}, start, start.Add(time.Hour), "raw-token")
	booked = true
	if !paid || !booked {
		t.Fatal("caller stopped after email")
	}
	if fx.paid.count() != 0 || fx.brevo.count() != 0 {
		t.Fatal("checkout waited on a provider")
	}
	course, err := fx.box.Get(context.Background(), models.EmailPurchaseCourse, "7")
	if err != nil || course.Status != models.EmailStatusPending {
		t.Fatalf("course %+v %v", course, err)
	}
	booking, err := fx.box.Get(context.Background(), models.EmailBookingNew, "3")
	if err != nil || booking.Status != models.EmailStatusPending {
		t.Fatalf("booking %+v %v", booking, err)
	}
	fx.app.emailRebuild = func(_ context.Context, kind, entity string) (emailJob, error) {
		if kind == models.EmailBookingNew {
			return emailJob{Kind: kind, EntityID: entity, To: "guest@example.com", Data: notify.SampleData(kind)}, nil
		}
		return emailJob{Kind: kind, EntityID: entity, To: "buyer@example.com", Data: notify.SampleData(kind)}, nil
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/internal/email/retry", nil)
	req.Header.Set("Authorization", "Bearer retry-secret")
	fx.app.Routes().ServeHTTP(rec, req)
	course, err = fx.box.Get(context.Background(), models.EmailPurchaseCourse, "7")
	if err != nil || course.Status != models.EmailStatusFailed || course.Attempts != 1 {
		t.Fatalf("course %+v %v", course, err)
	}
	booking, err = fx.box.Get(context.Background(), models.EmailBookingNew, "3")
	if err != nil || booking.Status != models.EmailStatusFailed {
		t.Fatalf("booking %+v %v", booking, err)
	}
	if !strings.Contains(fx.tg.joined(), "email failed: purchase_course 7") || !strings.Contains(fx.tg.joined(), "email failed: booking_new 3") {
		t.Fatal(fx.tg.joined())
	}
}

func TestDisabledMailerLeavesFlowsAlone(t *testing.T) {
	fx := newEmailFixture(t)
	fx.app.PaidMail = notify.Disabled{}
	fx.app.BrevoMail = notify.Disabled{}
	fx.app.emailSoftwarePurchase(context.Background(), models.SoftwarePurchase{
		ID: 4, Email: "buyer@example.com", AmountCents: 500, Currency: "INR",
	}, models.Software{Title: "Tool", Version: "1"})
	fx.app.emailNewsletterConfirm(context.Background(), 8, "reader@example.com")
	if fx.paid.count() != 0 || fx.brevo.count() != 0 {
		t.Fatal("checkout contacted a provider")
	}
	fx.app.emailRebuild = func(_ context.Context, kind, entity string) (emailJob, error) {
		return emailJob{Kind: kind, EntityID: entity, To: "buyer@example.com", Data: notify.SampleData(kind)}, nil
	}
	req := httptest.NewRequest(http.MethodGet, "/internal/email/retry", nil)
	req.Header.Set("Authorization", "Bearer retry-secret")
	fx.app.Routes().ServeHTTP(httptest.NewRecorder(), req)
	row, err := fx.box.Get(context.Background(), models.EmailPurchaseSoftware, "4")
	if err != nil || row.Status != models.EmailStatusPending {
		t.Fatalf("software %+v %v", row, err)
	}
	news, err := fx.box.Get(context.Background(), models.EmailNewsletterConfirm, "8")
	if err != nil || news.Status != models.EmailStatusPending {
		t.Fatalf("news %+v %v", news, err)
	}
	if fx.tg.joined() != "" {
		t.Fatal(fx.tg.joined())
	}
}

func TestEmailRetryTokenAndAttemptCap(t *testing.T) {
	fx := newEmailFixture(t)
	h := fx.app.Routes()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/internal/email/retry", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad token %d", rec.Code)
	}

	fx.app.Cfg.ChatbotReindexToken = ""
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/internal/email/retry", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unset token %d", rec.Code)
	}
	fx.app.Cfg.ChatbotReindexToken = "retry-secret"

	old := time.Now().Add(-10 * time.Minute)
	fx.box.put(models.EmailOutbox{
		Kind: models.EmailPurchaseCourse, EntityID: "99", ToEmail: "buyer@example.com",
		Status: models.EmailStatusFailed, Attempts: 4, UpdatedAt: old, CreatedAt: old,
	})
	fx.box.put(models.EmailOutbox{
		Kind: models.EmailSendTest, EntityID: "skip", ToEmail: "owner@example.com",
		Status: models.EmailStatusFailed, Attempts: 1, UpdatedAt: old, CreatedAt: old,
	})
	fx.app.emailRebuild = func(context.Context, string, string) (emailJob, error) {
		return emailJob{
			Kind: models.EmailPurchaseCourse, EntityID: "99", To: "buyer@example.com",
			Data: notify.SampleData(models.EmailPurchaseCourse),
		}, nil
	}
	fx.paid.err = errors.New("still down")
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/internal/email/retry", nil)
	req.Header.Set("Authorization", "Bearer retry-secret")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "retried 0") {
		t.Fatalf("retry %d %s", rec.Code, rec.Body.String())
	}
	row, _ := fx.box.Get(context.Background(), models.EmailPurchaseCourse, "99")
	if row.Attempts != 5 {
		t.Fatalf("attempts %d", row.Attempts)
	}
	before := fx.paid.count()
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/internal/email/retry", nil)
	req.Header.Set("Authorization", "Bearer retry-secret")
	h.ServeHTTP(rec, req)
	if fx.paid.count() != before {
		t.Fatalf("attempt cap did not hold: %d -> %d", before, fx.paid.count())
	}
}

func TestAdminEmailPreviewAndAuth(t *testing.T) {
	fx := newEmailFixture(t)
	h := fx.app.Routes()
	for _, name := range []string{"anonymous", "user"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/admin/email-templates/purchase_course/preview", nil)
		if name == "user" {
			req.AddCookie(userCookie(t, fx.app.Auth))
		}
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/admin/login") {
			t.Fatalf("%s: %d %s", name, rec.Code, rec.Header().Get("Location"))
		}
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/email-templates/purchase_course/preview", nil)
	req.AddCookie(adminCookie(t, fx.app.Auth))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Security-Policy") != emailPreviewCSP {
		t.Fatal(rec.Header().Get("Content-Security-Policy"))
	}
	body := rec.Body.String()
	if !strings.Contains(body, `sandbox srcdoc=`) || strings.Contains(body, "allow-scripts") {
		t.Fatal("preview iframe is not a script-less sandbox")
	}
	if !strings.Contains(body, "390") || !strings.Contains(body, "768") || !strings.Contains(body, "1100") {
		t.Fatal("preview widths missing")
	}
}

func TestSendTestGoesToAdminOnly(t *testing.T) {
	fx := newEmailFixture(t)
	h := fx.app.Routes()
	form := url.Values{"to": {"evil@example.com"}}
	req := httptest.NewRequest(http.MethodPost, "/admin/email-templates/booking_new/send-test", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(adminCookie(t, fx.app.Auth))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	if fx.brevo.count() != 1 {
		t.Fatalf("brevo sends %d resend %d", fx.brevo.count(), fx.paid.count())
	}
	msg := fx.brevo.last()
	if msg.To != "owner@example.com" || strings.Contains(msg.To, "evil") {
		t.Fatal(msg.To)
	}
	if len(msg.Bcc) != 0 {
		t.Fatalf("admin recipient was also bcc: %#v", msg.Bcc)
	}
	if fx.paid.count() != 0 {
		t.Fatal("booking test used Resend")
	}
}

func TestAdminResendSendsPendingAndFailed(t *testing.T) {
	fx := newEmailFixture(t)
	fx.app.emailRebuild = func(_ context.Context, kind, entity string) (emailJob, error) {
		return emailJob{
			Kind: kind, EntityID: entity, To: "buyer@example.com",
			Data: notify.SampleData(kind),
		}, nil
	}
	now := time.Now()
	fx.box.put(models.EmailOutbox{
		Kind: models.EmailPurchaseCourse, EntityID: "15", ToEmail: "buyer@example.com",
		Status: models.EmailStatusFailed, Attempts: 5, CreatedAt: now, UpdatedAt: now,
	})
	fx.box.put(models.EmailOutbox{
		Kind: models.EmailBookingNew, EntityID: "4", ToEmail: "guest@example.com",
		Status: models.EmailStatusPending, Attempts: 0, CreatedAt: now, UpdatedAt: now,
	})
	post := func(kind, entity, next string) *httptest.ResponseRecorder {
		t.Helper()
		form := url.Values{"kind": {kind}, "entity_id": {entity}, "next": {next}}
		req := httptest.NewRequest(http.MethodPost, "/admin/email/resend", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(adminCookie(t, fx.app.Auth))
		rec := httptest.NewRecorder()
		fx.app.Routes().ServeHTTP(rec, req)
		return rec
	}
	rec := post(models.EmailPurchaseCourse, "15", "/admin/payments")
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "email=sent") {
		t.Fatalf("resend %d %s", rec.Code, rec.Header().Get("Location"))
	}
	if fx.paid.count() != 1 {
		t.Fatalf("course resend sends %d", fx.paid.count())
	}
	row, err := fx.box.Get(context.Background(), models.EmailPurchaseCourse, "15")
	if err != nil || row.Status != models.EmailStatusSent || row.Attempts != 0 {
		t.Fatalf("sent %+v %v", row, err)
	}
	rec = post(models.EmailBookingNew, "4", "/admin/bookings")
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "email=sent") {
		t.Fatalf("pending resend %d %s", rec.Code, rec.Header().Get("Location"))
	}
	if fx.brevo.count() != 1 {
		t.Fatalf("booking resend sends %d", fx.brevo.count())
	}
	row, err = fx.box.Get(context.Background(), models.EmailBookingNew, "4")
	if err != nil || row.Status != models.EmailStatusSent {
		t.Fatalf("booking %+v %v", row, err)
	}

	fx.app.PaidMail = notify.Disabled{}
	fx.box.put(models.EmailOutbox{
		Kind: models.EmailPurchaseSoftware, EntityID: "9", ToEmail: "buyer@example.com",
		Status: models.EmailStatusPending, CreatedAt: now, UpdatedAt: now,
	})
	rec = post(models.EmailPurchaseSoftware, "9", "/admin/payments")
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "email=queued") {
		t.Fatalf("disabled %d %s", rec.Code, rec.Header().Get("Location"))
	}
	row, err = fx.box.Get(context.Background(), models.EmailPurchaseSoftware, "9")
	if err != nil || row.Status != models.EmailStatusPending {
		t.Fatalf("stays pending %+v %v", row, err)
	}
}

func TestCustomTemplateFallbackAndVersions(t *testing.T) {
	fx := newEmailFixture(t)
	custom := `<p>CUSTOM {{.BuyerName}}</p><p><a href="{{.ReceiptURL}}">r</a></p>`
	if err := fx.tpl.Save(context.Background(), models.EmailTemplate{
		Kind: models.EmailPurchaseCourse, Subject: "Custom subject", HTML: custom, TextBody: "CUSTOM {{.BuyerName}} {{.ReceiptURL}}", UpdatedBy: "admin",
	}); err != nil {
		t.Fatal(err)
	}
	subject, html, _, fellBack, err := fx.app.renderEmail(context.Background(), models.EmailPurchaseCourse, notify.SampleData(models.EmailPurchaseCourse))
	if err != nil || fellBack || subject != "Custom subject" || !strings.Contains(html, "CUSTOM") {
		t.Fatalf("custom subject %q html %q fell %v err %v", subject, html, fellBack, err)
	}
	fx.tpl.rows[models.EmailPurchaseCourse] = models.EmailTemplate{
		Kind: models.EmailPurchaseCourse, Subject: "Broken", HTML: `<script>{{.ReceiptURL}}</script>`,
	}
	job := emailJob{
		Kind: models.EmailPurchaseCourse, EntityID: "broken", To: "buyer@example.com",
		Data: notify.SampleData(models.EmailPurchaseCourse),
	}
	if fx.app.enqueueEmail(context.Background(), job) != "queued" {
		t.Fatal("enqueue")
	}
	if fx.paid.count() != 0 {
		t.Fatal("enqueue called the provider")
	}
	status := fx.app.sendQueued(context.Background(), job)
	if status != "sent" {
		t.Fatalf("broken template status %s", status)
	}
	if strings.Contains(fx.paid.last().HTML, "<script") || !strings.Contains(fx.paid.last().HTML, "This confirms your purchase") {
		t.Fatalf("fallback html %s", fx.paid.last().HTML)
	}
	if !strings.Contains(fx.tg.joined(), "email template fell back") {
		t.Fatal(fx.tg.joined())
	}
	if err := fx.tpl.Delete(context.Background(), models.EmailPurchaseCourse); err != nil {
		t.Fatal(err)
	}
	subject, html, _, fellBack, err = fx.app.renderEmail(context.Background(), models.EmailPurchaseCourse, notify.SampleData(models.EmailPurchaseCourse))
	if err != nil || fellBack || subject != models.SubjectFor(models.EmailPurchaseCourse) || strings.Contains(html, "CUSTOM") {
		t.Fatalf("default subject %q err %v", subject, err)
	}

	first := `<p>V1 {{.BuyerName}}</p><a href="{{.ReceiptURL}}">r</a>`
	second := `<p>V2 {{.BuyerName}}</p><a href="{{.ReceiptURL}}">r</a>`
	if err := fx.tpl.Save(context.Background(), models.EmailTemplate{Kind: models.EmailPurchaseCourse, Subject: "v1", HTML: first, UpdatedBy: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := fx.tpl.Save(context.Background(), models.EmailTemplate{Kind: models.EmailPurchaseCourse, Subject: "v2", HTML: second, UpdatedBy: "b"}); err != nil {
		t.Fatal(err)
	}
	versions, err := fx.tpl.Versions(context.Background(), models.EmailPurchaseCourse)
	if err != nil || len(versions) != 1 || versions[0].Subject != "v1" {
		t.Fatalf("versions %+v %v", versions, err)
	}
	for i := 0; i < 25; i++ {
		if err := fx.tpl.Save(context.Background(), models.EmailTemplate{
			Kind: models.EmailPurchaseCourse, Subject: "loop", HTML: first, UpdatedBy: "a",
		}); err != nil {
			t.Fatal(err)
		}
	}
	versions, err = fx.tpl.Versions(context.Background(), models.EmailPurchaseCourse)
	if err != nil || len(versions) > 20 {
		t.Fatalf("kept %d", len(versions))
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/email-templates/purchase_course/restore", strings.NewReader(fmt.Sprintf("version_id=%d", versions[0].ID)))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("kind", "purchase_course")
	rec := httptest.NewRecorder()
	fx.app.AdminEmailTemplateRestore(rec, req)
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "notice=restored") {
		t.Fatalf("restore %d %s", rec.Code, rec.Header().Get("Location"))
	}
}
