package admin

import (
	"strings"
	"testing"
)

func TestParseSpec_OpenAPI3_JSON(t *testing.T) {
	spec := `{
		"openapi": "3.0.0",
		"info": {"title": "Currency Converter API", "version": "2.1.0", "description": "Converts currencies."},
		"servers": [{"url": "https://api.example.com:8443/currency/v2"}],
		"tags": [{"name": "Finance"}, {"name": "FX"}]
	}`
	p, err := parseSpec([]byte(spec))
	if err != nil {
		t.Fatalf("parseSpec error: %v", err)
	}
	if p.Name != "Currency Converter API" {
		t.Errorf("Name = %q", p.Name)
	}
	if p.Version != "2.1.0" {
		t.Errorf("Version = %q", p.Version)
	}
	if p.Description != "Converts currencies." {
		t.Errorf("Description = %q", p.Description)
	}
	if p.Slug != "currency-converter-api" {
		t.Errorf("Slug = %q", p.Slug)
	}
	if p.ContextPath != "/currency/v2" {
		t.Errorf("ContextPath = %q", p.ContextPath)
	}
	if p.UpstreamURL != "https://api.example.com:8443" {
		t.Errorf("UpstreamURL = %q", p.UpstreamURL)
	}
	if p.Category != "Finance" {
		t.Errorf("Category = %q", p.Category)
	}
	if len(p.Tags) != 2 || p.Tags[0] != "Finance" || p.Tags[1] != "FX" {
		t.Errorf("Tags = %v", p.Tags)
	}
	if p.Published {
		t.Error("Published should default to false")
	}
}

func TestParseSpec_YAML(t *testing.T) {
	spec := "openapi: 3.0.0\ninfo:\n  title: Weather\n  version: 1.2.0\nservers:\n  - url: https://weather.example.com/w\n"
	p, err := parseSpec([]byte(spec))
	if err != nil {
		t.Fatalf("parseSpec error: %v", err)
	}
	if p.Name != "Weather" || p.Version != "1.2.0" {
		t.Errorf("got %q / %q", p.Name, p.Version)
	}
	if p.ContextPath != "/w" {
		t.Errorf("ContextPath = %q", p.ContextPath)
	}
	// no explicit port, https -> 443
	if p.UpstreamURL != "https://weather.example.com:443" {
		t.Errorf("UpstreamURL = %q", p.UpstreamURL)
	}
}

func TestParseSpec_Swagger2(t *testing.T) {
	spec := `{
		"swagger": "2.0",
		"info": {"title": "Pet Store", "version": "1.0.0"},
		"host": "petstore.example.com",
		"basePath": "/v1",
		"schemes": ["https"]
	}`
	p, err := parseSpec([]byte(spec))
	if err != nil {
		t.Fatalf("parseSpec error: %v", err)
	}
	if p.ContextPath != "/v1" {
		t.Errorf("ContextPath = %q", p.ContextPath)
	}
	if p.UpstreamURL != "https://petstore.example.com:443" {
		t.Errorf("UpstreamURL = %q", p.UpstreamURL)
	}
}

func TestParseSpec_MissingTitle(t *testing.T) {
	if _, err := parseSpec([]byte(`{"openapi":"3.0.0","info":{"version":"1.0.0"}}`)); err == nil {
		t.Fatal("expected error for missing title")
	}
}

func TestParseSpec_Garbage(t *testing.T) {
	if _, err := parseSpec([]byte("not a spec at all: [unbalanced")); err == nil {
		t.Fatal("expected error for unparseable bytes")
	}
}

func TestParseSpec_NoServersFallsBackToSlugPath(t *testing.T) {
	p, err := parseSpec([]byte(`{"openapi":"3.0.0","info":{"title":"Bare API","version":"1.0.0"}}`))
	if err != nil {
		t.Fatalf("parseSpec error: %v", err)
	}
	if p.ContextPath != "/bare" {
		t.Errorf("ContextPath = %q, want /bare", p.ContextPath)
	}
	if p.UpstreamURL != "" {
		t.Errorf("UpstreamURL = %q, want empty", p.UpstreamURL)
	}
	if !strings.HasPrefix(p.ContextPath, "/") {
		t.Errorf("ContextPath must start with /")
	}
}
