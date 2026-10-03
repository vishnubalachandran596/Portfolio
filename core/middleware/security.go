package middleware

import "net/http"

// SecurityHeaders sets a baseline of defensive HTTP headers on every
// response. The CSP allows 'unsafe-inline' for scripts/styles because the
// site's own templates and injected lesson/article scripts rely on inline
// <script>/<style> blocks — it still blocks any *other* origin from
// injecting script or style, which is the actual XSS-mitigation value.
// Fonts are self-hosted (web/static/fonts/, see style.css) rather than
// loaded from Google Fonts, so font-src/style-src need no external
// allowance for them; Razorpay's Checkout widget is the one remaining
// external origin the site actually loads from.
func SecurityHeaders(next http.Handler) http.Handler {
	const csp = "default-src 'self'; " +
		"script-src 'self' 'unsafe-inline' https://checkout.razorpay.com; " +
		"style-src 'self' 'unsafe-inline'; " +
		"font-src 'self'; " +
		"img-src 'self' data: https:; " +
		"connect-src 'self' https://*.razorpay.com; " +
		"frame-src 'self' https://*.razorpay.com; " +
		"object-src 'none'; " +
		"base-uri 'self'; " +
		"frame-ancestors 'none'"

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Content-Security-Policy", csp)
		next.ServeHTTP(w, r)
	})
}
