# Plan 4a — Admin Auth + Product CRUD Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add admin authorization (role-gated `/api/admin/*`) and full CRUD for API products — including per-product `upstream_url` — closing the Plan-3 gap where seeded products have no real upstream.

**Architecture:** A new `internal/admin` package owns writes to `api_products` behind a `RequireAdmin` middleware; the public `catalog` package keeps its read-only `published=true` view. Route (re)provisioning logic lives in the existing `subscriptions.Service` and is exposed via two new methods (`ReprovisionRoute`, `DeprovisionRoute`) so the admin path triggers gateway changes without duplicating provisioning code. Admin is seeded by promoting a configurable email (`ADMIN_EMAIL`) to role `admin` at startup; login already issues `role` into the JWT.

**Tech Stack:** Go, chi v5 (per-handler routers), pgx v5, golang-jwt v5, the existing `apisix.Gateway` interface + `Fake`, `httpx` JSON/Error helpers.

---

## Context the implementer needs

- **JWT already carries role.** `internal/auth/token.go` defines `Claims{UserID, Email, Role, ...}` and `Issue(uid, email, role)`. `internal/auth/handler.go` login/register already call `h.tok.Issue(u.ID, u.Email, u.Role)`. So once a user's DB `role` is `admin` and they log in, their token's `role` claim is `admin`. The only missing piece is a middleware that checks it.
- **`RequireAuth` currently discards role** (`internal/auth/middleware.go`): it stores only the user id in context. We add role to context and a separate `RequireAdmin`.
- **`subscriptions.status` already exists** (migration `0003_subscribe.sql`, `status TEXT NOT NULL DEFAULT 'active'`). No schema migration is needed in 4a. `ConsumersForProduct` already filters `status='active'`.
- **Handler pattern:** handlers embed a `chi.Router`, register **full** paths (e.g. `h.router.Get("/api/products/{slug}", ...)`), expose `ServeHTTP`, and are mounted in `cmd/portal/main.go` via `http.ServeMux` with both `"/path"` and `"/path/"` entries. Handlers depend on a narrow interface (e.g. `catalog.Lister`) so tests inject fakes.
- **Response helpers:** `httpx.JSON(w, status, v)` and `httpx.Error(w, status, msg)`.
- **Route id / uri convention:** route id is `prod_<productID>`; route uri is `contextPath + "/*"`; upstream is `host:port`. These live in `subscriptions` (currently the unexported `routeID`); this plan exports `RouteID`.
- **Run a single Go test:** `go test ./internal/<pkg>/ -run TestName -v`. Run a package: `go test ./internal/<pkg>/`. Run all: `go test ./internal/... ./cmd/...` (or `make test`).
- **Commit convention:** messages end with the `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>` trailer. Work happens on branch `master` (no remote).

## File structure (what this plan creates / modifies)

- **Modify** `internal/apisix/gateway.go` — add `DeleteRoute` to the `Gateway` interface.
- **Modify** `internal/apisix/fake.go` — implement `DeleteRoute`.
- **Modify** `internal/apisix/client.go` — implement `DeleteRoute` (real Admin API).
- **Modify** `internal/subscriptions/service.go` — export `RouteID`; add `ReprovisionRoute` + `DeprovisionRoute`; refactor `Subscribe`/`Unsubscribe` to use them.
- **Modify** `internal/config/config.go` — add `AdminEmail`.
- **Create** `internal/config/config_test.go` — cover the new default/override.
- **Modify** `internal/auth/middleware.go` — add `WithRole`/`Role`; store role in `RequireAuth`; add `RequireAdmin`.
- **Create** `internal/auth/middleware_test.go` — cover `RequireAdmin` (401/403/200) and `Role` context.
- **Modify** `internal/auth/repo.go` — add `EnsureAdminRole`.
- **Create** `internal/admin/product.go` — `Product` type + validation.
- **Create** `internal/admin/product_test.go` — validation tests.
- **Create** `internal/admin/repo.go` — SQL: ListAll/Get/Create/Update/Delete/CountActiveSubscriptions.
- **Create** `internal/admin/service.go` — `Store` + `Provisioner` interfaces; reprovision-on-upstream-change; block-delete-on-active-subs + teardown.
- **Create** `internal/admin/service_test.go` — service logic with fakes.
- **Create** `internal/admin/handler.go` — chi routes for `/api/admin/products*`.
- **Create** `internal/admin/handler_test.go` — handler status/validation/error mapping with a fake service.
- **Modify** `cmd/portal/main.go` — seed admin role at startup; wire admin handler behind `RequireAdmin`.

---

## Task 1: Add `DeleteRoute` to the gateway

**Files:**
- Modify: `internal/apisix/gateway.go`
- Modify: `internal/apisix/fake.go`
- Modify: `internal/apisix/client.go`
- Test: `internal/apisix/fake_test.go` (create)

- [ ] **Step 1: Write the failing test**

Create `internal/apisix/fake_test.go`:

```go
package apisix

import (
	"context"
	"testing"
)

func TestFakeDeleteRoute(t *testing.T) {
	f := NewFake()
	if err := f.EnsureRoute(context.Background(), "prod_1", "/x/*", "echo:8080", nil); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if err := f.DeleteRoute(context.Background(), "prod_1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok := f.Routes["prod_1"]; ok {
		t.Fatal("route prod_1 still present after DeleteRoute")
	}
	// Deleting a missing route is a no-op, not an error.
	if err := f.DeleteRoute(context.Background(), "prod_missing"); err != nil {
		t.Fatalf("delete missing: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/apisix/ -run TestFakeDeleteRoute -v`
