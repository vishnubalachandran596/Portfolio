package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdminInboxRequiresAdmin(t *testing.T) {
	fx := newEmailFixture(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/inbox", nil)
	fx.app.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "/admin/login") {
		t.Fatalf("unauthenticated %d %s", rec.Code, rec.Header().Get("Location"))
	}
	req = httptest.NewRequest(http.MethodPost, "/admin/inbox", strings.NewReader("email_address=a@b.c"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	fx.app.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("post %d", rec.Code)
	}
}

func TestAdminInboxDisabledDoesNotCallResend(t *testing.T) {
	fx := newEmailFixture(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/inbox", nil)
	req.AddCookie(adminCookie(t, fx.app.Auth))
	fx.app.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "API key") {
		t.Fatalf("body %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `sandbox`) && strings.Contains(rec.Body.String(), "iframe") {
		t.Fatal("list page should not render a message iframe")
	}
}
