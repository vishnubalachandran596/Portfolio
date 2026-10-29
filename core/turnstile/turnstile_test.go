package turnstile

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVerify(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		w.Write([]byte(map[bool]string{true: `{"success":true}`, false: `{"success":false}`}[r.Form.Get("response") == "good" && r.Form.Get("secret") == "s"]))
	}))
	defer srv.Close()
	c := New("s")
	c.url = srv.URL
	if !c.Verify(context.Background(), "good", "1.2.3.4") {
		t.Error("good token rejected")
	}
	if c.Verify(context.Background(), "bad", "") || c.Verify(context.Background(), "", "") {
		t.Error("bad/empty token accepted")
	}
	if !New("").Verify(context.Background(), "", "") {
		t.Error("disabled client must not block")
	}
	c.url = "http://127.0.0.1:1"
	if c.Verify(context.Background(), "good", "") {
		t.Error("must fail closed when Cloudflare is unreachable")
	}
}
