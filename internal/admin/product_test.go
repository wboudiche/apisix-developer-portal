package admin

import (
	"errors"
	"net"
	"testing"
)

func TestProductValidate(t *testing.T) {
	base := Product{Name: "Pizza", Slug: "pizza", Category: "Food", ContextPath: "/pizza"}

	cases := []struct {
		name    string
		mutate  func(p *Product)
		wantErr bool
	}{
		{"valid minimal", func(p *Product) {}, false},
		{"valid with upstream", func(p *Product) { p.UpstreamURL = "echo:8080" }, false},
		{"missing name", func(p *Product) { p.Name = "" }, true},
		{"missing slug", func(p *Product) { p.Slug = "" }, true},
		{"missing category", func(p *Product) { p.Category = "" }, true},
		{"missing contextPath", func(p *Product) { p.ContextPath = "" }, true},
		{"bad contextPath bare slash", func(p *Product) { p.ContextPath = "/" }, true},
		{"bad contextPath no leading slash", func(p *Product) { p.ContextPath = "orders" }, true},
		{"bad contextPath wildcard", func(p *Product) { p.ContextPath = "/orders/*" }, true},
		{"bad upstream no port", func(p *Product) { p.UpstreamURL = "echo" }, true},
		{"bad upstream non-numeric port", func(p *Product) { p.UpstreamURL = "echo:abc" }, true},
		{"bad upstream empty host", func(p *Product) { p.UpstreamURL = ":8080" }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := base
			tc.mutate(&p)
			// allowPrivate=true so docker-internal upstreams (echo:8080) are allowed
			msg := p.validate(true)
			if tc.wantErr && msg == "" {
				t.Fatal("expected validation error, got none")
			}
			if !tc.wantErr && msg != "" {
				t.Fatalf("unexpected validation error: %s", msg)
			}
		})
	}
}

func stubResolver(t *testing.T, table map[string][]net.IP) {
	t.Helper()
	orig := lookupIP
	lookupIP = func(host string) ([]net.IP, error) {
		if ips, ok := table[host]; ok {
			return ips, nil
		}
		return nil, errors.New("no such host")
	}
	t.Cleanup(func() { lookupIP = orig })
}

func TestValidUpstreamBlocksPrivateByDefault(t *testing.T) {
	for _, h := range []string{"127.0.0.1:80", "[::1]:80", "169.254.169.254:80", "10.0.0.5:8080", "192.168.1.1:9000", "localhost:8080", "echo:8080"} {
		if ValidUpstream(h, false) {
			t.Fatalf("%s must be rejected when private targets are blocked", h)
		}
	}
}

func TestValidUpstreamResolvesHostnames(t *testing.T) {
	stubResolver(t, map[string][]net.IP{
		"api.example.com": {net.ParseIP("93.184.216.34")},
		"evil.example":    {net.ParseIP("203.0.113.7"), net.ParseIP("169.254.169.254")},
		"127.1":           {net.ParseIP("127.0.0.1")}, // libc shorthand for loopback
	})
	if !ValidUpstream("api.example.com:443", false) {
		t.Fatal("public host resolving to public IPs must be allowed")
	}
	if ValidUpstream("evil.example:80", false) {
		t.Fatal("host with a private resolved address must be rejected")
	}
	if ValidUpstream("127.1:80", false) {
		t.Fatal("shorthand loopback (127.1) must be rejected")
	}
	if ValidUpstream("nonexistent.example.com:80", false) {
		t.Fatal("unresolvable host must be rejected")
	}
}

func TestValidUpstreamAllowsPrivateWithFlag(t *testing.T) {
	if !ValidUpstream("echo:8080", true) {
		t.Fatal("dev flag must allow internal docker hosts like echo:8080")
	}
}

func TestValidContextPath(t *testing.T) {
	ok := []string{"/orders", "/v1/orders", "/a-b_c"}
	bad := []string{"orders", "/orders/*", "/orders ", "/", "//x", "/a;b", "/orders/"}
	for _, p := range ok {
		if !ValidContextPath(p) {
			t.Fatalf("%q should be valid", p)
		}
	}
	for _, p := range bad {
		if ValidContextPath(p) {
			t.Fatalf("%q should be invalid", p)
		}
	}
}
