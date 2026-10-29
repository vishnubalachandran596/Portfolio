package middleware

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/http"
)

type nonceKey struct{}

// Nonce returns the per-request script nonce SecurityHeaders generated, or ""
// outside a request that passed through it. Inline <script> tags carry it as
// nonce="..." so only markup the server wrote can run; the policy has no
// 'unsafe-inline' for scripts.
func Nonce(r *http.Request) string {
	n, _ := r.Context().Value(nonceKey{}).(string)
	return n
}

func newNonce() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return base64.RawStdEncoding.EncodeToString(b)
}

// SecurityHeaders sets a baseline of defensive HTTP headers on every
// response. script-src has no 'unsafe-inline': a script runs only if it is
// loaded from our own origin (or Razorpay, JDoodle or Cloudflare Turnstile),
// or is an inline block carrying this request's random nonce, which only the
// server's own templates can add. Injected markup cannot run script, and
// inline on* attributes are not used (see web/static/js/actions.js).
// style-src still allows 'unsafe-inline' because pages use style attributes;
// that cannot run code. Course lessons are a different document: they are
// served inside a sandboxed iframe under their own policy (lessonCSP in
// core/handlers/learn.go), which is the only place inline script and eval are
// allowed. Fonts are self-hosted, and the one remaining external script
// origins are Razorpay Checkout, JDoodle's embed and Turnstile.
func SecurityHeaders(next http.Handler) http.Handler {
	const cspPrefix = "default-src 'self'; script-src 'self' 'nonce-"
	const cspRest = "' https://checkout.razorpay.com https://cdn.razorpay.com https://www.jdoodle.com https://challenges.cloudflare.com; " +
		"style-src 'self' 'unsafe-inline'; " +
		"font-src 'self'; " +
		"img-src 'self' data: https:; " +
		"connect-src 'self' https://*.razorpay.com https://challenges.cloudflare.com; " +
		"frame-src 'self' https://*.razorpay.com https://www.jdoodle.com https://challenges.cloudflare.com; " +
		"object-src 'none'; " +
		"base-uri 'self'; " +
		"frame-ancestors 'none'"

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(self)")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		nonce := newNonce()
		h.Set("Content-Security-Policy", cspPrefix+nonce+cspRest)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), nonceKey{}, nonce)))
	})
}
