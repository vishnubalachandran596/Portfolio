// Package coderun calls the server-side code runner (api/run.js), which
// loads a JDoodle embed in a headless browser, puts the learner's code in,
// runs it and returns the output. The browser never talks to the runner or
// to JDoodle, so no embed key ever reaches a visitor.
package coderun

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

var (
	// ErrBusy means the runner is at capacity; the learner can retry.
	ErrBusy = errors.New("coderun: runner busy")
	// ErrUnavailable covers a runner that is down, misconfigured or timed out.
	ErrUnavailable = errors.New("coderun: runner unavailable")
)

// Result is what a run printed.
type Result struct {
	OK       bool   `json:"ok"`
	TimedOut bool   `json:"timedOut"`
	Output   string `json:"output"`
	MS       int    `json:"ms"`
	// Debug-only (admins, ?debug=1): the raw embed messages, the embed's
	// visible text and a base64 JPEG of the page.
	Trace   json.RawMessage `json:"trace,omitempty"`
	DomText *string         `json:"domText,omitempty"`
	Shot    string          `json:"shot,omitempty"`
}

// Client signs and sends run requests.
type Client struct {
	URL    string
	Secret []byte
	HTTP   *http.Client
}

// New returns nil when there is nothing to call (no URL or no secret), so
// the reader simply hides its Run controls.
func New(url string, secret []byte) *Client {
	if url == "" || len(secret) == 0 {
		return nil
	}
	return &Client{URL: url, Secret: secret, HTTP: &http.Client{Timeout: 58 * time.Second}}
}

// Sign returns the hex HMAC the runner checks (api/_runner/verify.js): the
// key is the shared secret, the message "runner:<unix-ms>:<sha256(body)>".
func Sign(secret []byte, tsMillis int64, body []byte) string {
	digest := sha256.Sum256(body)
	mac := hmac.New(sha256.New, secret)
	fmt.Fprintf(mac, "runner:%d:%s", tsMillis, hex.EncodeToString(digest[:]))
	return hex.EncodeToString(mac.Sum(nil))
}

// Run executes code in the given language. debug asks the runner to include
// the raw embed messages (admins only; it exists to inspect the embed).
func (c *Client) Run(ctx context.Context, lang, code string, debug bool) (Result, error) {
	payload := map[string]any{"lang": lang, "code": code}
	if debug {
		payload["debug"] = true
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Result{}, err
	}
	ts := time.Now().UnixMilli()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Runner-Token", strconv.FormatInt(ts, 10)+"."+Sign(c.Secret, ts, body))

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return Result{}, ErrBusy
	case resp.StatusCode != http.StatusOK:
		return Result{}, fmt.Errorf("%w: status %d", ErrUnavailable, resp.StatusCode)
	}
	var res Result
	if err := json.Unmarshal(raw, &res); err != nil {
		return Result{}, fmt.Errorf("%w: bad reply", ErrUnavailable)
	}
	return res, nil
}