Expected: compile failure — `f.DeleteRoute undefined` (and `Gateway` interface lacks it).

- [ ] **Step 3: Add `DeleteRoute` to the interface**

In `internal/apisix/gateway.go`, add to the `Gateway` interface (after `EnsureRoute`):

```go
	// DeleteRoute removes the route routeID. Deleting a missing route is a no-op.
	DeleteRoute(ctx context.Context, routeID string) error
```

- [ ] **Step 4: Implement on the Fake**

In `internal/apisix/fake.go`, add (after `EnsureRoute`):

```go
func (f *Fake) DeleteRoute(_ context.Context, routeID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.Routes, routeID)
	return nil
}
```

- [ ] **Step 5: Implement on the real Client**

The existing `c.do` helper returns only `error` and treats **any** status ≥300 as an
error — including the `404` APISIX returns when deleting a route that does not
exist. Since `DeleteRoute` must treat a missing route as a no-op, it issues the
request directly (mirroring `do`'s header/auth setup) rather than calling `do`.

In `internal/apisix/client.go`, add (after `EnsureRoute`, before the `var _ Gateway` line):

```go
func (c *Client) DeleteRoute(ctx context.Context, routeID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+"/apisix/admin/routes/"+routeID, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-KEY", c.apiKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// 404 means the route is already gone — treat as success (idempotent delete).
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusNotFound {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("apisix delete route %s: %d %s", routeID, resp.StatusCode, string(msg))
	}
	return nil
}
```

(`io`, `fmt`, and `net/http` are already imported in `client.go`.)

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/apisix/ -v`
Expected: PASS (the `var _ Gateway = (*Fake)(nil)` and `var _ Gateway = (*Client)(nil)` assertions now compile).

- [ ] **Step 7: Commit**

```bash
git add internal/apisix/
git commit -m "feat(apisix): add DeleteRoute to the Gateway interface

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: Export `RouteID` and add reprovision helpers to the subscriptions service

This extracts the route (re)build into reusable methods so the admin path can trigger gateway changes without duplicating provisioning logic.

**Files:**
- Modify: `internal/subscriptions/service.go`
- Test: `internal/subscriptions/service_test.go` (add a test; file already exists)

- [ ] **Step 1: Write the failing test**

Add to `internal/subscriptions/service_test.go`. These use the **existing**
`fakeStore` in that file (fields `products map[int64]ProductInfo` and
`consumers map[int64][]string`, constructed via `newFakeStore()`):

```go
func TestReprovisionRoute(t *testing.T) {
	store := newFakeStore()
	store.products[7] = ProductInfo{ID: 7, ContextPath: "/seven", Upstream: "echo:8080"}
	store.consumers[7] = []string{"app_1", "app_2"}
	gw := apisix.NewFake()
	svc := NewService(store, gw, GenerateKey)

	if err := svc.ReprovisionRoute(context.Background(), 7); err != nil {
		t.Fatalf("reprovision: %v", err)
	}
	r, ok := gw.Routes[RouteID(7)]
	if !ok {
		t.Fatalf("route %s not created", RouteID(7))
	}
	if r.Upstream != "echo:8080" || r.URI != "/seven/*" {
		t.Fatalf("unexpected route: %+v", r)
	}
	if len(r.Allowed) != 2 {
		t.Fatalf("want 2 allowed consumers, got %v", r.Allowed)
	}
}

func TestDeprovisionRoute(t *testing.T) {
	store := newFakeStore()
	store.products[7] = ProductInfo{ID: 7, ContextPath: "/seven", Upstream: "echo:8080"}
	gw := apisix.NewFake()
	_ = gw.EnsureRoute(context.Background(), RouteID(7), "/seven/*", "echo:8080", nil)
	svc := NewService(store, gw, GenerateKey)

	if err := svc.DeprovisionRoute(context.Background(), 7); err != nil {
		t.Fatalf("deprovision: %v", err)
	}
	if _, ok := gw.Routes[RouteID(7)]; ok {
		t.Fatal("route still present after DeprovisionRoute")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/subscriptions/ -run 'TestReprovisionRoute|TestDeprovisionRoute' -v`
Expected: compile failure — `RouteID` and `svc.ReprovisionRoute`/`DeprovisionRoute` undefined.

- [ ] **Step 3: Export `RouteID` and add the helpers**

In `internal/subscriptions/service.go`:

Replace the unexported helper line:

```go
func routeID(productID int64) string  { return fmt.Sprintf("prod_%d", productID) }
```

with an exported one:

```go
// RouteID is the deterministic APISIX route id for a product.
func RouteID(productID int64) string { return fmt.Sprintf("prod_%d", productID) }
```

Add these two methods (place after `NewService`):

```go
// ReprovisionRoute rebuilds the product's APISIX route from its current upstream
// and the set of active subscribers' consumer names. Safe to call repeatedly.
func (s *Service) ReprovisionRoute(ctx context.Context, productID int64) error {
	prod, err := s.store.GetProduct(ctx, productID)
	if err != nil {
		return err
	}
	allowed, err := s.store.ConsumersForProduct(ctx, productID)
	if err != nil {
		return err
	}
	return s.gw.EnsureRoute(ctx, RouteID(prod.ID), prod.ContextPath+"/*", prod.Upstream, allowed)
}

// DeprovisionRoute removes the product's APISIX route entirely.
func (s *Service) DeprovisionRoute(ctx context.Context, productID int64) error {
	return s.gw.DeleteRoute(ctx, RouteID(productID))
}
```

- [ ] **Step 4: Refactor `Subscribe`/`Unsubscribe` to use `ReprovisionRoute` (DRY)**

Replace the existing `Subscribe` method body with:

```go
// Subscribe provisions APISIX and persists the subscription, returning the app's credential.
func (s *Service) Subscribe(ctx context.Context, appID, productID, planID int64) (Credential, error) {
	if _, err := s.store.GetProduct(ctx, productID); err != nil {
		return Credential{}, err
	}
	plan, err := s.store.GetPlan(ctx, planID)
	if err != nil {
		return Credential{}, err
	}
	cred, err := s.store.GetOrCreateCredential(ctx, appID, s.genKey)
	if err != nil {
		return Credential{}, err
	}
	if err := s.gw.EnsureConsumer(ctx, cred.ConsumerUsername, cred.APIKey,
		apisix.RateLimit{Count: plan.Count, WindowSeconds: plan.WindowSeconds}); err != nil {
		return Credential{}, err
	}
	if err := s.store.SaveSubscription(ctx, appID, productID, planID); err != nil {
		return Credential{}, err
	}
	if err := s.ReprovisionRoute(ctx, productID); err != nil {
		return Credential{}, err
	}
	return cred, nil
}
```

Replace the existing `Unsubscribe` method body with:

```go
// Unsubscribe removes the subscription and updates the route whitelist.
func (s *Service) Unsubscribe(ctx context.Context, appID, productID int64) error {
	if err := s.store.DeleteSubscription(ctx, appID, productID); err != nil {
		return err
	}
	return s.ReprovisionRoute(ctx, productID)
}
```

- [ ] **Step 5: Run the full subscriptions package tests**

Run: `go test ./internal/subscriptions/ -v`
Expected: PASS — the two new tests pass and all existing tests still pass (behavior is unchanged; only the internal route id helper name and the route-build call site moved).

- [ ] **Step 6: Commit**

```bash
git add internal/subscriptions/
git commit -m "refactor(subscriptions): export RouteID, add Reprovision/DeprovisionRoute

Subscribe/Unsubscribe now reuse ReprovisionRoute. These methods let the
admin path trigger route changes without duplicating provisioning logic.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: Add `AdminEmail` to config

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go` (create)

- [ ] **Step 1: Write the failing test**

Create `internal/config/config_test.go`:

```go
package config

import (
	"os"
	"testing"
)

func TestAdminEmailDefault(t *testing.T) {
	os.Unsetenv("ADMIN_EMAIL")
	if got := Load().AdminEmail; got != "admin@portal.local" {
		t.Fatalf("default AdminEmail = %q, want admin@portal.local", got)
	}
}

func TestAdminEmailOverride(t *testing.T) {
	t.Setenv("ADMIN_EMAIL", "boss@example.com")
	if got := Load().AdminEmail; got != "boss@example.com" {
		t.Fatalf("AdminEmail = %q, want boss@example.com", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -v`
Expected: compile failure — `Load().AdminEmail` undefined.

- [ ] **Step 3: Add the field and default**

In `internal/config/config.go`, add `AdminEmail string` to the `Config` struct (after `APISIXAdminKey`), and in `Load()` add:

```go
		AdminEmail:     get("ADMIN_EMAIL", "admin@portal.local"),
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat(config): add ADMIN_EMAIL (default admin@portal.local)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: Role in context + `RequireAdmin` middleware

**Files:**
- Modify: `internal/auth/middleware.go`
- Test: `internal/auth/middleware_test.go` (create)

- [ ] **Step 1: Write the failing test**

Create `internal/auth/middleware_test.go`:

```go
package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func adminTestHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(Role(r.Context())))
	})
}

