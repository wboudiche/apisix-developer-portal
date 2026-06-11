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

// MaxBodyBytes caps every request body at n bytes. A handler reading past the
// limit gets an error from the decoder (handled as a 400), so an oversized POST
// can't force unbounded allocation. GET/HEAD bodies are absent, so this is a
// no-op for them.
func MaxBodyBytes(n int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, n)
			}
			next.ServeHTTP(w, r)
		})
	}
}
