package admin

import (
	"net"
	"regexp"
	"strings"
	"time"
)

// Product is an API product as managed by an admin: the full field set, including
// upstream_url and the published flag (admins see unpublished products too).
type Product struct {
	ID                 int64    `json:"id"`
	Name               string   `json:"name"`
	Slug               string   `json:"slug"`
	Category           string   `json:"category"`
	Version            string   `json:"version"`
	ContextPath        string   `json:"contextPath"`
	Description        string   `json:"description"`
	Tags               []string `json:"tags"`
	Icon               string   `json:"icon"`
	UpstreamURL        string   `json:"upstreamUrl"`
	SandboxUpstreamURL string   `json:"sandboxUpstreamUrl"`
	Published          bool     `json:"published"`
	AuthType           string   `json:"authType"`
	// OpenAPISpec is the raw OpenAPI/Swagger document (JSON or YAML) backing the
	// product's docs + Try-it. Empty = no docs. omitempty so list/update
	// responses (which don't re-select it) don't echo an empty string.
	OpenAPISpec string `json:"openapiSpec,omitempty"`

	// LifecycleStatus is one of "active" (default), "deprecated", "sunset".
	LifecycleStatus string `json:"lifecycleStatus"`
	// SunsetDate is the YYYY-MM-DD date the product is/will be retired, when set.
	SunsetDate *string `json:"sunsetDate"`
}

// ChangelogEntry is one recorded change for a product, managed by an admin.
type ChangelogEntry struct {
	ID      int64  `json:"id"`
	Version string `json:"version"`
	Kind    string `json:"kind"`
	Notes   string `json:"notes"`
	Date    string `json:"date"`
}

// validate returns "" when the product is valid, otherwise an i18n message key
// (see internal/i18n) describing the reason. upstream_url is optional (a
// product may be defined before its backend exists), but when present it must
// be host:port.
func (p Product) validate(allowPrivate bool) string {
	if strings.TrimSpace(p.Name) == "" {
		return "common.nameRequired"
	}
	if strings.TrimSpace(p.Slug) == "" {
		return "admin.product.slugRequired"
	}
	if strings.TrimSpace(p.Category) == "" {
		return "admin.product.categoryRequired"
	}
	if strings.TrimSpace(p.ContextPath) == "" {
		return "admin.product.contextPathRequired"
	}
	if !ValidContextPath(p.ContextPath) {
		return "admin.product.badContextPath"
	}
	if p.UpstreamURL != "" && !ValidUpstream(p.UpstreamURL, allowPrivate) {
		return "admin.product.badUpstream"
	}
	if p.SandboxUpstreamURL != "" && !ValidUpstream(p.SandboxUpstreamURL, allowPrivate) {
		return "admin.product.badSandboxUpstream"
	}
	if p.AuthType != "" && p.AuthType != "key-auth" && p.AuthType != "oauth2" {
		return "admin.product.badAuthType"
	}
	if p.LifecycleStatus != "" && p.LifecycleStatus != "active" && p.LifecycleStatus != "deprecated" && p.LifecycleStatus != "sunset" {
		return "admin.product.badLifecycleStatus"
	}
	if p.SunsetDate != nil && *p.SunsetDate != "" {
		if _, err := time.Parse("2006-01-02", *p.SunsetDate); err != nil {
			return "admin.product.badSunsetDate"
		}
	}
	return ""
}

// lookupIP resolves a hostname at validation time. Overridden in tests.
var lookupIP = net.LookupIP

func isPrivateIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsPrivate() || ip.IsUnspecified()
}

// ValidUpstream checks host:port shape and, unless allowPrivate is set, rejects
// targets in loopback / link-local / private ranges to prevent SSRF (H4).
// Hostnames are resolved here and rejected if ANY resolved address is private —
// this catches libc shorthand IPs ("127.1") and attacker domains pointing at
// internal ranges. Residual risk: DNS can change between validation and
// proxying (rebinding); the long-term fix is an operator allow-list. The dev
// stack sets allowPrivate so docker-internal hosts (echo:8080) work.
func ValidUpstream(s string, allowPrivate bool) bool {
	// Accept an optional scheme prefix (https://host:port); imported products
	// carry one so a TLS backend is reached over HTTPS. The SSRF checks below
	// still apply to the host underneath.
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+len("://"):]
	}
	host, port, err := net.SplitHostPort(s)
	if err != nil || host == "" || port == "" {
		return false
	}
	for _, r := range port {
		if r < '0' || r > '9' {
			return false
		}
	}
	if allowPrivate {
		return true
	}
	if strings.EqualFold(host, "localhost") {
		return false
	}
	// IP literal (SplitHostPort already unbracketed IPv6).
	if ip := net.ParseIP(host); ip != nil {
		return !isPrivateIP(ip)
	}
	// A hostname with no dot is an internal/docker name → block.
	if !strings.Contains(host, ".") {
		return false
	}
	// Resolve and fail closed: unresolvable or any-private → reject.
	ips, err := lookupIP(host)
	if err != nil || len(ips) == 0 {
		return false
	}
	for _, ip := range ips {
		if isPrivateIP(ip) {
			return false
		}
	}
	return true
}

var ctxPathRe = regexp.MustCompile(`^/[A-Za-z0-9](?:[A-Za-z0-9/_-]*[A-Za-z0-9])?$`)

// ValidContextPath enforces a safe route prefix: must start with "/", only
// alnum/_/-//, no spaces or wildcards, no trailing slash, not bare "/" (M1).
func ValidContextPath(p string) bool {
	if p == "/" || strings.Contains(p, "//") {
		return false
	}
	return ctxPathRe.MatchString(p)
}