func TestRequireAdminAllowsAdmin(t *testing.T) {
	tk := NewTokenizer("s3cret")
	tok, err := tk.Issue(1, "boss@example.com", "admin")
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/products", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	RequireAdmin(tk)(adminTestHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "admin" {
		t.Fatalf("Role(ctx) = %q, want admin", rec.Body.String())
	}
}

func TestRequireAdminRejectsDeveloper(t *testing.T) {
	tk := NewTokenizer("s3cret")
	tok, _ := tk.Issue(2, "dev@example.com", "developer")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/products", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	RequireAdmin(tk)(adminTestHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestRequireAdminRejectsMissingToken(t *testing.T) {
	tk := NewTokenizer("s3cret")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/products", nil)

	RequireAdmin(tk)(adminTestHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/auth/ -run TestRequireAdmin -v`
Expected: compile failure — `Role` and `RequireAdmin` undefined.

- [ ] **Step 3: Add role context accessors and `RequireAdmin`**

In `internal/auth/middleware.go`:

Add a role key next to the existing `userIDKey`:

```go
const (
	userIDKey ctxKey = 0
	roleKey   ctxKey = 1
)
```

(Replace the existing `const userIDKey ctxKey = 0` line with the block above.)

Add accessors (after `UserID`):

```go
// WithRole returns a context carrying the given role.
func WithRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, roleKey, role)
}

// Role returns the authenticated user's role, or "" if unauthenticated.
func Role(ctx context.Context) string {
	role, _ := ctx.Value(roleKey).(string)
	return role
}
```

In `RequireAuth`, also store the role. Replace the success line:

```go
			next.ServeHTTP(w, r.WithContext(WithUserID(r.Context(), claims.UserID)))
```

with:

```go
			ctx := WithUserID(r.Context(), claims.UserID)
			ctx = WithRole(ctx, claims.Role)
			next.ServeHTTP(w, r.WithContext(ctx))
