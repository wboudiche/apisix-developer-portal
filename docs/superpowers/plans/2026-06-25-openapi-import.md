# OpenAPI / Swagger Import Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let an admin create an API product by importing an OpenAPI 3.x / Swagger 2.0 spec (file upload or URL), which pre-fills the existing Composer form for review before the normal Create.

**Architecture:** A new backend endpoint `POST /api/admin/products/import` parses a spec (JSON or YAML, from a pasted-file body or a server-fetched URL) into a draft `admin.Product` and returns it **without persisting**. The frontend's import dialog calls it, then opens the existing Composer pre-filled. The admin reviews/edits and clicks Create, which hits the unchanged `POST /api/admin/products` (all existing validation applies). Spec is discarded after pre-fill.

**Tech Stack:** Go 1.25 (`net/http`, `go-chi/chi/v5`, new dep `sigs.k8s.io/yaml`), React 19 + TS + Vite, vitest.

## Global Constraints

- Go module is `apisix-portal`; admin code lives in `internal/admin`.
- New route mounts on the existing `admin.Handler` chi router — it is already served under `/api/admin/products/` behind `requireAdmin` in `internal/server/server.go:90-91`; **no server.go change needed**.
- SSRF: the URL fetch must reuse the existing private-range rejection logic (`isPrivateIP` in `internal/admin/product.go`) and honour the handler's `allowPrivate` flag (true in dev). Only `http`/`https` schemes.
- Error responses use `httpx.Error(w, status, msg)`; success uses `httpx.JSON(w, status, v)`. The error body shape is `{"error": "..."}`.
- Draft product returned to the client uses the exact `admin.Product` JSON shape (matches the frontend `AdminProduct` type) — see field tags in `internal/admin/product.go`.
- Frontend French copy, light/dark Atlas tokens. New API client fns follow the existing `parse<T>(...)` + `authHeaders(token)` pattern in `web/src/api/client.ts`.
- Imported draft always has `published = false` and is treated as a **create** (no `id`) in the Composer.

---

## Task 1: Add the `sigs.k8s.io/yaml` dependency

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Add the dependency**

Run:
```bash
cd /home/walidboudiche/working/apisix-developper-portal
go get sigs.k8s.io/yaml@v1.4.0
```
Expected: `go.mod` gains `sigs.k8s.io/yaml v1.4.0` and `go.sum` is updated.

- [ ] **Step 2: Verify it resolves**

