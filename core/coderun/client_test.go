package coderun

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The same vector is asserted in api/_runner/run.test.js, so the Go signer
// and the Node verifier can't drift apart.
const (
	vecSecret = "unit-test-secret-unit-test-secret"
	vecTS     = int64(1760000000000)
	vecBody   = `{"a":1}`
)

func TestSignVector(t *testing.T) {
	got := Sign([]byte(vecSecret), vecTS, []byte(vecBody))
	t.Logf("vector mac: %s", got)
	if got != vectorMAC {
		t.Fatalf("Sign = %s, want %s", got, vectorMAC)
	}
}

func TestRunSignsAndDecodes(t *testing.T) {
	var gotLang, gotCode string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		ts, mac, _ := strings.Cut(r.Header.Get("X-Runner-Token"), ".")
		n, _ := strconv.ParseInt(ts, 10, 64)
		if mac != Sign([]byte(vecSecret), n, body) {
			http.Error(w, "bad token", http.StatusUnauthorized)
			return
		}
		var m map[string]any
		_ = json.Unmarshal(body, &m)
		gotLang, gotCode = m["lang"].(string), m["code"].(string)
		_ = json.NewEncoder(w).Encode(Result{OK: true, Output: "hi\n", MS: 12})
	}))
	defer srv.Close()

	c := New(srv.URL, []byte(vecSecret))
	res, err := c.Run(context.Background(), "python", "print('hi')", false)
	if err != nil || !res.OK || res.Output != "hi\n" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if gotLang != "python" || gotCode != "print('hi')" {
		t.Fatalf("runner saw %q %q", gotLang, gotCode)
	}
}

func TestRunMapsStatuses(t *testing.T) {
	for status, want := range map[int]error{http.StatusTooManyRequests: ErrBusy, http.StatusBadGateway: ErrUnavailable, http.StatusUnauthorized: ErrUnavailable} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
		c := New(srv.URL, []byte(vecSecret))
		c.HTTP.Timeout = 5 * time.Second
		if _, err := c.Run(context.Background(), "go", "x", false); !errors.Is(err, want) {
			t.Errorf("status %d: err=%v, want %v", status, err, want)
		}
		srv.Close()
	}
}

func TestNewNeedsURLAndSecret(t *testing.T) {
	if New("", []byte("s")) != nil || New("http://x", nil) != nil {
		t.Fatal("a runner with no URL or no secret must be nil so the editor hides")
	}
}