```

Add `RequireAdmin` (after `RequireAuth`):

```go
// RequireAdmin returns middleware that requires a valid Bearer JWT whose role
// claim is "admin". It stores the user id and role in the request context.
func RequireAdmin(tk *Tokenizer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			if !strings.HasPrefix(h, "Bearer ") {
				httpx.Error(w, http.StatusUnauthorized, "missing bearer token")
				return
			}
			claims, err := tk.Parse(strings.TrimPrefix(h, "Bearer "))
			if err != nil {
				httpx.Error(w, http.StatusUnauthorized, "invalid token")
				return
			}
			if claims.Role != "admin" {
				httpx.Error(w, http.StatusForbidden, "admin only")
				return
			}
			ctx := WithUserID(r.Context(), claims.UserID)
			ctx = WithRole(ctx, claims.Role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/auth/ -v`
Expected: PASS (new middleware tests + existing auth tests).

- [ ] **Step 5: Commit**

```bash
git add internal/auth/middleware.go internal/auth/middleware_test.go
git commit -m "feat(auth): role in request context + RequireAdmin middleware

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: `EnsureAdminRole` repo method

This is a thin SQL method (like the other `auth.Repo` methods); it is exercised at startup and by the integration smoke, not by a hermetic unit test.

**Files:**
- Modify: `internal/auth/repo.go`

- [ ] **Step 1: Add the method**

In `internal/auth/repo.go`, add (after `GetByID`):

```go
// EnsureAdminRole promotes the user with the given email to role 'admin'.
// Idempotent and a no-op if no such user exists yet (e.g. before first register).
func (r *Repo) EnsureAdminRole(ctx context.Context, email string) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET role='admin' WHERE email=$1`, email)
	return err
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/auth/`
Expected: no output (success).

- [ ] **Step 3: Commit**

```bash
git add internal/auth/repo.go
git commit -m "feat(auth): EnsureAdminRole to promote a seeded admin by email

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 6: Admin `Product` type + validation

**Files:**
- Create: `internal/admin/product.go`
- Test: `internal/admin/product_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/admin/product_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/admin/ -run TestProductValidate -v`
Expected: failure — package/`Product` does not exist.

- [ ] **Step 3: Write the type + validation**

Create `internal/admin/product.go`:

```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/admin/ -run TestProductValidate -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/admin/product.go internal/admin/product_test.go
git commit -m "feat(admin): Product type + validation

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 7: Admin `Repo` (SQL)

SQL methods follow the existing repo pattern (thin, covered by build + the integration smoke; the interesting logic is unit-tested at the service layer in Task 8).

**Files:**
- Create: `internal/admin/repo.go`

- [ ] **Step 1: Write the repo**

Create `internal/admin/repo.go`:

```go
package admin

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a product id does not exist.
var ErrNotFound = errors.New("admin: product not found")

// ErrSlugTaken is returned when a create/update would duplicate a slug.
var ErrSlugTaken = errors.New("admin: slug already exists")

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

const productCols = `id, name, slug, category, version, context_path, description, tags, icon, upstream_url, published`

func scanProduct(row pgx.Row) (Product, error) {
	var p Product
	err := row.Scan(&p.ID, &p.Name, &p.Slug, &p.Category, &p.Version,
		&p.ContextPath, &p.Description, &p.Tags, &p.Icon, &p.UpstreamURL, &p.Published)
	return p, err
}

func (r *Repo) ListAll(ctx context.Context) ([]Product, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+productCols+` FROM api_products ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Product
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repo) Get(ctx context.Context, id int64) (Product, error) {
	p, err := scanProduct(r.pool.QueryRow(ctx, `SELECT `+productCols+` FROM api_products WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Product{}, ErrNotFound
	}
	return p, err
}

func (r *Repo) Create(ctx context.Context, p Product) (Product, error) {
	created, err := scanProduct(r.pool.QueryRow(ctx,
		`INSERT INTO api_products(name, slug, category, version, context_path, description, tags, icon, upstream_url, published)
		 VALUES($1,$2,$3,COALESCE(NULLIF($4,''),'1.0.0'),$5,$6,$7,$8,$9,$10)
		 RETURNING `+productCols,
		p.Name, p.Slug, p.Category, p.Version, p.ContextPath, p.Description, p.Tags, p.Icon, p.UpstreamURL, p.Published))
	if isUniqueViolation(err) {
		return Product{}, ErrSlugTaken
	}
	return created, err
}

func (r *Repo) Update(ctx context.Context, p Product) (Product, error) {
	updated, err := scanProduct(r.pool.QueryRow(ctx,
		`UPDATE api_products SET name=$2, slug=$3, category=$4, version=COALESCE(NULLIF($5,''),'1.0.0'),
		   context_path=$6, description=$7, tags=$8, icon=$9, upstream_url=$10, published=$11
		 WHERE id=$1
		 RETURNING `+productCols,
		p.ID, p.Name, p.Slug, p.Category, p.Version, p.ContextPath, p.Description, p.Tags, p.Icon, p.UpstreamURL, p.Published))
	if errors.Is(err, pgx.ErrNoRows) {
		return Product{}, ErrNotFound
	}
	if isUniqueViolation(err) {
		return Product{}, ErrSlugTaken
	}
	return updated, err
}

func (r *Repo) Delete(ctx context.Context, id int64) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM api_products WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repo) CountActiveSubscriptions(ctx context.Context, productID int64) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM subscriptions WHERE api_product_id=$1 AND status='active'`, productID,
	).Scan(&n)
	return n, err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
```

