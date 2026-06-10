package httpx

import (
	"net/http"
	"strings"
)

// SecurityHeaders sets baseline security response headers on every response.
// CSP is tuned for the SPA: same-origin by default, the app's Google Fonts
// origins allowed, framing denied. style-src keeps 'unsafe-inline' because the
// React app uses inline styles — a deliberate CSP weakening. HSTS is
// intentionally omitted here (added at the TLS-terminating proxy in
// production). /api/ responses are marked no-store: the credentials endpoint
// returns the live API key over GET and must never land in an HTTP cache.
func SecurityHeaders(next http.Handler) http.Handler {
	const csp = "default-src 'self'; " +
		"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; " +
		"font-src 'self' https://fonts.gstatic.com; " +
		"img-src 'self' data:; " +
		"connect-src 'self'; " +
		"frame-ancestors 'none'; base-uri 'self'"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", csp)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}
