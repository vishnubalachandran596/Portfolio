package storage

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/config"
)

func TestCourseObject(t *testing.T) {
	got, err := CourseObject("java-sse-ultimate-guide")
	if err != nil {
		t.Fatal(err)
	}
	if got != "courses/java-sse-ultimate-guide.html" {
		t.Fatalf("got %s", got)
	}
	if _, err := CourseObject("../etc/passwd"); err == nil {
		t.Fatal("expected invalid slug to fail")
	}
}

func TestNewRequiresCredentials(t *testing.T) {
	if New(nil) != nil {
		t.Fatal("nil config")
	}
	if New(&config.Config{SupabaseURL: "https://example.supabase.co"}) != nil {
		t.Fatal("url alone is not enough")
	}
	if New(&config.Config{SupabaseURL: "https://example.supabase.co", SupabaseSecretKey: "secret"}) == nil {
		t.Fatal("secret key should configure the client")
	}
}

func TestOpenUsesSecretKeyAndStreams(t *testing.T) {
	var saw authCapture
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		saw.record(r)
		if r.URL.Path != "/storage/v1/object/course-content/courses/java-fundamentals.html" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, "<html><body>lesson</body></html>")
	}))
	defer srv.Close()

	c := New(&config.Config{SupabaseURL: srv.URL, SupabaseSecretKey: "service-secret"})
	body, err := c.Open(context.Background(), "courses/java-fundamentals.html")
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	b, _ := io.ReadAll(body)
	if string(b) != "<html><body>lesson</body></html>" {
		t.Fatalf("body %q", b)
	}
	if saw.apiKey != "service-secret" || saw.auth != "Bearer service-secret" {
		t.Fatalf("auth headers api=%q auth=%q", saw.apiKey, saw.auth)
	}
	if saw.signIn {
		t.Fatal("secret key must not sign in")
	}
}

func TestOpenSignsInThenDownloads(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		switch {
		case r.URL.Path == "/auth/v1/token":
			if r.Header.Get("apikey") != "anon-key" {
				t.Errorf("apikey %q", r.Header.Get("apikey"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"access_token":"user-jwt","expires_in":3600}`)
		case r.URL.Path == "/storage/v1/object/course-content/courses/go-portfolio-from-scratch.html":
			if r.Header.Get("Authorization") != "Bearer user-jwt" {
				t.Errorf("auth %q", r.Header.Get("Authorization"))
			}
			_, _ = io.WriteString(w, "ok")
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New(&config.Config{
		SupabaseURL:             srv.URL,
		SupabaseAnonKey:         "anon-key",
		SupabaseStorageEmail:    "course-content@example.com",
		SupabaseStoragePassword: "pw",
	})
	body, err := c.Open(context.Background(), "courses/go-portfolio-from-scratch.html")
	if err != nil {
		t.Fatal(err)
	}
	body.Close()
	body, err = c.Open(context.Background(), "courses/go-portfolio-from-scratch.html")
	if err != nil {
		t.Fatal(err)
	}
	body.Close()
	if hits != 3 {
		t.Fatalf("hits=%d, want sign-in once plus two downloads", hits)
	}
}

func TestPutSendsUpsert(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method %s", r.Method)
		}
		if r.Header.Get("x-upsert") != "true" {
			t.Errorf("upsert %q", r.Header.Get("x-upsert"))
		}
		if r.Header.Get("Content-Type") != "text/html" {
			t.Errorf("type %q", r.Header.Get("Content-Type"))
		}
		b, _ := io.ReadAll(r.Body)
		if string(b) != "<html></html>" {
			t.Errorf("body %q", b)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(&config.Config{SupabaseURL: srv.URL, SupabaseSecretKey: "s"})
	if err := c.Put(context.Background(), "courses/kafka-messaging-caching.html", "text/html", []byte("<html></html>")); err != nil {
		t.Fatal(err)
	}
}

func TestOpenRejectsEscapePath(t *testing.T) {
	c := &Client{base: "https://example.supabase.co", secretKey: "s", http: &http.Client{Timeout: time.Second}}
	if _, err := c.Open(context.Background(), "courses/../../other.html"); err == nil {
		t.Fatal("expected rejection")
	}
}

func TestReadErrRedactsToken(t *testing.T) {
	resp := &http.Response{
		Status:     "400 Bad Request",
		StatusCode: 400,
		Body:       io.NopCloser(strings.NewReader(`{"access_token":"sekret"}`)),
	}
	if got := readErr(resp); strings.Contains(got, "sekret") {
		t.Fatalf("leaked token: %s", got)
	}
}

type authCapture struct {
	apiKey string
	auth   string
	signIn bool
}

func (a *authCapture) record(r *http.Request) {
	if strings.Contains(r.URL.Path, "/auth/") {
		a.signIn = true
	}
	a.apiKey = r.Header.Get("apikey")
	a.auth = r.Header.Get("Authorization")
}