> **Note on `tags`:** the column is `TEXT[]`. pgx v5 scans a Postgres text array into `[]string` and binds a `[]string` parameter to `TEXT[]` directly (the catalog repo already relies on this). When the JSON body omits `tags`, the handler defaults it to an empty slice (Task 9) so the bind is `{}`, never SQL NULL.

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/admin/`
Expected: no output (success).

- [ ] **Step 3: Commit**

```bash
git add internal/admin/repo.go
git commit -m "feat(admin): product repo (CRUD + active-subscription count)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 8: Admin `Service` (reprovision-on-change, block-delete-on-subs)

**Files:**
- Create: `internal/admin/service.go`
- Test: `internal/admin/service_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/admin/service_test.go`:

```go
package admin

import (
	"context"
	"errors"
	"testing"
)

type fakeStore struct {
	products map[int64]Product
	counts   map[int64]int
	deleted  map[int64]bool
}

func newFakeStore() *fakeStore {
	return &fakeStore{products: map[int64]Product{}, counts: map[int64]int{}, deleted: map[int64]bool{}}
}

func (f *fakeStore) ListAll(_ context.Context) ([]Product, error) {
	out := []Product{}
	for _, p := range f.products {
		out = append(out, p)
	}
	return out, nil
}
func (f *fakeStore) Get(_ context.Context, id int64) (Product, error) {
	p, ok := f.products[id]
	if !ok {
		return Product{}, ErrNotFound
	}
	return p, nil
}
func (f *fakeStore) Create(_ context.Context, p Product) (Product, error) {
	p.ID = int64(len(f.products) + 1)
	f.products[p.ID] = p
	return p, nil
}
func (f *fakeStore) Update(_ context.Context, p Product) (Product, error) {
	if _, ok := f.products[p.ID]; !ok {
		return Product{}, ErrNotFound
	}
	f.products[p.ID] = p
	return p, nil
}
func (f *fakeStore) Delete(_ context.Context, id int64) error {
	if _, ok := f.products[id]; !ok {
		return ErrNotFound
	}
	delete(f.products, id)
	f.deleted[id] = true
	return nil
}
func (f *fakeStore) CountActiveSubscriptions(_ context.Context, id int64) (int, error) {
	return f.counts[id], nil
}

type fakeProv struct {
	reprovisioned []int64
	deprovisioned []int64
}

func (f *fakeProv) ReprovisionRoute(_ context.Context, id int64) error {
	f.reprovisioned = append(f.reprovisioned, id)
	return nil
}
func (f *fakeProv) DeprovisionRoute(_ context.Context, id int64) error {
	f.deprovisioned = append(f.deprovisioned, id)
	return nil
}

func TestUpdateReprovisionsWhenUpstreamChangesAndHasSubs(t *testing.T) {
	store := newFakeStore()
	store.products[1] = Product{ID: 1, Name: "P", Slug: "p", Category: "C", ContextPath: "/p", UpstreamURL: "old:8080"}
	store.counts[1] = 2
	prov := &fakeProv{}
	svc := NewService(store, prov)

	updated := store.products[1]
	updated.UpstreamURL = "new:9090"
	if _, err := svc.Update(context.Background(), updated); err != nil {
		t.Fatal(err)
	}
	if len(prov.reprovisioned) != 1 || prov.reprovisioned[0] != 1 {
		t.Fatalf("expected reprovision of product 1, got %v", prov.reprovisioned)
	}
}

func TestUpdateNoReprovisionWhenUpstreamUnchanged(t *testing.T) {
	store := newFakeStore()
	store.products[1] = Product{ID: 1, Name: "P", Slug: "p", Category: "C", ContextPath: "/p", UpstreamURL: "same:8080"}
	store.counts[1] = 5
	prov := &fakeProv{}
	svc := NewService(store, prov)

	updated := store.products[1]
	updated.Description = "changed text only"
	if _, err := svc.Update(context.Background(), updated); err != nil {
		t.Fatal(err)
	}
	if len(prov.reprovisioned) != 0 {
		t.Fatalf("expected no reprovision, got %v", prov.reprovisioned)
	}
}

func TestUpdateNoReprovisionWhenNoSubs(t *testing.T) {
	store := newFakeStore()
	store.products[1] = Product{ID: 1, Name: "P", Slug: "p", Category: "C", ContextPath: "/p", UpstreamURL: "old:8080"}
	store.counts[1] = 0
	prov := &fakeProv{}
	svc := NewService(store, prov)

	updated := store.products[1]
	updated.UpstreamURL = "new:9090"
	if _, err := svc.Update(context.Background(), updated); err != nil {
		t.Fatal(err)
	}
	if len(prov.reprovisioned) != 0 {
		t.Fatalf("expected no reprovision (no active subs), got %v", prov.reprovisioned)
	}
}

func TestDeleteBlockedByActiveSubs(t *testing.T) {
	store := newFakeStore()
	store.products[1] = Product{ID: 1, Name: "P", Slug: "p", Category: "C", ContextPath: "/p"}
	store.counts[1] = 1
	prov := &fakeProv{}
	svc := NewService(store, prov)

	err := svc.Delete(context.Background(), 1)
	if !errors.Is(err, ErrHasSubscriptions) {
		t.Fatalf("err = %v, want ErrHasSubscriptions", err)
	}
	if store.deleted[1] {
		t.Fatal("product should not have been deleted")
	}
	if len(prov.deprovisioned) != 0 {
		t.Fatalf("should not deprovision a blocked delete, got %v", prov.deprovisioned)
	}
}

func TestDeleteTearsDownRouteWhenNoSubs(t *testing.T) {
	store := newFakeStore()
	store.products[1] = Product{ID: 1, Name: "P", Slug: "p", Category: "C", ContextPath: "/p"}
	store.counts[1] = 0
	prov := &fakeProv{}
	svc := NewService(store, prov)

	if err := svc.Delete(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if !store.deleted[1] {
		t.Fatal("product should have been deleted")
	}
	if len(prov.deprovisioned) != 1 || prov.deprovisioned[0] != 1 {
		t.Fatalf("expected deprovision of product 1, got %v", prov.deprovisioned)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/admin/ -run 'TestUpdate|TestDelete' -v`
