package admin

import "strings"

// Product is an API product as managed by an admin: the full field set, including
// upstream_url and the published flag (admins see unpublished products too).
type Product struct {
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	Slug        string   `json:"slug"`
	Category    string   `json:"category"`
	Version     string   `json:"version"`
	ContextPath string   `json:"contextPath"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Icon        string   `json:"icon"`
	UpstreamURL string   `json:"upstreamUrl"`
	Published   bool     `json:"published"`
}

// validate returns "" when the product is valid, otherwise a human-readable reason.
// upstream_url is optional (a product may be defined before its backend exists),
// but when present it must be host:port.
func (p Product) validate() string {
	if strings.TrimSpace(p.Name) == "" {
		return "name is required"
	}
	if strings.TrimSpace(p.Slug) == "" {
		return "slug is required"
	}
	if strings.TrimSpace(p.Category) == "" {
		return "category is required"
	}
	if strings.TrimSpace(p.ContextPath) == "" {
		return "contextPath is required"
	}
	if p.UpstreamURL != "" && !validUpstream(p.UpstreamURL) {
		return "upstreamUrl must be host:port"
	}
	return ""
}

func validUpstream(s string) bool {
	host, port, found := strings.Cut(s, ":")
	if !found || host == "" || port == "" {
		return false
	}
	for _, r := range port {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
