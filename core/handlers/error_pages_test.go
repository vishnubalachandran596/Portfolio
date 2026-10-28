package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestErrorPagesReplacesPlain500(t *testing.T) {
	fx := newEmailFixture(t)
	h := fx.app.errorPages(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom internal detail", http.StatusInternalServerError)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/courses", nil))
	if rec.Code != 500 || !strings.Contains(rec.Body.String(), "Something went wrong") {
		t.Fatalf("code %d body %q", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "boom internal detail") {
		t.Fatal("internal detail leaked")
	}
}

func TestErrorPagesRecoversPanicAndAlerts(t *testing.T) {
	fx := newEmailFixture(t)
	h := fx.app.errorPages(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("kaboom") }))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rec.Code != 500 || !strings.Contains(rec.Body.String(), "Something went wrong") {
		t.Fatalf("code %d", rec.Code)
	}
	if !strings.Contains(fx.tg.joined(), "Crash") {
		t.Fatalf("no alert: %q", fx.tg.joined())
	}
}

func TestErrorPagesLeavesInternalAlone(t *testing.T) {
	fx := newEmailFixture(t)
	h := fx.app.errorPages(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "plain", http.StatusInternalServerError)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/internal/email/retry", nil))
	if strings.TrimSpace(rec.Body.String()) != "plain" {
		t.Fatalf("body %q", rec.Body.String())
	}
}