Expected: compile failure — `NewService`, `Service`, `ErrHasSubscriptions` undefined.

- [ ] **Step 3: Write the service**

Create `internal/admin/service.go`:

```go
package admin

import (
	"context"
	"errors"
)

// ErrHasSubscriptions is returned when a product cannot be deleted because it
// still has active subscriptions.
var ErrHasSubscriptions = errors.New("admin: product has active subscriptions")

// Store is the persistence surface the service needs (satisfied by *Repo).
type Store interface {
	ListAll(ctx context.Context) ([]Product, error)
	Get(ctx context.Context, id int64) (Product, error)
	Create(ctx context.Context, p Product) (Product, error)
	Update(ctx context.Context, p Product) (Product, error)
	Delete(ctx context.Context, id int64) error
	CountActiveSubscriptions(ctx context.Context, productID int64) (int, error)
}

// Provisioner triggers APISIX route changes (satisfied by *subscriptions.Service).
type Provisioner interface {
	ReprovisionRoute(ctx context.Context, productID int64) error
	DeprovisionRoute(ctx context.Context, productID int64) error
}

// Service applies admin product operations and keeps APISIX in sync.
type Service struct {
	store Store
	prov  Provisioner
}

func NewService(store Store, prov Provisioner) *Service {
	return &Service{store: store, prov: prov}
}

func (s *Service) List(ctx context.Context) ([]Product, error) { return s.store.ListAll(ctx) }
func (s *Service) Get(ctx context.Context, id int64) (Product, error) { return s.store.Get(ctx, id) }
func (s *Service) Create(ctx context.Context, p Product) (Product, error) { return s.store.Create(ctx, p) }

// Update persists changes and, when the upstream changed on a product that has
// active subscriptions, rebuilds its APISIX route so the new upstream takes effect.
func (s *Service) Update(ctx context.Context, p Product) (Product, error) {
	old, err := s.store.Get(ctx, p.ID)
	if err != nil {
		return Product{}, err
	}
	updated, err := s.store.Update(ctx, p)
	if err != nil {
		return Product{}, err
	}
	if updated.UpstreamURL != old.UpstreamURL {
		n, err := s.store.CountActiveSubscriptions(ctx, p.ID)
		if err != nil {
			return Product{}, err
		}
		if n > 0 {
			if err := s.prov.ReprovisionRoute(ctx, p.ID); err != nil {
				return Product{}, err
			}
		}
	}
	return updated, nil
}

// Delete refuses (ErrHasSubscriptions) while active subscriptions exist; otherwise
// it removes the product and tears down its APISIX route (best effort).
func (s *Service) Delete(ctx context.Context, id int64) error {
	n, err := s.store.CountActiveSubscriptions(ctx, id)
	if err != nil {
		return err
	}
	if n > 0 {
		return ErrHasSubscriptions
	}
	if err := s.store.Delete(ctx, id); err != nil {
		return err
	}
	// Best effort: the row is already gone; a stale gateway route is harmless and
	// will be overwritten if the id is ever reused.
	_ = s.prov.DeprovisionRoute(ctx, id)
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/admin/ -v`
Expected: PASS (validation + service tests).

- [ ] **Step 5: Commit**

```bash
git add internal/admin/service.go internal/admin/service_test.go
git commit -m "feat(admin): product service (reprovision on upstream change, block delete on active subs)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 9: Admin `Handler` (`/api/admin/products*`)

**Files:**
- Create: `internal/admin/handler.go`
- Test: `internal/admin/handler_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/admin/handler_test.go`:

```go
package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeService implements ProductService for handler tests.
type fakeService struct {
	products  map[int64]Product
	createErr error
	updateErr error
	deleteErr error
}

func (f *fakeService) List(_ context.Context) ([]Product, error) {
	out := []Product{}
	for _, p := range f.products {
		out = append(out, p)
	}
	return out, nil
}
func (f *fakeService) Get(_ context.Context, id int64) (Product, error) {
	p, ok := f.products[id]
	if !ok {
		return Product{}, ErrNotFound
	}
	return p, nil
}
func (f *fakeService) Create(_ context.Context, p Product) (Product, error) {
	if f.createErr != nil {
		return Product{}, f.createErr
	}
	p.ID = 1
	return p, nil
}
func (f *fakeService) Update(_ context.Context, p Product) (Product, error) {
	if f.updateErr != nil {
		return Product{}, f.updateErr
	}
	return p, nil
}
func (f *fakeService) Delete(_ context.Context, id int64) error { return f.deleteErr }

func newTestHandler(svc ProductService) *Handler { return NewHandler(svc) }

