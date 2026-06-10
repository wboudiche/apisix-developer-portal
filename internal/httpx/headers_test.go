package httpx_test

import (
	"net/http"
	"net/http/httptest"
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
	if rr.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("CSP header must be set")
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
