package httpx_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"apisix-portal/internal/httpx"
)

func TestSecurityHeadersSet(t *testing.T) {
	h := httpx.SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	want := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	}
	for k, v := range want {
		if got := rr.Header().Get(k); got != v {
			t.Fatalf("%s: got %q want %q", k, got, v)
		}
	}
	csp := rr.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Fatal("CSP header must be set")
	}
	for _, directive := range []string{"default-src 'self'", "fonts.googleapis.com", "font-src", "frame-ancestors 'none'"} {
		if !strings.Contains(csp, directive) {
			t.Errorf("CSP missing %q, got: %s", directive, csp)
		}
	}
}

func TestAPIResponsesAreNoStore(t *testing.T) {
	h := httpx.SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/applications/1/credentials", nil))
	if got := rr.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("/api/ responses must be no-store (they can carry live keys), got %q", got)
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rr.Header().Get("Cache-Control") != "" {
		t.Fatal("non-API paths must not be forced no-store")
	}
}

func TestMaxBodyBytesCapsRequestBody(t *testing.T) {
	var readErr error
	h := httpx.MaxBodyBytes(16)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, readErr = io.ReadAll(r.Body)
		w.WriteHeader(200)
	}))

	// Within the limit: reads cleanly.
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/x", strings.NewReader("0123456789")))
	if readErr != nil {
		t.Fatalf("body under the limit must read without error, got %v", readErr)
	}

	// Over the limit: the handler's read fails (oversized body rejected).
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/x", strings.NewReader(strings.Repeat("A", 1000))))
	if readErr == nil {
		t.Fatal("a body over the cap must surface a read error to the handler")
	}
}
