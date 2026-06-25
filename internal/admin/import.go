package admin

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"sigs.k8s.io/yaml"
)

// ErrBadSpec is returned when an OpenAPI/Swagger document cannot be parsed or
// is missing a title.
var ErrBadSpec = errors.New("admin: spec could not be parsed")

// specDoc is the minimal subset of OpenAPI 3.x and Swagger 2.0 we map from.
// Unmapped fields are ignored. JSON tags are used because sigs.k8s.io/yaml
// converts YAML to JSON before unmarshalling, so JSON tags cover both formats.
type specDoc struct {
	Info struct {
		Title       string `json:"title"`
		Version     string `json:"version"`
		Description string `json:"description"`
	} `json:"info"`
	// OpenAPI 3.x
	Servers []struct {
		URL string `json:"url"`
	} `json:"servers"`
	// Swagger 2.0
	Host     string   `json:"host"`
	BasePath string   `json:"basePath"`
	Schemes  []string `json:"schemes"`
	// Both
	Tags []struct {
		Name string `json:"name"`
	} `json:"tags"`
}

// parseSpec decodes JSON-or-YAML spec bytes into a draft Product. It never
// persists or contacts the network. Returns ErrBadSpec on unparseable input or
// a missing info.title.
func parseSpec(data []byte) (Product, error) {
	var doc specDoc
	// yaml.Unmarshal accepts JSON (a subset of YAML) and YAML alike.
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return Product{}, ErrBadSpec
	}
	title := strings.TrimSpace(doc.Info.Title)
	if title == "" {
		return Product{}, ErrBadSpec
	}

	slug := specSlugify(title)
	ctxPath, upstream := serverParts(doc)
	if ctxPath == "" {
		// Strip trailing "-api" for the context path (avoids redundant suffix
		// like /bare-api when the title is "Bare API").
		base := strings.TrimSuffix(slug, "-api")
		if base == "" {
			base = slug
		}
		ctxPath = "/" + base
	}

	tags := make([]string, 0, len(doc.Tags))
	for _, t := range doc.Tags {
		if n := strings.TrimSpace(t.Name); n != "" {
			tags = append(tags, n)
		}
	}
	category := ""
	if len(tags) > 0 {
		category = tags[0]
	}

	version := strings.TrimSpace(doc.Info.Version)
	if version == "" {
		version = "1.0.0"
	}

	return Product{
		Name:        title,
		Slug:        slug,
		Category:    category,
		Version:     version,
		ContextPath: ctxPath,
		Description: strings.TrimSpace(doc.Info.Description),
		Tags:        tags,
		Icon:        "",
		UpstreamURL: upstream,
		Published:   false,
	}, nil
}

// serverParts derives (contextPath, upstreamHostPort) from a spec's server
// definition. For OpenAPI 3.x it uses servers[0].url; for Swagger 2.0 it uses
// host + basePath + schemes. Either may be empty.
func serverParts(doc specDoc) (ctxPath, upstream string) {
	if len(doc.Servers) > 0 && strings.TrimSpace(doc.Servers[0].URL) != "" {
		return fromServerURL(doc.Servers[0].URL)
	}
	if strings.TrimSpace(doc.Host) != "" {
		scheme := "https"
		if len(doc.Schemes) > 0 && strings.EqualFold(doc.Schemes[0], "http") {
			scheme = "http"
		}
		return normalizePath(doc.BasePath), hostPort(doc.Host, scheme)
	}
	return "", ""
}

// fromServerURL parses an OpenAPI server URL into (path, host:port).
func fromServerURL(raw string) (ctxPath, upstream string) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return "", ""
	}
	return normalizePath(u.Path), hostPort(u.Host, u.Scheme)
}

// hostPort returns host:port, defaulting the port from the scheme when absent.
func hostPort(host, scheme string) string {
	if host == "" {
		return ""
	}
	if _, _, err := net.SplitHostPort(host); err == nil {
		return host // already host:port
	}
	port := "443"
	if strings.EqualFold(scheme, "http") {
		port = "80"
	}
	return host + ":" + port
}

// normalizePath trims a trailing slash and ensures a leading slash; "" or "/"
// become "".
func normalizePath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.TrimRight(p, "/")
	if p == "" {
		return ""
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p
}

// specSlugify lowercases and replaces runs of non-alphanumerics with '-'.
func specSlugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevDash = false
		} else if !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

// ErrUnsafeURL is returned when an import URL uses a disallowed scheme or
// resolves to a private/internal address.
var ErrUnsafeURL = errors.New("admin: url is not allowed")

const maxSpecBytes = 2 << 20 // 2 MiB

// fetchSpec GETs rawURL and returns up to maxSpecBytes of its body. It only
// allows http/https and, unless allowPrivate is set, rejects hosts that resolve
// to loopback/link-local/private/unspecified ranges (SSRF guard, mirroring
// ValidUpstream). Redirects are disabled so a public URL cannot bounce to an
// internal one.
func fetchSpec(ctx context.Context, rawURL string, allowPrivate bool) ([]byte, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, ErrUnsafeURL
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, ErrUnsafeURL
	}
	if !hostAllowed(u.Hostname(), allowPrivate) {
		return nil, ErrUnsafeURL
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse // do not follow redirects
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, ErrUnsafeURL
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, ErrBadSpec
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, ErrBadSpec
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSpecBytes))
	if err != nil {
		return nil, ErrBadSpec
	}
	return body, nil
}

// hostAllowed mirrors the SSRF policy in ValidUpstream: "localhost", literal
// private IPs, single-label hostnames (no dot — internal/docker names), and
// hostnames that resolve to any private address are blocked unless allowPrivate
// is set.
func hostAllowed(host string, allowPrivate bool) bool {
	if host == "" {
		return false
	}
	if allowPrivate {
		return true
	}
	if strings.EqualFold(host, "localhost") {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return !isPrivateIP(ip)
	}
	// A hostname with no dot is an internal/docker name → block.
	if !strings.Contains(host, ".") {
		return false
	}
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