func do(h *Handler, method, target string, body any) *httptest.ResponseRecorder {
	var rdr *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, target, rdr)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestCreateValid(t *testing.T) {
	h := newTestHandler(&fakeService{products: map[int64]Product{}})
	rec := do(h, http.MethodPost, "/api/admin/products",
		Product{Name: "Pizza", Slug: "pizza", Category: "Food", ContextPath: "/pizza"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestCreateInvalidReturns400(t *testing.T) {
	h := newTestHandler(&fakeService{products: map[int64]Product{}})
	rec := do(h, http.MethodPost, "/api/admin/products", Product{Slug: "x", Category: "c", ContextPath: "/x"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestList(t *testing.T) {
	h := newTestHandler(&fakeService{products: map[int64]Product{1: {ID: 1, Name: "A"}}})
	rec := do(h, http.MethodGet, "/api/admin/products", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got []Product
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body not a JSON array: %v", err)
	}
}

func TestGetUnknownReturns404(t *testing.T) {
	h := newTestHandler(&fakeService{products: map[int64]Product{}})
	rec := do(h, http.MethodGet, "/api/admin/products/99", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestDeleteWithActiveSubsReturns409(t *testing.T) {
	h := newTestHandler(&fakeService{products: map[int64]Product{1: {ID: 1}}, deleteErr: ErrHasSubscriptions})
	rec := do(h, http.MethodDelete, "/api/admin/products/1", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
}

func TestUpdateSlugTakenReturns409(t *testing.T) {
	h := newTestHandler(&fakeService{products: map[int64]Product{1: {ID: 1}}, updateErr: ErrSlugTaken})
	rec := do(h, http.MethodPut, "/api/admin/products/1",
		Product{Name: "Pizza", Slug: "dup", Category: "Food", ContextPath: "/pizza"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
}

func TestDeleteSuccessReturns204(t *testing.T) {
	h := newTestHandler(&fakeService{products: map[int64]Product{1: {ID: 1}}})
	rec := do(h, http.MethodDelete, "/api/admin/products/1", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/admin/ -run 'TestCreate|TestList|TestGet|TestDelete|TestUpdate' -v`
Expected: compile failure — `Handler`, `NewHandler`, `ProductService` undefined.

- [ ] **Step 3: Write the handler**

Create `internal/admin/handler.go`:

```go
package admin

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"apisix-portal/internal/httpx"
)

// ProductService is the surface the handler needs (satisfied by *Service).
type ProductService interface {
	List(ctx context.Context) ([]Product, error)
	Get(ctx context.Context, id int64) (Product, error)
	Create(ctx context.Context, p Product) (Product, error)
	Update(ctx context.Context, p Product) (Product, error)
	Delete(ctx context.Context, id int64) error
}

type Handler struct {
	svc    ProductService
	router chi.Router
}

func NewHandler(svc ProductService) *Handler {
	h := &Handler{svc: svc, router: chi.NewRouter()}
	h.router.Get("/api/admin/products", h.list)
	h.router.Post("/api/admin/products", h.create)
	h.router.Get("/api/admin/products/{id}", h.get)
	h.router.Put("/api/admin/products/{id}", h.update)
	h.router.Delete("/api/admin/products/{id}", h.delete)
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.router.ServeHTTP(w, r) }

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List(r.Context())
	if err != nil {
		log.Printf("admin list products: %v", err)
		httpx.Error(w, http.StatusInternalServerError, "failed to list products")
		return
	}
	if items == nil {
		items = []Product{}
	}
	httpx.JSON(w, http.StatusOK, items)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	p, err := h.svc.Get(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "product not found")
		return
	}
	if err != nil {
		log.Printf("admin get product %d: %v", id, err)
		httpx.Error(w, http.StatusInternalServerError, "failed to load product")
		return
	}
	httpx.JSON(w, http.StatusOK, p)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	p, ok := decodeProduct(w, r)
	if !ok {
		return
	}
	created, err := h.svc.Create(r.Context(), p)
	if errors.Is(err, ErrSlugTaken) {
		httpx.Error(w, http.StatusConflict, "slug already exists")
		return
	}
	if err != nil {
		log.Printf("admin create product: %v", err)
		httpx.Error(w, http.StatusInternalServerError, "failed to create product")
		return
	}
	httpx.JSON(w, http.StatusCreated, created)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	p, ok := decodeProduct(w, r)
	if !ok {
		return
	}
	p.ID = id
	updated, err := h.svc.Update(r.Context(), p)
	if errors.Is(err, ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "product not found")
		return
	}
	if errors.Is(err, ErrSlugTaken) {
		httpx.Error(w, http.StatusConflict, "slug already exists")
		return
	}
	if err != nil {
		log.Printf("admin update product %d: %v", id, err)
		httpx.Error(w, http.StatusInternalServerError, "failed to update product")
		return
	}
	httpx.JSON(w, http.StatusOK, updated)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	err := h.svc.Delete(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "product not found")
		return
	}
	if errors.Is(err, ErrHasSubscriptions) {
		httpx.Error(w, http.StatusConflict, "product has active subscriptions")
		return
	}
	if err != nil {
		log.Printf("admin delete product %d: %v", id, err)
		httpx.Error(w, http.StatusInternalServerError, "failed to delete product")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "bad product id")
		return 0, false
	}
	return id, true
}

func decodeProduct(w http.ResponseWriter, r *http.Request) (Product, bool) {
	var p Product
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid body")
		return Product{}, false
	}
	if p.Tags == nil {
		p.Tags = []string{}
	}
	if msg := p.validate(); msg != "" {
		httpx.Error(w, http.StatusBadRequest, msg)
		return Product{}, false
	}
	return p, true
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/admin/ -v`
Expected: PASS (validation + service + handler tests).

- [ ] **Step 5: Commit**

```bash
git add internal/admin/handler.go internal/admin/handler_test.go
git commit -m "feat(admin): product CRUD handler (/api/admin/products)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 10: Wire admin into `cmd/portal/main.go`

**Files:**
- Modify: `cmd/portal/main.go`

- [ ] **Step 1: Add the admin import**

In the import block of `cmd/portal/main.go`, add (keep imports alphabetically grouped as they are):

```go
	"apisix-portal/internal/admin"
```

- [ ] **Step 2: Reuse the auth repo and seed the admin role**

Replace this line:

```go
	authH := auth.NewHandler(auth.NewRepo(pool), tok)
```

with:

```go
	authRepo := auth.NewRepo(pool)
	authH := auth.NewHandler(authRepo, tok)
	if err := authRepo.EnsureAdminRole(ctx, cfg.AdminEmail); err != nil {
		log.Printf("seed admin role (%s): %v", cfg.AdminEmail, err)
	}
```

- [ ] **Step 3: Construct the admin handler and admin middleware**

After the `subH := subscriptions.NewHandler(...)` line, add:

```go
	adminSvc := admin.NewService(admin.NewRepo(pool), subSvc)
	adminH := admin.NewHandler(adminSvc)
```

After the `requireAuth := auth.RequireAuth(tok)` line, add:

```go
	requireAdmin := auth.RequireAdmin(tok)
```

- [ ] **Step 4: Mount the admin routes**

After the existing `mux.Handle("/api/applications/", requireAuth(subH))` line, add:

```go
	mux.Handle("/api/admin/products", requireAdmin(adminH))
	mux.Handle("/api/admin/products/", requireAdmin(adminH))
```

- [ ] **Step 5: Verify the whole module builds and all tests pass**

Run: `go build ./... && go test ./internal/... ./cmd/...`
Expected: build succeeds; all packages pass (`admin`, `apisix`, `applications`, `auth`, `catalog`, `config`, `plans`, `subscriptions`).

> **Note:** `subSvc` (a `*subscriptions.Service`) is passed where `admin.Provisioner` is expected. This compiles only if `subscriptions.Service` has both `ReprovisionRoute` and `DeprovisionRoute` (Task 2). If the build complains that `*subscriptions.Service` does not implement `admin.Provisioner`, re-check Task 2.

- [ ] **Step 6: Commit**

```bash
git add cmd/portal/main.go
git commit -m "feat(admin): wire product admin behind RequireAdmin + seed admin role

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 11: Live smoke (optional, requires running stack)

This is a manual end-to-end check, not an automated test. Skip if the Docker stack / Postgres is not running; the hermetic tests above cover the logic.

- [ ] **Step 1: Bring up the stack and the portal**

```bash
make up        # docker compose: postgres, etcd, apisix
make run       # go run ./cmd/portal  (runs migrations, seeds admin role)
```

- [ ] **Step 2: Register the admin email, then restart to pick up the role**

```bash
curl -s localhost:8080/api/auth/register \
  -H 'content-type: application/json' \
  -d '{"email":"admin@portal.local","password":"adminpass","name":"Admin"}'
```

Restart `make run` so `EnsureAdminRole` promotes the freshly-registered user (or run the `UPDATE users SET role='admin'` once), then log in to get an admin token:

```bash
TOKEN=$(curl -s localhost:8080/api/auth/login \
  -H 'content-type: application/json' \
  -d '{"email":"admin@portal.local","password":"adminpass"}' | jq -r .token)
```

- [ ] **Step 3: Create a product with a real upstream and confirm 401 vs 200/201**

```bash
# Non-admin / no token → 401
curl -s -o /dev/null -w '%{http_code}\n' localhost:8080/api/admin/products   # 401

# Admin create → 201
curl -s -w '\n%{http_code}\n' localhost:8080/api/admin/products \
  -H "authorization: Bearer $TOKEN" -H 'content-type: application/json' \
  -d '{"name":"Echo","slug":"echo","category":"Demo","contextPath":"/echo","upstreamUrl":"echo:8080","published":true}'

# List includes it → 200
curl -s -w '\n%{http_code}\n' localhost:8080/api/admin/products -H "authorization: Bearer $TOKEN"
```

Expected: `401` for the anonymous call; `201` then `200` for the admin calls.

---

## Self-review notes (already applied)

- **Spec coverage:** 4a covers admin auth (Tasks 4–5, 10), product CRUD incl. `upstream_url` and `published` (Tasks 6–9), reprovision-on-upstream-change (Tasks 2, 8), block-delete-on-active-subs + route teardown (Tasks 1, 2, 8), seeded admin (Tasks 3, 5, 10). Plans/approval/UI are out of scope for 4a (later sub-plans).
- **Spec correction:** the spec's migration `0006` (promote admin) is implemented as a **startup `UPDATE`** (`EnsureAdminRole`) instead of a static SQL migration, because a `.sql` file cannot read the `ADMIN_EMAIL` env var. The spec's migration `0007` (subscription status) is **not needed** — the `status` column already exists from migration `0003`. These two corrections will be reflected when writing the 4c plan.
- **Type consistency:** `RouteID` (exported) is used identically in Tasks 2 and the subscriptions methods; `Product` fields are identical across `product.go`, `repo.go`, `service.go`, `handler.go`; `Store`/`Provisioner`/`ProductService` method sets match their implementations and the fakes.
- **No placeholders:** every code step contains complete, compilable code and exact run commands with expected results.
