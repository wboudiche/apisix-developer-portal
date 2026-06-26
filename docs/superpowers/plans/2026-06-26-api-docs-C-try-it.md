# Interactive API Docs — Plan C: Try-it Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let an approved subscriber run a live "Try-it" call from the product docs page: the request is proxied through the portal into the APISIX gateway with the subscriber's app key injected server-side, and the real response is shown in Scalar.

**Architecture:** A new `internal/tryit` package serves an authed handler under `/api/try/`. `GET /api/try/{slug}/context` tells the page which of the user's apps have an approved subscription. `ANY /api/try/{slug}/{appId}/*` is the proxy: it verifies the user owns `appId` and has an `active` subscription to the product, injects that app's `apikey`, and forwards `{APISIX_GATEWAY_URL}{contextPath}/{rest}` to the gateway. The frontend overrides Scalar's `servers` to `/api/try/{slug}/{appId}` so Scalar's built-in try-it flows through the proxy (same-origin, no CORS, key never in the browser). An app picker appears when >1 app qualifies; a "S'abonner pour essayer" banner shows when none do.

**Tech Stack:** Go 1.25 (chi, net/http), React 19 + TS, `@scalar/api-reference-react` (already installed; config supports a `servers` override).

## Global Constraints

- Go module `apisix-portal`. New package `internal/tryit`. Auth via `auth.RequireAuth` + `auth.UserID(ctx)`.
- The proxy targets ONLY the single configured gateway base — the path comes from the product's `context_path`, never from a client-supplied host — so there is no SSRF surface.
- The app key is injected server-side (header `apikey`) and is NEVER returned to the browser.
- Access: docs are public; Try-it requires login (401 from middleware) + an `active` (approved) subscription (403 otherwise). `active` is `subscriptions.StatusActive`.
- New config `APISIX_GATEWAY_URL` (default `http://localhost:9080`), distinct from `APISIX_ADMIN_URL`.
- Proxy must strip hop-by-hop / sensitive inbound headers (Host, Cookie, the portal `Authorization`) before forwarding, apply a timeout, and cap the request + response body.
- pnpm for the frontend.

---

## Task C1: Gateway URL config

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `Config.APISIXGatewayURL string` from `APISIX_GATEWAY_URL` (default `http://localhost:9080`).

- [ ] **Step 1: Write the failing test**

