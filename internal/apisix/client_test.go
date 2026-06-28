package apisix

import "testing"

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
