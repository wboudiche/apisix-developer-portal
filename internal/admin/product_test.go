package admin

import "testing"

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
		{"bad upstream no port", func(p *Product) { p.UpstreamURL = "echo" }, true},
		{"bad upstream non-numeric port", func(p *Product) { p.UpstreamURL = "echo:abc" }, true},
		{"bad upstream empty host", func(p *Product) { p.UpstreamURL = ":8080" }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := base
			tc.mutate(&p)
			msg := p.validate()
			if tc.wantErr && msg == "" {
				t.Fatal("expected validation error, got none")
			}
			if !tc.wantErr && msg != "" {
				t.Fatalf("unexpected validation error: %s", msg)
			}
		})
	}
}