Run: `go build ./...`
Expected: builds with no error (dependency downloaded).

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "build: add sigs.k8s.io/yaml for OpenAPI spec parsing"
```

---

## Task 2: Spec parsing + field mapping (`draftFromSpec`)

Pure, HTTP-free parsing and mapping. This is the core of the feature and is fully unit-testable.

**Files:**
- Create: `internal/admin/import.go`
- Test: `internal/admin/import_test.go`

**Interfaces:**
- Consumes: `Product` and `ValidContextPath`/`slug` helpers from `internal/admin/product.go`; `sigs.k8s.io/yaml`.
- Produces:
  - `func parseSpec(data []byte) (Product, error)` — decodes JSON-or-YAML bytes into a draft `Product`. Returns `ErrBadSpec` (sentinel) when the bytes can't be decoded or `info.title` is empty.
  - `var ErrBadSpec = errors.New("admin: spec could not be parsed")`
  - `func specSlugify(title string) string` — internal helper (lowercase, non-alphanumerics→`-`, trim). (Mirrors the frontend `slugify` minus the trailing-"api" strip; backend keeps it simple.)

- [ ] **Step 1: Write the failing tests**

Create `internal/admin/import_test.go`:
```go
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
	if p.UpstreamURL != "api.example.com:8443" {
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
	if p.UpstreamURL != "weather.example.com:443" {
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
	if p.UpstreamURL != "petstore.example.com:443" {
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/admin/ -run TestParseSpec -v`
Expected: FAIL — `undefined: parseSpec`.

- [ ] **Step 3: Implement `internal/admin/import.go`**

```go
package admin

import (
	"errors"
	"net"
	"net/url"
	"strings"

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
		ctxPath = "/" + slug
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
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/admin/ -run TestParseSpec -v`
Expected: PASS (all 6 subtests).

- [ ] **Step 5: Run the full admin package + vet**

Run: `go vet ./internal/admin/ && go test ./internal/admin/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/admin/import.go internal/admin/import_test.go
git commit -m "feat(admin): parse OpenAPI 3.x / Swagger 2.0 spec into a draft product"
```

---

## Task 3: Server-side URL fetch with SSRF guard (`fetchSpec`)

**Files:**
- Modify: `internal/admin/import.go`
- Test: `internal/admin/import_fetch_test.go`

**Interfaces:**
- Consumes: `isPrivateIP` from `internal/admin/product.go`; `lookupIP` (the overridable resolver var in `product.go`).
- Produces:
  - `func fetchSpec(ctx context.Context, rawURL string, allowPrivate bool) ([]byte, error)` — validates scheme + host, GETs the URL with a 5s timeout, returns up to `maxSpecBytes` of the body. Returns `ErrUnsafeURL` when the scheme isn't http(s) or the host resolves to a private range (and `allowPrivate` is false).
  - `var ErrUnsafeURL = errors.New("admin: url is not allowed")`
  - `const maxSpecBytes = 2 << 20` (2 MiB)

- [ ] **Step 1: Write the failing tests**

Create `internal/admin/import_fetch_test.go`:
```go
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
	if len(body) > maxSpecBytes {
		t.Fatalf("body not capped: %d bytes", len(body))
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/admin/ -run TestFetchSpec -v`
Expected: FAIL — `undefined: fetchSpec`.

- [ ] **Step 3: Append the implementation to `internal/admin/import.go`**

Add these imports to the existing import block: `"context"`, `"io"`, `"net/http"`, `"time"`. Then append:
```go
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

// hostAllowed mirrors the SSRF policy in ValidUpstream: literal private IPs,
// "localhost", and hostnames resolving to any private address are blocked
// unless allowPrivate is set.
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
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/admin/ -run TestFetchSpec -v`
Expected: PASS (all 5 subtests).

- [ ] **Step 5: Run the full admin package + vet**

Run: `go vet ./internal/admin/ && go test ./internal/admin/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/admin/import.go internal/admin/import_fetch_test.go
git commit -m "feat(admin): SSRF-guarded URL fetch for spec import"
```

---

## Task 4: Import HTTP endpoint `POST /api/admin/products/import`

**Files:**
- Modify: `internal/admin/handler.go`
- Test: `internal/admin/import_handler_test.go`

**Interfaces:**
- Consumes: `parseSpec`, `fetchSpec`, `ErrBadSpec`, `ErrUnsafeURL` (Tasks 2–3); `httpx.JSON`/`httpx.Error`; the handler's existing `allowPrivate` field.
- Produces: a registered route `POST /api/admin/products/import` on the existing `Handler` chi router. Request body `{"spec": "..."}` OR `{"url": "..."}` (exactly one). Returns `200` draft `Product`, `400` bad body, `422` bad spec / unsafe url. **Persists nothing** (does not call `svc`).

- [ ] **Step 1: Write the failing tests**

Create `internal/admin/import_handler_test.go`:
```go
package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newImportHandler builds a Handler with a nil service: import must not touch it.
func newImportHandler(allowPrivate bool) *Handler {
	return NewHandler(nil, allowPrivate)
}

func TestImport_SpecBody_OK(t *testing.T) {
	h := newImportHandler(false)
	body := `{"spec": "{\"openapi\":\"3.0.0\",\"info\":{\"title\":\"Imported API\",\"version\":\"3.0.0\"},\"servers\":[{\"url\":\"https://api.example.com/v3\"}]}"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/products/import", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var p Product
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if p.Name != "Imported API" || p.Version != "3.0.0" {
		t.Errorf("got %q / %q", p.Name, p.Version)
	}
	if p.ContextPath != "/v3" {
		t.Errorf("ContextPath = %q", p.ContextPath)
	}
	if p.Published {
		t.Error("imported product must be unpublished")
	}
}

func TestImport_BadSpec_422(t *testing.T) {
	h := newImportHandler(false)
	body := `{"spec": "this is not a spec"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/products/import", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestImport_NeitherField_400(t *testing.T) {
	h := newImportHandler(false)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/products/import", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestImport_BothFields_400(t *testing.T) {
	h := newImportHandler(false)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/products/import",
		strings.NewReader(`{"spec":"x","url":"http://example.com"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestImport_UnsafeURL_422(t *testing.T) {
	h := newImportHandler(false)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/products/import",
		strings.NewReader(`{"url":"http://127.0.0.1/spec.json"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/admin/ -run TestImport -v`
Expected: FAIL — route returns 404/405 (handler not registered).

- [ ] **Step 3: Register the route and add the handler method**

In `internal/admin/handler.go`, add the route inside `NewHandler` (after the existing `POST /api/admin/products` line):
```go
	h.router.Post("/api/admin/products/import", h.importSpec)
```

Add these imports if missing: `"context"`. Then add the method (place near `create`):
```go
// importSpec parses an OpenAPI/Swagger spec (from a pasted body or a fetched
// URL) into a draft product and returns it WITHOUT persisting. The admin then
// reviews it in the form and POSTs it to /api/admin/products as usual.
func (h *Handler) importSpec(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Spec string `json:"spec"`
		URL  string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	body.Spec = strings.TrimSpace(body.Spec)
	body.URL = strings.TrimSpace(body.URL)
	if (body.Spec == "") == (body.URL == "") {
		httpx.Error(w, http.StatusBadRequest, "provide exactly one of spec or url")
		return
	}

	data := []byte(body.Spec)
	if body.URL != "" {
		fetched, err := fetchSpec(r.Context(), body.URL, h.allowPrivate)
		if err != nil {
			httpx.Error(w, http.StatusUnprocessableEntity, "could not fetch spec from url")
			return
		}
		data = fetched
	}

	draft, err := parseSpec(data)
	if err != nil {
		httpx.Error(w, http.StatusUnprocessableEntity, "spec could not be parsed (need OpenAPI 3.x or Swagger 2.0 with a title)")
		return
	}
	httpx.JSON(w, http.StatusOK, draft)
}
```
Add `"strings"` to the handler's import block if not already present.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/admin/ -run TestImport -v`
Expected: PASS (all 5 subtests).

- [ ] **Step 5: Full backend suite + vet**

Run: `go vet ./... && go test ./internal/... ./cmd/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/admin/handler.go internal/admin/import_handler_test.go
git commit -m "feat(admin): POST /api/admin/products/import endpoint"
```

---

## Task 5: Frontend API client `adminImportProduct`

**Files:**
- Modify: `web/src/api/client.ts`
- Test: `web/src/api/client.import.test.ts`

**Interfaces:**
- Consumes: existing `parse<T>`, `authHeaders(token)`, `AdminProduct` type.
- Produces:
  - `export async function adminImportProduct(token: string, src: { spec: string } | { url: string }): Promise<AdminProduct>` — POSTs to `/api/admin/products/import`, returns the draft.

- [ ] **Step 1: Write the failing test**

Create `web/src/api/client.import.test.ts`:
```ts
import { describe, it, expect, vi, afterEach } from 'vitest'
import { adminImportProduct, ApiError } from './client'

afterEach(() => vi.restoreAllMocks())

const draft = {
  name: 'Imported API', slug: 'imported', category: 'Finance', version: '1.0.0',
  contextPath: '/v1', description: '', tags: ['Finance'], icon: '', upstreamUrl: 'api.example.com:443', published: false,
}

it('POSTs a pasted spec and returns the draft', async () => {
  const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
    new Response(JSON.stringify(draft), { status: 200, headers: { 'Content-Type': 'application/json' } }),
  )
  const out = await adminImportProduct('jwt', { spec: '{"openapi":"3.0.0"}' })
  expect(out.name).toBe('Imported API')
  const [url, init] = fetchMock.mock.calls[0]
  expect(url).toBe('/api/admin/products/import')
  expect((init as RequestInit).method).toBe('POST')
  expect(JSON.parse((init as RequestInit).body as string)).toEqual({ spec: '{"openapi":"3.0.0"}' })
})

it('surfaces a 422 as an ApiError', async () => {
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(
    new Response(JSON.stringify({ error: 'spec could not be parsed' }), { status: 422, headers: { 'Content-Type': 'application/json' } }),
  )
  await expect(adminImportProduct('jwt', { url: 'https://x/y' })).rejects.toBeInstanceOf(ApiError)
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd web && npx vitest run src/api/client.import.test.ts`
Expected: FAIL — `adminImportProduct` is not exported.

- [ ] **Step 3: Implement the client fn**

In `web/src/api/client.ts`, in the `// --- Admin: products ---` section (after `adminDeleteProduct`):
```ts
export async function adminImportProduct(token: string, src: { spec: string } | { url: string }): Promise<AdminProduct> {
  const url = '/api/admin/products/import'
  return parse<AdminProduct>(await fetch(url, { method: 'POST', headers: authHeaders(token), body: JSON.stringify(src) }), url)
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd web && npx vitest run src/api/client.import.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/api/client.ts web/src/api/client.import.test.ts
git commit -m "feat(web): adminImportProduct API client fn"
```

---

## Task 6: `ImportModal` component (Fichier / URL tabs)

**Files:**
- Create: `web/src/pages/admin/ImportModal.tsx`
- Test: `web/src/pages/admin/ImportModal.test.tsx`

**Interfaces:**
- Consumes: `adminImportProduct` (Task 5); `useAuth` (`web/src/auth/AuthProvider`); `AdminProduct` type; overlay styles `web/src/styles/overlays.css` (reuse `.appdetail-scrim` + a modal class — match `ConfirmModal`).
- Produces:
  - `export function ImportModal({ open, onClose, onImported }: { open: boolean; onClose: () => void; onImported: (draft: AdminProduct) => void }): JSX.Element | null` — renders nothing when `!open`. Two tabs: **Fichier** (`<input type="file" accept=".json,.yaml,.yml">`, read via `FileReader`) and **URL** (text input). On submit calls `adminImportProduct`, then `onImported(draft)` + `onClose()` on success; on error shows the message inline (`role="alert"`). A `busy` state disables the submit button while the request is in flight.

- [ ] **Step 1: Write the failing tests**

Create `web/src/pages/admin/ImportModal.test.tsx`:
```tsx
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ImportModal } from './ImportModal'
import { AuthProvider } from '../../auth/AuthProvider'
import * as api from '../../api/client'
import type { AdminProduct } from '../../api/types'

const draft: AdminProduct = {
  name: 'Imported API', slug: 'imported', category: 'Finance', version: '1.0.0',
  contextPath: '/v1', description: '', tags: ['Finance'], icon: '', upstreamUrl: 'api.example.com:443', published: false,
}

beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('token', 'jwt')
  localStorage.setItem('user', JSON.stringify({ id: 1, email: 'a@b.c', name: 'Admin', role: 'admin' }))
})
afterEach(() => vi.restoreAllMocks())

function renderModal(onImported = vi.fn(), onClose = vi.fn()) {
  render(<AuthProvider><ImportModal open onClose={onClose} onImported={onImported} /></AuthProvider>)
  return { onImported, onClose }
}

it('imports from a URL and calls onImported with the draft', async () => {
  const spy = vi.spyOn(api, 'adminImportProduct').mockResolvedValue(draft)
  const { onImported } = renderModal()
  await userEvent.click(screen.getByRole('tab', { name: /URL/i }))
  await userEvent.type(screen.getByPlaceholderText(/https/i), 'https://api.example.com/openapi.json')
  await userEvent.click(screen.getByRole('button', { name: /Importer/i }))
  await waitFor(() => expect(onImported).toHaveBeenCalledWith(draft))
  expect(spy).toHaveBeenCalledWith('jwt', { url: 'https://api.example.com/openapi.json' })
})

it('shows the backend error message on failure', async () => {
  vi.spyOn(api, 'adminImportProduct').mockRejectedValue(new api.ApiError('spec could not be parsed', 422))
  renderModal()
  await userEvent.click(screen.getByRole('tab', { name: /URL/i }))
  await userEvent.type(screen.getByPlaceholderText(/https/i), 'https://x/y')
  await userEvent.click(screen.getByRole('button', { name: /Importer/i }))
  expect(await screen.findByRole('alert')).toHaveTextContent(/spec could not be parsed/i)
})
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd web && npx vitest run src/pages/admin/ImportModal.test.tsx`
Expected: FAIL — cannot find `./ImportModal`.

- [ ] **Step 3: Implement `web/src/pages/admin/ImportModal.tsx`**

```tsx
import '../../styles/overlays.css'
import { useEffect, useState } from 'react'
import type { AdminProduct } from '../../api/types'
import { adminImportProduct } from '../../api/client'
import { useAuth } from '../../auth/AuthProvider'

type Tab = 'file' | 'url'

export function ImportModal({ open, onClose, onImported }: {
  open: boolean
  onClose: () => void
  onImported: (draft: AdminProduct) => void
}) {
  const { token } = useAuth()
  const [tab, setTab] = useState<Tab>('file')
  const [url, setUrl] = useState('')
  const [spec, setSpec] = useState('')
  const [fileName, setFileName] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (!open) return
    function onKey(e: KeyboardEvent) { if (e.key === 'Escape') onClose() }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [open, onClose])

  if (!open) return null

  async function onFile(e: React.ChangeEvent<HTMLInputElement>) {
    const f = e.target.files?.[0]
    if (!f) return
    setFileName(f.name)
    setSpec(await f.text())
    setErr('')
  }

  async function submit() {
    if (!token || busy) return
    const src = tab === 'url' ? { url: url.trim() } : { spec: spec.trim() }
    if ((tab === 'url' && !src.url) || (tab === 'file' && !('spec' in src && src.spec))) {
      setErr(tab === 'url' ? 'Saisissez une URL.' : 'Choisissez un fichier de spécification.')
      return
    }
    setBusy(true); setErr('')
    try {
      const draft = await adminImportProduct(token, src)
      onImported(draft)
      onClose()
    } catch (e) {
      setErr(e instanceof Error ? e.message : "Échec de l'import.")
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="appdetail-scrim" onClick={e => { if (e.target === e.currentTarget) onClose() }}>
      <div className="dmodal" role="dialog" aria-modal="true" aria-label="Importer une API">
        <div className="composer-head">
          <span className="dot" />
          <h2>Importer une API</h2>
          <span className="hint">OpenAPI 3.x ou Swagger 2.0</span>
        </div>

        <div className="tabs" role="tablist" aria-label="Source de la spécification">
          <button role="tab" aria-selected={tab === 'file'} className={`tab ${tab === 'file' ? 'on' : ''}`}
            onClick={() => { setTab('file'); setErr('') }}>Fichier</button>
          <button role="tab" aria-selected={tab === 'url'} className={`tab ${tab === 'url' ? 'on' : ''}`}
            onClick={() => { setTab('url'); setErr('') }}>URL</button>
        </div>

        <div className="composer-body">
          {tab === 'file' ? (
            <div className="field">
              <label htmlFor="imp-file">Fichier de spécification</label>
              <input id="imp-file" type="file" accept=".json,.yaml,.yml" onChange={onFile} />
              {fileName && <div className="help">{fileName}</div>}
            </div>
          ) : (
            <div className="field">
              <label htmlFor="imp-url">URL de la spécification</label>
              <input id="imp-url" className="ipt mono" placeholder="https://api.example.com/openapi.json"
                autoComplete="off" value={url} onChange={e => setUrl(e.target.value)} />
            </div>
          )}

          {err && <p className="autherr" role="alert">{err}</p>}

          <div className="composer-foot">
            <div className="foot-acts">
              <button type="button" className="btn btn-ghost btn-sm" onClick={onClose}>Annuler</button>
              <button type="button" className="btn btn-primary btn-sm" onClick={submit} disabled={busy}>
                {busy ? 'Import…' : 'Importer'}
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
```

- [ ] **Step 4: Add minimal tab styling**

Append to `web/src/styles/overlays.css`:
```css
/* Import modal tabs */
.tabs { display: flex; gap: 4px; margin: 12px 0 4px; border-bottom: 1px solid var(--ink-line, #2a2a2a); }
.tabs .tab { background: none; border: none; padding: 8px 14px; cursor: pointer; color: var(--ink-soft); font: inherit; border-bottom: 2px solid transparent; }
.tabs .tab.on { color: var(--accent); border-bottom-color: var(--accent); }
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd web && npx vitest run src/pages/admin/ImportModal.test.tsx`
Expected: PASS (both tests).

- [ ] **Step 6: Commit**

```bash
git add web/src/pages/admin/ImportModal.tsx web/src/pages/admin/ImportModal.test.tsx web/src/styles/overlays.css
git commit -m "feat(web): ImportModal with file and URL tabs"
```

---

## Task 7: Wire import into `ProductsPage` (open Composer pre-filled)

**Files:**
- Modify: `web/src/pages/admin/ProductsPage.tsx`
- Modify: `web/src/pages/admin/ProductsPage.test.tsx`

**Interfaces:**
- Consumes: `ImportModal` (Task 6); existing `FormState`, `EMPTY`, `openCreate`, `setForm`, `setOpen`, `setEditing`, `setSlugTouched` in `ProductsPage`.
- Produces: an "Importer" button that opens `ImportModal`; an `onImported(draft)` handler that fills the Composer as a **create** (no `editing`) from the draft and opens it.

- [ ] **Step 1: Write the failing test**

Add to `web/src/pages/admin/ProductsPage.test.tsx` (inside the `describe('ProductsPage', …)` block):
```tsx
  it('import opens the Composer pre-filled from the returned draft', async () => {
    vi.spyOn(api, 'adminImportProduct').mockResolvedValue({
      name: 'Imported API', slug: 'imported', category: 'Finance', version: '2.5.0',
      contextPath: '/v2', description: 'desc', tags: ['Finance'], icon: '', upstreamUrl: 'api.example.com:443', published: false,
    })
    renderPage()
    await screen.findByText('CurrencyConverterAPI')
    await userEvent.click(screen.getByRole('button', { name: /Importer/i }))
    await userEvent.click(screen.getByRole('tab', { name: /URL/i }))
    await userEvent.type(screen.getByPlaceholderText(/https/i), 'https://api.example.com/openapi.json')
    await userEvent.click(screen.getByRole('button', { name: /^Importer$/i }))

    // Composer is now open, pre-filled as a create
    expect(await screen.findByText('Créer un produit')).toBeInTheDocument()
    expect(screen.getByLabelText('Nom')).toHaveValue('Imported API')
    expect(screen.getByLabelText('Version')).toHaveValue('2.5.0')
    expect(screen.getByLabelText('Context path')).toHaveValue('/v2')
  })
```
Note: there are two "Importer" matches once the modal opens (the page button and the modal submit). The test clicks the page button first (only one exists at that point), then the modal's submit via the exact `/^Importer$/i` after switching tabs. If the page button label differs, keep the page button label as `Importer une API` and the modal submit as `Importer` to disambiguate — adjust the queries to match the labels chosen in Step 2/Task 6.

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd web && npx vitest run src/pages/admin/ProductsPage.test.tsx`
Expected: FAIL — no "Importer" button yet.

- [ ] **Step 3: Wire the modal and handler into `ProductsPage.tsx`**

Add the import at the top:
```tsx
import { ImportModal } from './ImportModal'
```
Add state near the other `useState` hooks:
```tsx
  const [importOpen, setImportOpen] = useState(false)
```
Add the imported-draft handler (next to `openCreate`):
```tsx
  function onImported(draft: AdminProduct) {
    setEditing(null)
    setForm({
      name: draft.name, slug: draft.slug, category: draft.category,
      contextPath: draft.contextPath, upstreamUrl: draft.upstreamUrl,
      version: draft.version, published: false,
    })
    setSlugTouched(true) // slug came from the spec; don't auto-overwrite on name edits
    setOpen(true)
  }
```
In the `action` prop of `<AdminShell>`, add an Import button before the existing "Nouveau produit" button:
```tsx
      action={
        <>
          <button className="btn btn-ghost" onClick={() => setImportOpen(true)}>Importer une API</button>
          <button className="btn btn-primary" onClick={() => open ? setOpen(false) : openCreate()}>
            <PlusIcon />Nouveau produit
          </button>
        </>
      }
```
Render the modal near the other overlays (before `</AdminShell>`):
```tsx
      <ImportModal open={importOpen} onClose={() => setImportOpen(false)} onImported={onImported} />
```
Adjust the new test's query labels to match: page button is **"Importer une API"**, modal submit is **"Importer"**. Update the Step 1 test so the first click targets `/Importer une API/i` and the modal submit targets `/^Importer$/i`.

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd web && npx vitest run src/pages/admin/ProductsPage.test.tsx`
Expected: PASS.

- [ ] **Step 5: Run the full frontend suite + typecheck + build**

Run: `cd web && npx vitest run && npx tsc --noEmit && npm run build`
Expected: all green.

- [ ] **Step 6: Commit**

```bash
git add web/src/pages/admin/ProductsPage.tsx web/src/pages/admin/ProductsPage.test.tsx
git commit -m "feat(web): import API button pre-fills the product Composer"
```

---

## Task 8: Full-stack verification + spec/memory note

**Files:**
- Modify: none (verification), optional `docs/superpowers/specs/2026-06-25-openapi-import-design.md` status line.

- [ ] **Step 1: Backend suite**

Run: `go vet ./... && go test ./internal/... ./cmd/...`
Expected: PASS.

- [ ] **Step 2: Frontend suite + build**

Run: `cd web && npx vitest run && npx tsc --noEmit && npm run build`
Expected: PASS.

- [ ] **Step 3: Manual smoke (optional, needs the dev stack)**

Bring up the stack (`PORTAL_ADDR=:8090`, vite `PORTAL_PROXY=http://localhost:8090 --port 5174 --strictPort` per project notes). As an admin: Products → "Importer une API" → URL tab → paste a public spec URL (e.g. `https://petstore3.swagger.io/api/v3/openapi.json`) → confirm the Composer opens pre-filled (name/version/context path) → edit upstream if needed → Create → product appears in the list as a draft. Then File tab → upload a local `.yaml` spec → same flow.

- [ ] **Step 4: Update the spec status + commit**

Edit the spec header `Status:` to `Implemented (2026-06-25)`.
```bash
git add docs/superpowers/specs/2026-06-25-openapi-import-design.md
git commit -m "docs(spec): mark OpenAPI import implemented"
```

---

## Self-Review notes

- **Spec coverage:** file upload (Task 6 file tab) + URL fetch (Task 3) ✓; backend parse 3.x + 2.0 (Task 2) ✓; pre-fill existing Composer, create path reused (Task 7) ✓; spec discarded / nothing persisted (Tasks 4 import handler never calls `svc`) ✓; SSRF guard reusing `isPrivateIP`/`allowPrivate` (Task 3) ✓; field mapping table (Task 2) ✓; 422/400 errors (Task 4) ✓; tests both stacks (every task) ✓.
- **Type consistency:** `parseSpec`/`fetchSpec`/`importSpec`/`adminImportProduct`/`ImportModal`/`onImported` names used consistently across tasks; `Product` JSON shape == `AdminProduct`.
- **Note for implementer:** `NewHandler(nil, allowPrivate)` is used in the import handler test because import never touches the service; if `NewHandler` ever dereferences `svc` at construction time, pass a zero `&Service{}` instead.
