package admin

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchSpec_RejectsNonHTTPScheme(t *testing.T) {
	if _, err := fetchSpec(context.Background(), "file:///etc/passwd", false); err == nil {
		t.Fatal("expected error for file:// scheme")
	}
}

func TestFetchSpec_RejectsPrivateHostWhenNotAllowed(t *testing.T) {
	// 127.0.0.1 is loopback/private -> rejected.
	if _, err := fetchSpec(context.Background(), "http://127.0.0.1/spec.json", false); err == nil {
		t.Fatal("expected error for private host")
	}
}

func TestFetchSpec_AllowsPrivateWhenFlagSet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"openapi":"3.0.0","info":{"title":"X","version":"1"}}`))
	}))
	defer srv.Close()
	body, err := fetchSpec(context.Background(), srv.URL, true)
	if err != nil {
		t.Fatalf("fetchSpec error: %v", err)
	}
	if len(body) == 0 {
		t.Fatal("expected a body")
	}
}

func TestFetchSpec_CapsBodySize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		big := make([]byte, (2<<20)+1024)
		for i := range big {
			big[i] = 'a'
		}
		_, _ = w.Write(big)
	}))
	defer srv.Close()
	body, err := fetchSpec(context.Background(), srv.URL, true)
	if err != nil {
		t.Fatalf("fetchSpec error: %v", err)
	}
	if len(body) != maxSpecBytes {
		t.Fatalf("body not capped to exact maxSpecBytes: got %d bytes, want %d", len(body), maxSpecBytes)
	}
}

func TestFetchSpec_DoesNotFollowRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://10.0.0.1/internal", http.StatusFound)
	}))
	defer srv.Close()
	// allowPrivate=true so the httptest 127.0.0.1 host is permitted; we are
	// testing that the 302 redirect is NOT followed (10.0.0.1 is never reached).
	_, err := fetchSpec(context.Background(), srv.URL, true)
	if err == nil {
		t.Fatal("expected error: redirect should not be followed (302 is not 200 → ErrBadSpec)")
	}
}

// ensure the resolver hook is honoured: a hostname resolving to a private IP is rejected
func TestFetchSpec_RejectsHostnameResolvingPrivate(t *testing.T) {
	orig := lookupIP
	lookupIP = func(string) ([]net.IP, error) { return []net.IP{net.ParseIP("10.0.0.5")}, nil }
	defer func() { lookupIP = orig }()
	if _, err := fetchSpec(context.Background(), "http://internal.evil.test/spec.json", false); err == nil {
		t.Fatal("expected rejection of hostname resolving to a private IP")
	}
}
