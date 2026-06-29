package apisix

import (
	"strings"
	"testing"
)

// routeBody must carry the upstream scheme so a TLS backend (e.g. httpbin:443)
// is actually reached over HTTPS, expose both the context root and its
// subpaths, and strip the context prefix before forwarding to the upstream
// (WSO2-style), so /headers/get reaches the backend at /get.
func TestRouteBodyCarriesSchemeAndStripsPrefix(t *testing.T) {
	body, err := routeBody("/headers", "https://httpbin.org:443", []string{"app_1"})
	if err != nil {
		t.Fatalf("routeBody: %v", err)
	}

	up := body["upstream"].(map[string]any)
	if up["scheme"] != "https" {
		t.Fatalf("scheme = %v, want https", up["scheme"])
	}
	nodes := up["nodes"].(map[string]int)
	if _, ok := nodes["httpbin.org:443"]; !ok {
		t.Fatalf("nodes = %v, want httpbin.org:443", nodes)
	}

	uris, ok := body["uris"].([]string)
	if !ok || len(uris) != 2 || uris[0] != "/headers" || uris[1] != "/headers/*" {
		t.Fatalf("uris = %v, want [/headers /headers/*]", body["uris"])
	}

	plugins := body["plugins"].(map[string]any)
	pr, ok := plugins["proxy-rewrite"].(map[string]any)
	if !ok {
		t.Fatalf("missing proxy-rewrite plugin: %v", plugins)
	}
	rx := pr["regex_uri"].([]string)
	if len(rx) != 2 || rx[0] != "^/headers/?(.*)$" || rx[1] != "/$1" {
		t.Fatalf("regex_uri = %v, want [^/headers/?(.*)$ /$1]", rx)
	}
}

// A schemeless upstream (the legacy host:port form, e.g. the local echo demo)
// must keep working and default to http.
func TestRouteBodySchemelessDefaultsHTTP(t *testing.T) {
	body, err := routeBody("/itecho", "echo:8080", nil)
	if err != nil {
		t.Fatalf("routeBody: %v", err)
	}
	up := body["upstream"].(map[string]any)
	if up["scheme"] != "http" {
		t.Fatalf("scheme = %v, want http", up["scheme"])
	}
	nodes := up["nodes"].(map[string]int)
	if _, ok := nodes["echo:8080"]; !ok {
		t.Fatalf("nodes = %v, want echo:8080", nodes)
	}
}

func TestOAuthRouteBodyHasOIDCAndWhitelist(t *testing.T) {
	body, err := oauthRouteBody("/orders", "echo:8080", "https://idp.example/realms/dev", "azp", []string{"client-a", "client-b"})
	if err != nil {
		t.Fatalf("oauthRouteBody: %v", err)
	}
	plugins := body["plugins"].(map[string]any)
	oidc, ok := plugins["openid-connect"].(map[string]any)
	if !ok || oidc["bearer_only"] != true {
		t.Fatalf("openid-connect missing/!bearer_only: %v", plugins["openid-connect"])
	}
	if d, _ := oidc["discovery"].(string); d != "https://idp.example/realms/dev/.well-known/openid-configuration" {
		t.Fatalf("discovery = %v", oidc["discovery"])
	}
	sp, ok := plugins["serverless-pre-function"].(map[string]any)
	if !ok {
		t.Fatalf("serverless-pre-function missing")
	}
	fns := sp["functions"].([]string)
	if len(fns) != 1 || !strings.Contains(fns[0], `["client-a"]=true`) || !strings.Contains(fns[0], `["client-b"]=true`) {
		t.Fatalf("allow table missing client ids: %s", fns[0])
	}
	if !strings.Contains(fns[0], `claims["azp"]`) {
		t.Fatalf("claim name not wired: %s", fns[0])
	}
	// openid-connect must carry a non-empty client_id (APISIX 3.9.1 schema requirement)
	if cid, _ := oidc["client_id"].(string); cid == "" {
		t.Fatalf("openid-connect missing required client_id")
	}
	// no key-auth / consumer-restriction on an oauth route
	if _, has := plugins["key-auth"]; has {
		t.Fatalf("oauth route must not carry key-auth")
	}
}

func TestValidClientIDRejectsInjection(t *testing.T) {
	for _, good := range []string{"client-a", "svc.account@corp", "ABC_123:role"} {
		if !ValidClientID(good) {
			t.Errorf("ValidClientID(%q) = false, want true", good)
		}
	}
	for _, bad := range []string{`a"]=true os.execute("x")--`, "a b", "", "a\nb", strings.Repeat("x", 201)} {
		if ValidClientID(bad) {
			t.Errorf("ValidClientID(%q) = true, want false", bad)
		}
	}
}
