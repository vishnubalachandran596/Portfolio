package middleware

import "net/http"

// SecurityHeaders sets a baseline of defensive HTTP headers on every
// response. The CSP allows 'unsafe-inline' for scripts/styles because the
// site's own templates and embedded course-content pages rely on inline
// <script>/<style> blocks — it still blocks any *other* origin from
// injecting script or style, which is the actual XSS-mitigation value, and
// explicitly allowlists the external origins the site really loads from
// (Google Fonts, Razorpay's Checkout widget).
func SecurityHeaders(next http.Handler) http.Handler {
	const csp = "default-src 'self'; " +
		"script-src 'self' 'unsafe-inline' https://checkout.razorpay.com; " +
		"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; " +
		"font-src 'self' https://fonts.gstatic.com; " +
		"img-src 'self' data: https:; " +
		"connect-src 'self' https://*.razorpay.com; " +
		"frame-src https://*.razorpay.com; " +
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