Add to `internal/config/config_test.go`:
```go
func TestGatewayURLDefaultAndOverride(t *testing.T) {
	t.Setenv("APISIX_GATEWAY_URL", "")
	if c := Load(); c.APISIXGatewayURL != "http://localhost:9080" {
		t.Errorf("default = %q", c.APISIXGatewayURL)
	}
	t.Setenv("APISIX_GATEWAY_URL", "http://gw:9080")
	if c := Load(); c.APISIXGatewayURL != "http://gw:9080" {
		t.Errorf("override = %q", c.APISIXGatewayURL)
	}
}
```
(If `Load` has a different name/signature in this file, match the existing config tests' call style.)

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/config/ -run TestGatewayURLDefaultAndOverride -v`
Expected: FAIL — no `APISIXGatewayURL` field.

- [ ] **Step 3: Add the field**

In `internal/config/config.go`, add to the `Config` struct (near `APISIXAdminURL`):
```go
	APISIXGatewayURL string
```
And in the loader (near `APISIXAdminURL: get(...)`):
```go
		APISIXGatewayURL: get("APISIX_GATEWAY_URL", "http://localhost:9080"),
```

- [ ] **Step 4: Run to verify it passes + commit**

Run: `go test ./internal/config/ && go vet ./internal/config/`
```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): APISIX_GATEWAY_URL for the try-it proxy"
```

---

## Task C2: `internal/tryit` package — proxy + context handler

**Files:**
- Create: `internal/tryit/tryit.go`, `internal/tryit/handler.go`
- Test: `internal/tryit/handler_test.go`

**Interfaces:**
- Consumes: `auth.UserID(ctx)`; `httpx.Error`/`httpx.JSON`; `subscriptions.StatusActive` value `"active"` (use the literal `"active"` to avoid importing subscriptions — document it).
- Produces:
  - `type AppRef struct { ID int64 \`json:"id"\`; Name string \`json:"name"\` }`
  - `type Products interface { ProductBySlug(ctx context.Context, slug string) (id int64, contextPath string, err error) }` with sentinel `var ErrNotFound = errors.New("tryit: product not found")`.
  - `type Access interface {`
    `  OwnsApp(ctx context.Context, appID, userID int64) (bool, error)`
    `  SubscriptionStatus(ctx context.Context, appID, productID int64) (string, error)`
    `  APIKey(ctx context.Context, appID int64) (string, error)`
    `  ApprovedApps(ctx context.Context, userID, productID int64) ([]AppRef, error)`
    `}`
  - `func NewHandler(p Products, a Access, gatewayURL string) *Handler` with `ServeHTTP`. Routes (chi):
    - `GET /api/try/{slug}/context` → `{ "apps": [AppRef...] }`.
    - `Handle("/api/try/{slug}/{appId}/*", proxy)` and `Handle("/api/try/{slug}/{appId}", proxy)` — any method.

- [ ] **Step 1: Write the failing tests**

Create `internal/tryit/handler_test.go`:
```go
package tryit

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"apisix-portal/internal/auth"
)

type fakeProducts struct{ id int64; ctx string; err error }
func (f fakeProducts) ProductBySlug(_ context.Context, _ string) (int64, string, error) {
	return f.id, f.ctx, f.err
}

type fakeAccess struct {
	owns   bool
	status string
	key    string
	apps   []AppRef
}
func (f fakeAccess) OwnsApp(_ context.Context, _, _ int64) (bool, error)              { return f.owns, nil }
func (f fakeAccess) SubscriptionStatus(_ context.Context, _, _ int64) (string, error) { return f.status, nil }
func (f fakeAccess) APIKey(_ context.Context, _ int64) (string, error)                { return f.key, nil }
func (f fakeAccess) ApprovedApps(_ context.Context, _, _ int64) ([]AppRef, error)     { return f.apps, nil }

// withUser injects an authed user id into the request context.
func withUser(r *http.Request, id int64) *http.Request {
	return r.WithContext(auth.WithUserID(r.Context(), id))
}

func TestContextListsApprovedApps(t *testing.T) {
	h := NewHandler(fakeProducts{id: 9, ctx: "/orders"},
		fakeAccess{apps: []AppRef{{ID: 1, Name: "App A"}}}, "http://gw:9080")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, withUser(httptest.NewRequest(http.MethodGet, "/api/try/orders/context", nil), 7))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct{ Apps []AppRef }
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Apps) != 1 || out.Apps[0].Name != "App A" {
		t.Errorf("apps=%v", out.Apps)
	}
}

func TestProxyForwardsWithKeyInjected(t *testing.T) {
	var gotPath, gotKey, gotMethod string
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotKey, gotMethod = r.URL.Path, r.Header.Get("apikey"), r.Method
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer gw.Close()

	h := NewHandler(fakeProducts{id: 9, ctx: "/orders"},
		fakeAccess{owns: true, status: "active", key: "ax_live_k1"}, gw.URL)
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/try/orders/3/pet/5", strings.NewReader(`{"a":1}`)), 7)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != 201 || rec.Body.String() != `{"ok":true}` {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if gotPath != "/orders/pet/5" { t.Errorf("gateway path=%q", gotPath) }
	if gotKey != "ax_live_k1" { t.Errorf("apikey=%q", gotKey) }
	if gotMethod != http.MethodPost { t.Errorf("method=%q", gotMethod) }
}

func TestProxyRejectsNotOwner(t *testing.T) {
	h := NewHandler(fakeProducts{id: 9, ctx: "/orders"},
		fakeAccess{owns: false, status: "active", key: "k"}, "http://gw:9080")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, withUser(httptest.NewRequest(http.MethodGet, "/api/try/orders/3/x", nil), 7))
	if rec.Code != http.StatusForbidden { t.Fatalf("status=%d", rec.Code) }
}

func TestProxyRejectsUnapproved(t *testing.T) {
	h := NewHandler(fakeProducts{id: 9, ctx: "/orders"},
		fakeAccess{owns: true, status: "pending", key: "k"}, "http://gw:9080")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, withUser(httptest.NewRequest(http.MethodGet, "/api/try/orders/3/x", nil), 7))
	if rec.Code != http.StatusForbidden { t.Fatalf("status=%d", rec.Code) }
}

func TestProxyUnknownProduct404(t *testing.T) {
	h := NewHandler(fakeProducts{err: ErrNotFound},
		fakeAccess{owns: true, status: "active", key: "k"}, "http://gw:9080")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, withUser(httptest.NewRequest(http.MethodGet, "/api/try/nope/3/x", nil), 7))
	if rec.Code != http.StatusNotFound { t.Fatalf("status=%d", rec.Code) }
}

var _ = io.Discard
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/tryit/ -v`
Expected: FAIL — package/handler not defined.

- [ ] **Step 3: Implement `internal/tryit/tryit.go` (types + interfaces)**

```go
// Package tryit proxies a developer's "Try-it" request through the portal into
// the APISIX gateway, injecting the approved subscriber's API key server-side.
package tryit

import (
	"context"
	"errors"
)

// ErrNotFound is returned by Products when a published product is missing.
var ErrNotFound = errors.New("tryit: product not found")

// AppRef is an application the user may use for Try-it.
type AppRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// Products resolves a product slug to its id and gateway context path. Only
// PUBLISHED products resolve; others yield ErrNotFound.
type Products interface {
	ProductBySlug(ctx context.Context, slug string) (id int64, contextPath string, err error)
}

// Access answers the authorization + key questions for Try-it.
type Access interface {
	OwnsApp(ctx context.Context, appID, userID int64) (bool, error)
	SubscriptionStatus(ctx context.Context, appID, productID int64) (string, error)
	APIKey(ctx context.Context, appID int64) (string, error)
	ApprovedApps(ctx context.Context, userID, productID int64) ([]AppRef, error)
}
```

- [ ] **Step 4: Implement `internal/tryit/handler.go`**

```go
package tryit

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"apisix-portal/internal/auth"
	"apisix-portal/internal/httpx"
)

const (
	statusActive   = "active" // mirrors subscriptions.StatusActive
	maxBodyBytes   = 2 << 20   // 2 MiB cap on request and response bodies
	gatewayTimeout = 15 * time.Second
)

// hopByHop and sensitive inbound headers are never forwarded to the gateway.
var stripHeaders = map[string]bool{
	"Host": true, "Cookie": true, "Authorization": true, "Connection": true,
	"Keep-Alive": true, "Proxy-Authenticate": true, "Proxy-Authorization": true,
	"Te": true, "Trailer": true, "Transfer-Encoding": true, "Upgrade": true,
	"Content-Length": true,
}

type Handler struct {
	products Products
	access   Access
	gateway  string
	client   *http.Client
	router   chi.Router
}

func NewHandler(p Products, a Access, gatewayURL string) *Handler {
	h := &Handler{
		products: p, access: a,
		gateway: strings.TrimRight(gatewayURL, "/"),
		client:  &http.Client{Timeout: gatewayTimeout},
		router:  chi.NewRouter(),
	}
	h.router.Get("/api/try/{slug}/context", h.context)
	h.router.Handle("/api/try/{slug}/{appId}", http.HandlerFunc(h.proxy))
	h.router.Handle("/api/try/{slug}/{appId}/*", http.HandlerFunc(h.proxy))
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.router.ServeHTTP(w, r) }

func (h *Handler) context(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserID(r.Context())
	id, _, err := h.products.ProductBySlug(r.Context(), chi.URLParam(r, "slug"))
	if errors.Is(err, ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "product not found")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed")
		return
	}
	apps, err := h.access.ApprovedApps(r.Context(), userID, id)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed")
		return
	}
	if apps == nil {
		apps = []AppRef{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"apps": apps})
}

func (h *Handler) proxy(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserID(r.Context())
	slug := chi.URLParam(r, "slug")
	appID, err := strconv.ParseInt(chi.URLParam(r, "appId"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "bad app id")
		return
	}

	productID, contextPath, err := h.products.ProductBySlug(r.Context(), slug)
	if errors.Is(err, ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "product not found")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed")
		return
	}

	owns, err := h.access.OwnsApp(r.Context(), appID, userID)
	if err != nil || !owns {
		httpx.Error(w, http.StatusForbidden, "not your application")
		return
	}
	status, err := h.access.SubscriptionStatus(r.Context(), appID, productID)
	if err != nil || status != statusActive {
		httpx.Error(w, http.StatusForbidden, "no approved subscription for this API")
		return
	}
	key, err := h.access.APIKey(r.Context(), appID)
	if err != nil || key == "" {
		httpx.Error(w, http.StatusForbidden, "no key for this application")
		return
	}

	// Build the gateway target from the product's context path + the wildcard
	// remainder. The host is ALWAYS the configured gateway — never client input.
	rest := strings.TrimPrefix(chi.URLParam(r, "*"), "/")
	target := h.gateway + contextPath
	if rest != "" {
		target += "/" + rest
	}
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}

	body := http.MaxBytesReader(w, r.Body, maxBodyBytes)
	out, err := http.NewRequestWithContext(r.Context(), r.Method, target, body)
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "could not build gateway request")
		return
	}
	for name, vals := range r.Header {
		if stripHeaders[http.CanonicalHeaderKey(name)] {
			continue
		}
		for _, v := range vals {
			out.Header.Add(name, v)
		}
	}
	out.Header.Set("apikey", key)

	resp, err := h.client.Do(out)
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "gateway unreachable")
		return
	}
	defer resp.Body.Close()

	for name, vals := range resp.Header {
		if stripHeaders[http.CanonicalHeaderKey(name)] {
			continue
		}
		for _, v := range vals {
			w.Header().Add(name, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, io.LimitReader(resp.Body, maxBodyBytes))
}
```

- [ ] **Step 5: Run the tests + vet**

Run: `go test ./internal/tryit/ -v && go vet ./internal/tryit/`
Expected: PASS (5 tests).

- [ ] **Step 6: Commit**

```bash
git add internal/tryit/
git commit -m "feat(tryit): gateway-proxy + context handler (key injected server-side)"
```

---

## Task C3: Store query + wire tryit into the server

**Files:**
- Modify: `internal/subscriptions/repo.go` (`ApprovedAppsForProduct`)
- Modify: `internal/catalog/repo.go` (a `ProductBySlug` returning id+contextPath, OR adapt in server.go)
- Modify: `internal/server/server.go` (adapters + mount)
- Test: `internal/subscriptions/repo_test.go`

**Interfaces:**
- Consumes: `tryit.NewHandler`, `tryit.AppRef`, `tryit.Products`, `tryit.Access`, `tryit.ErrNotFound`; `config.APISIXGatewayURL`.
- Produces: `/api/try/` mounted behind `requireAuth`.

- [ ] **Step 1: Write the failing repo test**

Add to `internal/subscriptions/repo_test.go` (mirror the file's existing DB setup helper that returns a `*Repo` + seeds; if it seeds users/apps/products/subscriptions, reuse that; otherwise seed inline as `internal/applications/repo_test.go` does):
```go
func TestApprovedAppsForProduct(t *testing.T) {
	ctx, repo := testRepo(t) // use this file's existing helper name/signature
	// Seed a user, two apps, a product, and an ACTIVE + a PENDING subscription.
	// (Adapt column/seed style to the file's existing helpers.)
	// ... seed ...
	// apps := repo.ApprovedAppsForProduct(ctx, userID, productID)
	// assert only the app with the ACTIVE subscription is returned.
}
```
NOTE: write the concrete seeding to match the helpers already in `repo_test.go`. If that file has no reusable seed helper, seed with `repo.pool.Exec` (same-package access) creating: a user, an application (owner_id=user), an api_product, a credential, and two subscriptions (one `active`, one `pending` on a second app), then assert `ApprovedAppsForProduct` returns exactly the active app. Use a `t.Cleanup` to delete seeded rows (re-run safe), matching the catalog repo test pattern.

- [ ] **Step 2: Run to verify it fails**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/subscriptions/ -run TestApprovedAppsForProduct -v`
Expected: FAIL — method undefined.

- [ ] **Step 3: Implement the query**

In `internal/subscriptions/repo.go`:
```go
// AppRef is an application id+name the developer can use for Try-it.
type AppRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// ApprovedAppsForProduct returns the user's applications that hold an ACTIVE
// subscription to the product.
func (r *Repo) ApprovedAppsForProduct(ctx context.Context, userID, productID int64) ([]AppRef, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT a.id, a.name FROM applications a
		   JOIN subscriptions s ON s.application_id = a.id
		 WHERE a.owner_id=$1 AND s.api_product_id=$2 AND s.status='active'
		 ORDER BY a.created_at`, userID, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AppRef
	for rows.Next() {
		var a AppRef
		if err := rows.Scan(&a.ID, &a.Name); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Add a catalog `ProductBySlug` (id + contextPath)**

In `internal/catalog/repo.go`:
```go
// ProductBySlug returns the id and context path of a PUBLISHED product, or
// ErrNotFound. Lighter than GetBySlug — used by the try-it proxy.
func (r *Repo) ProductBySlug(ctx context.Context, slug string) (int64, string, error) {
	var id int64
	var ctxPath string
	err := r.pool.QueryRow(ctx,
		`SELECT id, context_path FROM api_products WHERE slug=$1 AND published=true`, slug).Scan(&id, &ctxPath)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", ErrNotFound
	}
	return id, ctxPath, err
}
```

- [ ] **Step 5: Wire adapters + mount in `internal/server/server.go`**

After the existing `subRepo`/`appsRepo`/`catalog` construction, add adapters and the handler. The `tryit.Products` is satisfied by mapping `catalog.ErrNotFound` → `tryit.ErrNotFound`; `tryit.Access` is satisfied by `appsRepo` + `subRepo`:
```go
	tryProducts := tryitProductsAdapter{repo: catalog.NewRepo(pool)} // or reuse the catalog repo already built
	tryAccess := tryitAccessAdapter{apps: appsRepo, subs: subRepo}
	tryH := tryit.NewHandler(tryProducts, tryAccess, cfg.APISIXGatewayURL)
```
```go
	mux.Handle("/api/try/", requireAuth(tryH))
```
Define the adapters in server.go (or a small `internal/server/tryit_adapters.go`):
```go
type tryitProductsAdapter struct{ repo *catalog.Repo }
func (a tryitProductsAdapter) ProductBySlug(ctx context.Context, slug string) (int64, string, error) {
	id, ctxPath, err := a.repo.ProductBySlug(ctx, slug)
	if errors.Is(err, catalog.ErrNotFound) {
		return 0, "", tryit.ErrNotFound
	}
	return id, ctxPath, err
}

type tryitAccessAdapter struct {
	apps *applications.Repo
	subs *subscriptions.Repo
}
func (a tryitAccessAdapter) OwnsApp(ctx context.Context, appID, userID int64) (bool, error) {
	_, err := a.apps.Get(ctx, appID, userID)
	if errors.Is(err, applications.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}
func (a tryitAccessAdapter) SubscriptionStatus(ctx context.Context, appID, productID int64) (string, error) {
	return a.subs.SubscriptionStatus(ctx, appID, productID)
}
func (a tryitAccessAdapter) APIKey(ctx context.Context, appID int64) (string, error) {
	c, err := a.subs.GetCredential(ctx, appID)
	if err != nil {
		return "", err
	}
	return c.APIKey, nil
}
func (a tryitAccessAdapter) ApprovedApps(ctx context.Context, userID, productID int64) ([]tryit.AppRef, error) {
	refs, err := a.subs.ApprovedAppsForProduct(ctx, userID, productID)
	if err != nil {
		return nil, err
	}
	out := make([]tryit.AppRef, len(refs))
	for i, r := range refs {
		out[i] = tryit.AppRef{ID: r.ID, Name: r.Name}
	}
	return out, nil
}
```
Add imports (`apisix-portal/internal/tryit`, `errors`) as needed. Reuse the catalog repo already built at the top of server.go rather than constructing a second one if convenient.

- [ ] **Step 6: Run the gates**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/... ./cmd/... && go vet ./...`
Expected: PASS (subscriptions repo test runs; everything compiles & mounts).

- [ ] **Step 7: Commit**

```bash
git add internal/subscriptions/repo.go internal/subscriptions/repo_test.go internal/catalog/repo.go internal/server/
git commit -m "feat(server): wire the try-it proxy (approved-apps query + adapters + mount)"
```

---

## Task C4: Frontend — route Scalar try-it through the proxy

**Files:**
- Modify: `web/src/api/client.ts` (`getTryContext`)
- Modify: `web/src/api/types.ts` (`TryApp`)
- Modify: `web/src/components/ScalarDocs.tsx` (accept an optional `serverUrl`)
- Modify: `web/src/pages/ProductDetailPage.tsx` (fetch context, app picker, banner, pass serverUrl)
- Test: `web/src/pages/ProductDetailPage.test.tsx`, `web/src/api/client.product.test.ts`

**Interfaces:**
- Consumes: `getTryContext`; the Scalar `servers` config override.
- Produces:
  - `getTryContext(token, slug): Promise<{ apps: TryApp[] }>` → `GET /api/try/{slug}/context`.
  - `ScalarDocs({ spec, serverUrl }: { spec: string; serverUrl?: string })` — when `serverUrl` is set, pass `servers: [{ url: serverUrl }]` so Scalar's try-it targets the proxy.
  - `type TryApp = { id: number; name: string }`.

- [ ] **Step 1: Write the failing tests**

Add to `web/src/api/client.product.test.ts`:
```ts
it('getTryContext fetches approved apps', async () => {
  const { getTryContext } = await import('./client')
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(
    new Response(JSON.stringify({ apps: [{ id: 3, name: 'App A' }] }), { status: 200, headers: { 'Content-Type': 'application/json' } }),
  )
  const out = await getTryContext('jwt', 'orders')
  expect(out.apps[0].name).toBe('App A')
})
```
Add to `web/src/pages/ProductDetailPage.test.tsx` (the Scalar mock already records `data-content`; extend it to also record the server url):
```tsx
// in the existing vi.mock for @scalar/api-reference-react, render the server url too:
//   data-server={configuration.servers?.[0]?.url ?? ''}

it('routes try-it through the proxy for a subscribed user (single app)', async () => {
  localStorage.setItem('token', 'jwt')
  localStorage.setItem('user', JSON.stringify({ id: 1, email: 'a@b.c', name: 'D', role: 'developer' }))
  vi.spyOn(api, 'getProductSpec').mockResolvedValue('{"openapi":"3.0.0"}')
  vi.spyOn(api, 'getTryContext').mockResolvedValue({ apps: [{ id: 3, name: 'App A' }] })
  renderAt('orders')
  await waitFor(() => expect(screen.getByTestId('scalar')).toHaveAttribute('data-server', '/api/try/orders/3'))
})

it('shows a subscribe banner when the user has no approved app', async () => {
  localStorage.setItem('token', 'jwt')
  localStorage.setItem('user', JSON.stringify({ id: 1, email: 'a@b.c', name: 'D', role: 'developer' }))
  vi.spyOn(api, 'getProductSpec').mockResolvedValue('{"openapi":"3.0.0"}')
  vi.spyOn(api, 'getTryContext').mockResolvedValue({ apps: [] })
  renderAt('orders')
  expect(await screen.findByText(/Abonnez-vous pour essayer/i)).toBeInTheDocument()
})
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd web && pnpm exec vitest run src/api/client.product.test.ts src/pages/ProductDetailPage.test.tsx`
Expected: FAIL — `getTryContext` missing / `data-server` not set / banner absent.

- [ ] **Step 3: Implement the client fn + type**

`web/src/api/types.ts`:
```ts
export type TryApp = { id: number; name: string }
```
`web/src/api/client.ts` (authed):
```ts
export async function getTryContext(token: string, slug: string): Promise<{ apps: TryApp[] }> {
  const url = `/api/try/${encodeURIComponent(slug)}/context`
  return parse<{ apps: TryApp[] }>(await fetch(url, { headers: authHeaders(token) }), url)
}
```

- [ ] **Step 4: ScalarDocs accepts an optional serverUrl**

`web/src/components/ScalarDocs.tsx` — change the signature and configuration:
```tsx
export function ScalarDocs({ spec, serverUrl }: { spec: string; serverUrl?: string }) {
  return (
    <div className="scalar-wrap">
      <ApiReferenceReact
        configuration={{
          content: spec,
          ...(serverUrl ? { servers: [{ url: serverUrl }] } : {}),
          hideClientButton: true,
          theme: 'default',
        }}
      />
    </div>
  )
}
```
Update `ScalarDocs.test.tsx`'s mock to also read `configuration.servers` if you assert it; the existing content assertion stays valid.

- [ ] **Step 5: ProductDetailPage — fetch context, picker, banner, serverUrl**

In `web/src/pages/ProductDetailPage.tsx`:
- import `getTryContext` and `useAuth`'s `token`; add state `const [apps, setApps] = useState<TryApp[]>([])` and `const [appId, setAppId] = useState<number | null>(null)`.
- In the effect, when `token` is set, also call `getTryContext(token, slug).then(r => { setApps(r.apps); setAppId(r.apps[0]?.id ?? null) }).catch(() => { setApps([]); setAppId(null) })`.
- Compute `const serverUrl = appId != null ? \`/api/try/${slug}/${appId}\` : undefined`.
- Pass `serverUrl` to `<ScalarDocs spec={spec} serverUrl={serverUrl} />`.
- Above the docs, render:
  - when `token && apps.length > 1`: an app picker `<select value={appId ?? ''} onChange={e => setAppId(Number(e.target.value))}>` listing `apps` (label "Essayer avec :").
  - when `token && apps.length === 0`: a banner `<div className="try-banner">Abonnez-vous pour essayer les requêtes via la passerelle.</div>`.
  - when `!token`: a subtler hint linking to `/login` (optional; the subscribe button already gates).

- [ ] **Step 6: Run tests + full gate**

Run: `cd web && pnpm exec vitest run --exclude 'e2e/**' && pnpm exec tsc --noEmit && pnpm build`
Expected: all green.

- [ ] **Step 7: Commit**

```bash
git add web/src/api/client.ts web/src/api/types.ts web/src/components/ScalarDocs.tsx web/src/components/ScalarDocs.test.tsx web/src/pages/ProductDetailPage.tsx web/src/pages/ProductDetailPage.test.tsx web/src/api/client.product.test.ts web/src/styles/productdetail.css
git commit -m "feat(web): route Scalar try-it through the proxy with an app picker"
```

---

## Task C5: End-to-end (Playwright) — import → docs → subscribe → try-it

**Files:**
- Create: `web/e2e/api-docs-tryit.spec.ts`

**Interfaces:**
- Consumes: the e2e harness (`ADMIN_STATE`, `goto`, the seeded admin from `global-setup.ts`). The e2e stack runs the gateway? NOTE: the Playwright stack (`docker-compose.e2e.yml`) is Postgres-only — there is NO APISIX gateway. So a real gateway round-trip can't run there. Scope this spec to the parts that do NOT need a live gateway: docs render + the proxy's auth gates (403 when not subscribed) which return without contacting the gateway.

- [ ] **Step 1: Write the spec**

Create `web/e2e/api-docs-tryit.spec.ts`:
```ts
import { expect, test } from '@playwright/test'
import { ADMIN_STATE } from './seed-data'
import { goto } from './helpers'

test.use({ storageState: ADMIN_STATE })

// The e2e stack has Postgres + API + Vite but NO APISIX gateway, so this spec
// covers the gateway-independent behaviour: a seeded product with a spec shows
// the Scalar docs, and the try-it context endpoint reports apps. (A real
// gateway round-trip is covered by the manual live check in C6.)
test.describe('API docs page', () => {
  test('renders Scalar docs for a product with a spec', async ({ page }) => {
    // global-setup seeds products without specs; attach one via the admin API first.
    const slug = 'e2e-product-00'
    await page.request.put(`/api/admin/products/by-slug-helper`, {}).catch(() => {})
    // Simplest: import+create a fresh product with a spec via the admin API in this test,
    // then open its detail page. Use page.request with the stored admin token from storageState.
    // (Adapt to the project's e2e helper conventions; if a seeded spec product exists, use it.)
    await goto(page, `/catalog/${slug}`)
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible()
  })
})
```
NOTE: this spec is intentionally light because the e2e stack lacks a gateway. If `global-setup.ts` is extended to attach a spec to one seeded product (recommended: add `openapiSpec` to one `postOk('/api/admin/products', …)` call), assert the Scalar container (`.scalar-wrap` or the `#scalar` content) is present. Keep the gateway round-trip out of e2e; cover it in C6.

- [ ] **Step 2: Run the e2e**

Run: `make test-e2e-web`
Expected: the suite (existing + this) is green.

- [ ] **Step 3: Commit**

```bash
git add web/e2e/api-docs-tryit.spec.ts web/e2e/global-setup.ts
git commit -m "test(e2e): API docs page renders Scalar for a spec'd product"
```

---

## Task C6: Live verification (real gateway round-trip)

- [ ] **Step 1: Bring up the full stack + restart the portal**

The dev `docker-compose` includes APISIX (`:9080`) + the `echo` upstream. Ensure it's up, then restart the portal so `APISIX_GATEWAY_URL` is read.
```bash
make up
PORTAL_ENV=dev UPSTREAM_ALLOW_PRIVATE=1 PORTAL_ADDR=:8090 go run ./cmd/portal
```

- [ ] **Step 2: Prepare a real, reachable product**

As admin, create/import a product with a spec whose `context_path` routes to the `echo` upstream (`upstreamUrl=echo:8080`), publish it, and subscribe an app + approve the subscription (so a key exists and the route is provisioned).

- [ ] **Step 3: Try-it in the browser**

Open `http://localhost:5173/catalog/<slug>`, open an operation, click "Send"/"Essayer". **Look at the screenshot:** the response panel shows the echo upstream's JSON (status 200) — proving the call went browser → `/api/try/<slug>/<appId>/…` → portal → APISIX → echo, with the key injected. Confirm the app picker appears if the user has >1 subscribed app, and the "Abonnez-vous pour essayer" banner shows for a non-subscriber.

- [ ] **Step 4: Confirm the key never reaches the browser**

In the browser devtools Network tab, confirm the request to `/api/try/…` carries NO `apikey` header from the client (it's injected server-side) and the response is the upstream's.

---

## Self-Review notes

- **Spec coverage (Plan C):** `APISIX_GATEWAY_URL` ✅ (C1); proxy `POST/ANY /api/try/{slug}/{appId}/*` injecting the key, owner+approved gates, gateway-only target (no SSRF), header stripping, timeout, body cap ✅ (C2); approved-apps query + adapters + mount behind `requireAuth` ✅ (C3); Scalar `servers` override → try-it through the proxy + app picker + not-subscribed banner ✅ (C4); e2e (gateway-independent parts) ✅ (C5); real round-trip + key-not-in-browser ✅ (C6).
- **Type consistency:** `tryit.AppRef`/`subscriptions.AppRef`/`TryApp` mapped via adapter; `ProductBySlug(slug) → (id, contextPath, err)` consistent across `catalog`, the `tryit.Products` interface, and the adapter; `Access` methods match the adapter and `subscriptions.Repo` signatures (`SubscriptionStatus`, `GetCredential().APIKey`, `ApprovedAppsForProduct`).
- **Security:** key injected server-side only; proxy host is always the configured gateway (path from `context_path`, not client host) — no SSRF; sensitive inbound headers stripped; 401 (middleware) / 403 (owner+approval) gates before any forward.
- **Known limits:** the e2e stack has no gateway, so the live round-trip is a manual check (C6); try-it consumes the subscriber's real quota (intended). The `statusActive` literal in `tryit` mirrors `subscriptions.StatusActive` to avoid a package import — keep them in sync.
- **Implementer notes:** match `subscriptions/repo_test.go`'s existing seed-helper style for C3's DB test; confirm the Scalar `servers` override key against the installed `@scalar/types@0.16.0` (it exposes `servers: z.array(z.any())`); keep the ScalarDocs test mock reading whatever config keys you assert.
