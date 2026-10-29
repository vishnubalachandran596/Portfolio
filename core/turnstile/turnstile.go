// Package turnstile verifies Cloudflare Turnstile tokens on the server.
// The widget in the browser only produces a token; the check is not done
// until the token is confirmed with Cloudflare's siteverify API, because a
// bot can post any string. Tokens are single-use and live five minutes.
package turnstile

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const verifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

// Client verifies tokens. A Client with no secret is disabled: Enabled is
// false and callers skip the check, so the site works before keys are set.
type Client struct {
	secret string
	http   *http.Client
	url    string
}

func New(secret string) *Client {
	return &Client{secret: strings.TrimSpace(secret), http: &http.Client{Timeout: 6 * time.Second}, url: verifyURL}
}

func (c *Client) Enabled() bool { return c != nil && c.secret != "" }

// Verify reports whether token is a fresh, valid Turnstile token. Any
// failure to reach Cloudflare counts as not verified (fail closed).
func (c *Client) Verify(ctx context.Context, token, remoteIP string) bool {
	if !c.Enabled() {
		return true
	}
	if token == "" || len(token) > 2048 {
		return false
	}
	form := url.Values{"secret": {c.secret}, "response": {token}}
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, strings.NewReader(form.Encode()))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var out struct {
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&out); err != nil {
		return false
	}
	return out.Success
}
