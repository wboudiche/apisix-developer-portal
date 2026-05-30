# APISIX Developer Portal — Core Subscribe Loop (Plan 3 of 4, backend)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the portal's promise real — a logged-in developer creates an Application, subscribes it to an API at a plan, and receives an **API key that actually works against the live APISIX gateway** (and is rate-limited to the plan; calling an un-subscribed API is rejected).

**Architecture:** New Go packages on top of the Plan-1 foundation. JWT middleware protects write endpoints. A `subscriptions` service orchestrates provisioning through an `apisix.Gateway` interface (concrete Admin-API client + an in-memory fake for unit tests). Provisioning model: one APISIX **consumer per Application** (carries `key-auth` key + `limit-count` for its plan), one APISIX **route per published Product** (`key-auth` + `consumer-restriction` whitelist); subscribing adds the app's consumer to the product route's whitelist. A hermetic integration test (gated by `RUN_APISIX_IT=1`) provisions against the real compose APISIX with an echo upstream and asserts 200-then-429.

**Tech Stack:** same as Plan 1 (Go 1.25, chi, pgx, jwt) + the APISIX Admin API. Compose gains a `mendhak/http-https-echo` upstream for the integration test.

This is Plan 3 of 4 (Foundation ✓ → Frontend ✓ → **Core subscribe loop** → Admin). The frontend subscribe UI (SubscribeModal, Applications page) is a separate follow-on (Plan 3b) and is NOT in this plan — this plan delivers and tests the whole loop via the HTTP API. Spec: `docs/superpowers/specs/2026-05-29-apisix-developer-portal-design.md`.

---

## Provisioning model (reference for all tasks)

APISIX Admin API base (from host) `http://localhost:19180`, header `X-API-KEY: edd1c9f034335f136f87ad84b625c8f1`. From inside compose it's `http://apisix:9180`. The backend uses `cfg.APISIXAdminURL` / `cfg.APISIXAdminKey`.

- **Consumer** (per application) — `PUT /apisix/admin/consumers`:
  ```json
  { "username": "app_42",
    "plugins": {
      "key-auth": { "key": "<generated-api-key>" },
      "limit-count": { "count": 1000, "time_window": 60, "rejected_code": 429,
                       "key_type": "var", "key": "consumer_name", "policy": "local" } } }
  ```
- **Route** (per published product) — `PUT /apisix/admin/routes/{routeID}`:
  ```json
  { "uri": "/pizzashack/*",
    "upstream": { "type": "roundrobin", "nodes": { "echo:8080": 1 } },
    "plugins": {
      "key-auth": {},
      "consumer-restriction": { "type": "consumer_name", "whitelist": ["app_42"] } } }
  ```
  `routeID` is a stable string derived from the product (use the product id, e.g. `"prod_3"`). When no consumer is subscribed yet the whitelist is `[]` (deny-all).
- **Call**: `curl http://localhost:9080/pizzashack/anything -H "apikey: <key>"` → key-auth resolves the consumer → consumer-restriction checks membership → limit-count enforces the plan → echo upstream returns 200; beyond the limit → 429; a key not in the whitelist → 403; no key → 401.

---

## File structure (created by this plan)

```
internal/
├── auth/middleware.go              # RequireAuth (Bearer→claims in ctx) + UserID(ctx)
├── auth/middleware_test.go
├── plans/plan.go, repo.go, handler.go, *_test.go     # GET /api/plans
├── applications/app.go, repo.go, handler.go, *_test.go  # apps CRUD (protected)
├── apisix/gateway.go               # Gateway interface + RateLimit type
├── apisix/client.go                # Admin-API implementation
├── apisix/fake.go                  # in-memory fake (test double)
├── apisix/client_it_test.go        # integration test (gated)
├── subscriptions/service.go        # Subscribe/Unsubscribe orchestration
├── subscriptions/service_test.go   # against the fake gateway
├── subscriptions/repo.go           # subscriptions + credentials persistence
├── subscriptions/handler.go        # POST/GET/DELETE under /api/applications/{id}/...
└── subscriptions/handler_test.go
internal/db/migrations/0003_subscribe.sql   # applications, subscriptions, credentials, + product.apisix_route_id
docker-compose.yml                  # + echo upstream service
cmd/portal/main.go                  # wire plans/apps/subscriptions + gateway client
```

---

## Task 1: JWT auth middleware (TDD)

**Files:** `internal/auth/middleware.go`, `internal/auth/middleware_test.go`

- [ ] **Step 1: Write `internal/auth/middleware_test.go`**

```go
package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireAuthRejectsMissingToken(t *testing.T) {
	tk := NewTokenizer("s")
	h := RequireAuth(tk)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token: status=%d want 401", rec.Code)
	}
}

func TestRequireAuthAllowsValidTokenAndExposesUserID(t *testing.T) {
	tk := NewTokenizer("s")
	token, _ := tk.Issue(7, "a@b.c", "developer")
	var seen int64
	h := RequireAuth(tk)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = UserID(r.Context())
		w.WriteHeader(200)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 || seen != 7 {
		t.Fatalf("valid token: status=%d userID=%d want 200/7", rec.Code, seen)
	}
}

func TestRequireAuthRejectsBadToken(t *testing.T) {
	tk := NewTokenizer("s")
	h := RequireAuth(tk)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer not.a.jwt")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad token: status=%d want 401", rec.Code)
	}
}
```

- [ ] **Step 2: Run → fails** (`go test ./internal/auth/ -run TestRequireAuth -v`).

- [ ] **Step 3: Write `internal/auth/middleware.go`**

```go
package auth

import (
	"context"
	"net/http"
	"strings"

	"apisix-portal/internal/httpx"
)

type ctxKey int

const userIDKey ctxKey = 0

// RequireAuth returns middleware that requires a valid Bearer JWT and stores
// the authenticated user id in the request context (read it with UserID).
func RequireAuth(tk *Tokenizer) func(http.Handler) http.Handler {
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
			ctx := context.WithValue(r.Context(), userIDKey, claims.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserID returns the authenticated user id, or 0 if unauthenticated.
func UserID(ctx context.Context) int64 {
	id, _ := ctx.Value(userIDKey).(int64)
	return id
}
```

- [ ] **Step 4: Run → passes.** `go vet ./...` clean.

- [ ] **Step 5: Commit**

```bash
git add internal/auth/middleware.go internal/auth/middleware_test.go
git commit -m "feat: JWT auth middleware (RequireAuth + UserID) (TDD)"
```

---

## Task 2: Migration — applications, subscriptions, credentials, product route id

**Files:** `internal/db/migrations/0003_subscribe.sql`

- [ ] **Step 1: Write `internal/db/migrations/0003_subscribe.sql`**

```sql
ALTER TABLE api_products ADD COLUMN IF NOT EXISTS apisix_route_id TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS applications (
    id          BIGSERIAL PRIMARY KEY,
    owner_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_applications_owner ON applications(owner_id);

CREATE TABLE IF NOT EXISTS credentials (
    id                BIGSERIAL PRIMARY KEY,
    application_id    BIGINT NOT NULL UNIQUE REFERENCES applications(id) ON DELETE CASCADE,
    api_key           TEXT NOT NULL UNIQUE,
    consumer_username TEXT NOT NULL UNIQUE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS subscriptions (
    id             BIGSERIAL PRIMARY KEY,
    application_id BIGINT NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    api_product_id BIGINT NOT NULL REFERENCES api_products(id) ON DELETE CASCADE,
    plan_id        BIGINT NOT NULL REFERENCES plans(id),
    status         TEXT NOT NULL DEFAULT 'active',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (application_id, api_product_id)
);
```

- [ ] **Step 2: Apply & verify** — run the backend once (or a throwaway migrate program as in Plan-1 Task 4) and confirm the four objects exist:
```bash
docker exec apisix-portal-postgres-1 psql -U portal -d portal -c "\dt applications" -c "\dt credentials" -c "\dt subscriptions" -c "SELECT column_name FROM information_schema.columns WHERE table_name='api_products' AND column_name='apisix_route_id';"
```
Expected: all three tables listed and the `apisix_route_id` column present. (Remove any throwaway program before commit.)

- [ ] **Step 3: Commit**

```bash
git add internal/db/migrations/0003_subscribe.sql
git commit -m "feat: migration for applications, credentials, subscriptions"
```

---

## Task 3: Plans read API (TDD)

**Files:** `internal/plans/plan.go`, `internal/plans/repo.go`, `internal/plans/handler.go`, `internal/plans/repo_test.go`, `internal/plans/handler_test.go`

- [ ] **Step 1: Write `internal/plans/plan.go`**

```go
package plans

// Plan is a subscription rate-limit tier.
type Plan struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	RateLimit     int    `json:"rateLimit"`
	WindowSeconds int    `json:"windowSeconds"`
}
```

- [ ] **Step 2: Write `internal/plans/repo_test.go`**

```go
package plans

import (
	"context"
	"os"
	"testing"

	"apisix-portal/internal/db"
)

func testRepo(t *testing.T) (context.Context, *Repo) {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://portal:portal@localhost:5432/portal?sslmode=disable"
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, url)
	if err != nil {
		t.Skipf("no database: %v", err)
	}
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(pool.Close)
	return ctx, NewRepo(pool)
}

func TestListReturnsThreeSeededPlans(t *testing.T) {
	ctx, repo := testRepo(t)
	all, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("want 3 plans, got %d", len(all))
	}
}

func TestGetByIDFound(t *testing.T) {
	ctx, repo := testRepo(t)
	all, _ := repo.List(ctx)
	p, err := repo.GetByID(ctx, all[0].ID)
	if err != nil || p.ID != all[0].ID {
		t.Fatalf("GetByID: %v %+v", err, p)
	}
}

func TestGetByIDNotFound(t *testing.T) {
	ctx, repo := testRepo(t)
	if _, err := repo.GetByID(ctx, 999999); err != ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
```

- [ ] **Step 3: Run → fails.**

- [ ] **Step 4: Write `internal/plans/repo.go`**

```go
package plans

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("plan not found")

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

func (r *Repo) List(ctx context.Context) ([]Plan, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, rate_limit_count, rate_limit_window_s FROM plans ORDER BY rate_limit_count ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Plan
	for rows.Next() {
		var p Plan
		if err := rows.Scan(&p.ID, &p.Name, &p.RateLimit, &p.WindowSeconds); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repo) GetByID(ctx context.Context, id int64) (Plan, error) {
	var p Plan
	err := r.pool.QueryRow(ctx,
		`SELECT id, name, rate_limit_count, rate_limit_window_s FROM plans WHERE id=$1`, id,
	).Scan(&p.ID, &p.Name, &p.RateLimit, &p.WindowSeconds)
	if errors.Is(err, pgx.ErrNoRows) {
		return Plan{}, ErrNotFound
	}
	return p, err
}
```

- [ ] **Step 5: Write `internal/plans/handler_test.go`**

```go
package plans

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeLister struct{ items []Plan }

func (f fakeLister) List(_ context.Context) ([]Plan, error) { return f.items, nil }

func TestPlansEndpoint(t *testing.T) {
	h := NewHandler(fakeLister{items: []Plan{{ID: 1, Name: "Free"}, {ID: 2, Name: "Gold"}}})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/plans", nil))
	if rec.Code != 200 {
		t.Fatalf("status=%d", rec.Code)
	}
	var got []Plan
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got) != 2 {
		t.Fatalf("want 2 plans, got %d", len(got))
	}
}
```

- [ ] **Step 6: Run → fails.**

- [ ] **Step 7: Write `internal/plans/handler.go`**

```go
package plans

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"apisix-portal/internal/httpx"
)

type Lister interface {
	List(ctx context.Context) ([]Plan, error)
}

type Handler struct {
	repo   Lister
	router chi.Router
}

func NewHandler(repo Lister) *Handler {
	h := &Handler{repo: repo, router: chi.NewRouter()}
	h.router.Get("/api/plans", h.list)
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.router.ServeHTTP(w, r) }

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	items, err := h.repo.List(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to list plans")
		return
	}
	if items == nil {
		items = []Plan{}
	}
	httpx.JSON(w, http.StatusOK, items)
}
```

- [ ] **Step 8: Run → passes** (`go test ./internal/plans/ -v`, DB up). `go vet ./...` clean.

- [ ] **Step 9: Commit**

```bash
git add internal/plans
git commit -m "feat: plans read API GET /api/plans (TDD)"
```

---

## Task 4: Applications CRUD (protected) (TDD)

**Files:** `internal/applications/app.go`, `internal/applications/repo.go`, `internal/applications/handler.go`, `internal/applications/repo_test.go`, `internal/applications/handler_test.go`

- [ ] **Step 1: Write `internal/applications/app.go`**

```go
package applications

import "time"

type Application struct {
	ID          int64     `json:"id"`
	OwnerID     int64     `json:"ownerId"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
}
```

- [ ] **Step 2: Write `internal/applications/repo_test.go`**

```go
package applications

import (
	"context"
	"os"
	"testing"
	"time"

	"apisix-portal/internal/db"
)

func testRepo(t *testing.T) (context.Context, *Repo, int64) {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://portal:portal@localhost:5432/portal?sslmode=disable"
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, url)
	if err != nil {
		t.Skipf("no database: %v", err)
	}
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(pool.Close)
	// create a throwaway user to own applications
	var uid int64
	email := "appowner+" + time.Now().Format("150405.000000000") + "@example.com"
	if err := pool.QueryRow(ctx,
		`INSERT INTO users(email,password_hash,name) VALUES($1,'x','U') RETURNING id`, email).Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return ctx, NewRepo(pool), uid
}

func TestCreateAndListByOwner(t *testing.T) {
	ctx, repo, uid := testRepo(t)
	a, err := repo.Create(ctx, uid, "My App", "desc")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if a.ID == 0 || a.OwnerID != uid {
		t.Fatalf("bad app: %+v", a)
	}
	list, err := repo.ListByOwner(ctx, uid)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListByOwner: %v len=%d", err, len(list))
	}
}

func TestGetEnforcesOwnership(t *testing.T) {
	ctx, repo, uid := testRepo(t)
	a, _ := repo.Create(ctx, uid, "App", "")
	if _, err := repo.Get(ctx, a.ID, uid); err != nil {
		t.Fatalf("owner Get: %v", err)
	}
	if _, err := repo.Get(ctx, a.ID, uid+999); err != ErrNotFound {
		t.Fatalf("non-owner Get: want ErrNotFound, got %v", err)
	}
}
```

- [ ] **Step 3: Run → fails.**

- [ ] **Step 4: Write `internal/applications/repo.go`**

```go
package applications

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("application not found")

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

func (r *Repo) Create(ctx context.Context, ownerID int64, name, description string) (Application, error) {
	var a Application
	err := r.pool.QueryRow(ctx,
		`INSERT INTO applications(owner_id,name,description) VALUES($1,$2,$3)
		 RETURNING id,owner_id,name,description,created_at`,
		ownerID, name, description,
	).Scan(&a.ID, &a.OwnerID, &a.Name, &a.Description, &a.CreatedAt)
	return a, err
}

func (r *Repo) ListByOwner(ctx context.Context, ownerID int64) ([]Application, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id,owner_id,name,description,created_at FROM applications WHERE owner_id=$1 ORDER BY created_at DESC`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Application
	for rows.Next() {
		var a Application
		if err := rows.Scan(&a.ID, &a.OwnerID, &a.Name, &a.Description, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *Repo) Get(ctx context.Context, id, ownerID int64) (Application, error) {
	var a Application
	err := r.pool.QueryRow(ctx,
		`SELECT id,owner_id,name,description,created_at FROM applications WHERE id=$1 AND owner_id=$2`, id, ownerID,
	).Scan(&a.ID, &a.OwnerID, &a.Name, &a.Description, &a.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Application{}, ErrNotFound
	}
	return a, err
}
```

- [ ] **Step 5: Write `internal/applications/handler_test.go`** (handler reads the authenticated user id from context via `auth.UserID`; tests inject it directly)

```go
package applications

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"apisix-portal/internal/auth"
)

type fakeStore struct {
	apps   []Application
	nextID int64
}

func (f *fakeStore) Create(_ context.Context, ownerID int64, name, desc string) (Application, error) {
	f.nextID++
	a := Application{ID: f.nextID, OwnerID: ownerID, Name: name, Description: desc}
	f.apps = append(f.apps, a)
	return a, nil
}
func (f *fakeStore) ListByOwner(_ context.Context, ownerID int64) ([]Application, error) {
	var out []Application
	for _, a := range f.apps {
		if a.OwnerID == ownerID {
			out = append(out, a)
		}
	}
	return out, nil
}

// withUser injects an authenticated user id the way RequireAuth would.
func withUser(r *http.Request, id int64) *http.Request {
	return r.WithContext(auth.WithUserID(r.Context(), id))
}

func TestCreateApplication(t *testing.T) {
	h := NewHandler(&fakeStore{})
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/applications", strings.NewReader(`{"name":"App1"}`)), 5)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}
	var a Application
	_ = json.Unmarshal(rec.Body.Bytes(), &a)
	if a.OwnerID != 5 || a.Name != "App1" {
		t.Fatalf("bad app: %+v", a)
	}
}

func TestListApplicationsScopedToUser(t *testing.T) {
	store := &fakeStore{}
	h := NewHandler(store)
	h.ServeHTTP(httptest.NewRecorder(), withUser(httptest.NewRequest(http.MethodPost, "/api/applications", strings.NewReader(`{"name":"A"}`)), 5))
	h.ServeHTTP(httptest.NewRecorder(), withUser(httptest.NewRequest(http.MethodPost, "/api/applications", strings.NewReader(`{"name":"B"}`)), 9))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, withUser(httptest.NewRequest(http.MethodGet, "/api/applications", nil), 5))
	var got []Application
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got) != 1 {
		t.Fatalf("user 5 should see 1 app, got %d", len(got))
	}
}
```

- [ ] **Step 6: Add `auth.WithUserID` test helper** — append to `internal/auth/middleware.go` (so tests in other packages can inject an authenticated user without forging a JWT):

```go
// WithUserID returns a context carrying the given authenticated user id.
// Used by RequireAuth and available to tests that bypass HTTP auth.
func WithUserID(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, userIDKey, id)
}
```
Then in `RequireAuth`, replace the inline `context.WithValue(...)` with `WithUserID(r.Context(), claims.UserID)`.

- [ ] **Step 7: Run handler test → fails.**

- [ ] **Step 8: Write `internal/applications/handler.go`**

```go
package applications

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"apisix-portal/internal/auth"
	"apisix-portal/internal/httpx"
)

type Store interface {
	Create(ctx context.Context, ownerID int64, name, description string) (Application, error)
	ListByOwner(ctx context.Context, ownerID int64) ([]Application, error)
}

type Handler struct {
	store  Store
	router chi.Router
}

func NewHandler(store Store) *Handler {
	h := &Handler{store: store, router: chi.NewRouter()}
	h.router.Post("/api/applications", h.create)
	h.router.Get("/api/applications", h.list)
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.router.ServeHTTP(w, r) }

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		httpx.Error(w, http.StatusBadRequest, "name is required")
		return
	}
	a, err := h.store.Create(r.Context(), auth.UserID(r.Context()), body.Name, body.Description)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to create application")
		return
	}
	httpx.JSON(w, http.StatusCreated, a)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	apps, err := h.store.ListByOwner(r.Context(), auth.UserID(r.Context()))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to list applications")
		return
	}
	if apps == nil {
		apps = []Application{}
	}
	httpx.JSON(w, http.StatusOK, apps)
}
```

- [ ] **Step 9: Run → passes** (`go test ./internal/applications/ ./internal/auth/ -v`, DB up). `go vet ./...` clean.

- [ ] **Step 10: Commit**

```bash
git add internal/applications internal/auth/middleware.go internal/auth/middleware_test.go
git commit -m "feat: applications CRUD scoped to authed user; auth.WithUserID helper (TDD)"
```

---

## Task 5: APISIX Gateway interface + fake (TDD on the fake's contract)

**Files:** `internal/apisix/gateway.go`, `internal/apisix/fake.go`, `internal/apisix/fake_test.go`

- [ ] **Step 1: Write `internal/apisix/gateway.go`**

```go
package apisix

import "context"

// RateLimit is a per-consumer request quota.
type RateLimit struct {
	Count         int
	WindowSeconds int
}

// Gateway is the subset of APISIX provisioning the portal needs.
// Implemented by *Client (real Admin API) and *Fake (tests).
type Gateway interface {
	// EnsureConsumer creates/updates a consumer "username" with a key-auth key and a limit-count.
	EnsureConsumer(ctx context.Context, username, apiKey string, limit RateLimit) error
	// DeleteConsumer removes a consumer.
	DeleteConsumer(ctx context.Context, username string) error
	// EnsureRoute creates/updates the route routeID for uri→upstream with key-auth and a
	// consumer-restriction whitelist of the given consumer usernames.
	EnsureRoute(ctx context.Context, routeID, uri, upstream string, allowedConsumers []string) error
}
```

- [ ] **Step 2: Write `internal/apisix/fake_test.go`**

```go
package apisix

import (
	"context"
	"testing"
)

func TestFakeRecordsConsumersAndRoutes(t *testing.T) {
	ctx := context.Background()
	f := NewFake()
	if err := f.EnsureConsumer(ctx, "app_1", "key-abc", RateLimit{Count: 60, WindowSeconds: 60}); err != nil {
		t.Fatal(err)
	}
	if f.Consumers["app_1"].APIKey != "key-abc" || f.Consumers["app_1"].Limit.Count != 60 {
		t.Fatalf("consumer not recorded: %+v", f.Consumers["app_1"])
	}
	if err := f.EnsureRoute(ctx, "prod_3", "/pizzashack/*", "echo:8080", []string{"app_1"}); err != nil {
		t.Fatal(err)
	}
	if got := f.Routes["prod_3"].Allowed; len(got) != 1 || got[0] != "app_1" {
		t.Fatalf("route whitelist not recorded: %+v", got)
	}
	if err := f.DeleteConsumer(ctx, "app_1"); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.Consumers["app_1"]; ok {
		t.Fatal("consumer not deleted")
	}
}
```

- [ ] **Step 3: Run → fails.**

- [ ] **Step 4: Write `internal/apisix/fake.go`**

```go
package apisix

import (
	"context"
	"sync"
)

type FakeConsumer struct {
	APIKey string
	Limit  RateLimit
}
type FakeRoute struct {
	URI      string
	Upstream string
	Allowed  []string
}

// Fake is an in-memory Gateway for unit tests.
type Fake struct {
	mu        sync.Mutex
	Consumers map[string]FakeConsumer
	Routes    map[string]FakeRoute
}

func NewFake() *Fake {
	return &Fake{Consumers: map[string]FakeConsumer{}, Routes: map[string]FakeRoute{}}
}

func (f *Fake) EnsureConsumer(_ context.Context, username, apiKey string, limit RateLimit) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Consumers[username] = FakeConsumer{APIKey: apiKey, Limit: limit}
	return nil
}
func (f *Fake) DeleteConsumer(_ context.Context, username string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.Consumers, username)
	return nil
}
func (f *Fake) EnsureRoute(_ context.Context, routeID, uri, upstream string, allowed []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Routes[routeID] = FakeRoute{URI: uri, Upstream: upstream, Allowed: append([]string(nil), allowed...)}
	return nil
}
```

- [ ] **Step 5: Run → passes.** Commit:

```bash
git add internal/apisix/gateway.go internal/apisix/fake.go internal/apisix/fake_test.go
git commit -m "feat: APISIX Gateway interface + in-memory fake (TDD)"
```

---

## Task 6: APISIX Admin API client (real implementation)

**Files:** `internal/apisix/client.go`

> No unit test here (it just does HTTP); it is exercised by the integration test in Task 9. Keep it a faithful, small implementation.

- [ ] **Step 1: Write `internal/apisix/client.go`**

```go
package apisix

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client talks to the APISIX Admin API.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) do(ctx context.Context, method, path string, body any) error {
	var buf io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		buf = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, buf)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-KEY", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("apisix admin %s %s: %d %s", method, path, resp.StatusCode, string(msg))
	}
	return nil
}

func (c *Client) EnsureConsumer(ctx context.Context, username, apiKey string, limit RateLimit) error {
	body := map[string]any{
		"username": username,
		"plugins": map[string]any{
			"key-auth": map[string]any{"key": apiKey},
			"limit-count": map[string]any{
				"count": limit.Count, "time_window": limit.WindowSeconds,
				"rejected_code": 429, "key_type": "var", "key": "consumer_name", "policy": "local",
			},
		},
	}
	// Admin API consumers are created/updated via PUT with the username in the body.
	return c.do(ctx, http.MethodPut, "/apisix/admin/consumers", body)
}

func (c *Client) DeleteConsumer(ctx context.Context, username string) error {
	return c.do(ctx, http.MethodDelete, "/apisix/admin/consumers/"+username, nil)
}

func (c *Client) EnsureRoute(ctx context.Context, routeID, uri, upstream string, allowed []string) error {
	host, portStr, ok := strings.Cut(upstream, ":")
	if !ok {
		return fmt.Errorf("upstream must be host:port, got %q", upstream)
	}
	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
		return fmt.Errorf("bad upstream port %q: %w", portStr, err)
	}
	if allowed == nil {
		allowed = []string{}
	}
	body := map[string]any{
		"uri": uri,
		"upstream": map[string]any{
			"type":  "roundrobin",
			"nodes": map[string]int{fmt.Sprintf("%s:%d", host, port): 1},
		},
		"plugins": map[string]any{
			"key-auth":             map[string]any{},
			"consumer-restriction": map[string]any{"type": "consumer_name", "whitelist": allowed},
		},
	}
	return c.do(ctx, http.MethodPut, "/apisix/admin/routes/"+routeID, body)
}
```

- [ ] **Step 2: Verify build** — `go build ./...` clean; `go vet ./...` clean.

- [ ] **Step 3: Commit**

```bash
git add internal/apisix/client.go
git commit -m "feat: APISIX Admin API client (consumers + routes)"
```

---

## Task 7: Subscription service + persistence (TDD against the fake)

**Files:** `internal/subscriptions/service.go`, `internal/subscriptions/repo.go`, `internal/subscriptions/service_test.go`

The service depends on small interfaces it can fake: a `ProductLookup` (route id, context path, upstream for a product), the `apisix.Gateway`, and its own `Store` (persist credential + subscription, list allowed consumers for a product route). Key generation is injected for determinism in tests.

- [ ] **Step 1: Write `internal/subscriptions/service.go`**

```go
package subscriptions

import (
	"context"
	"fmt"

	"apisix-portal/internal/apisix"
)

// ProductInfo is what the service needs to provision a product's gateway route.
type ProductInfo struct {
	ID          int64
	ContextPath string
	Upstream    string // host:port
}

// PlanInfo is the rate limit for a subscription.
type PlanInfo struct {
	ID            int64
	Count         int
	WindowSeconds int
}

// Credential is an application's gateway identity.
type Credential struct {
	ApplicationID    int64  `json:"applicationId"`
	APIKey           string `json:"apiKey"`
	ConsumerUsername string `json:"consumerUsername"`
}

// Store persists credentials/subscriptions and answers provisioning queries.
type Store interface {
	GetOrCreateCredential(ctx context.Context, appID int64, genKey func() string) (Credential, error)
	GetProduct(ctx context.Context, productID int64) (ProductInfo, error)
	GetPlan(ctx context.Context, planID int64) (PlanInfo, error)
	SaveSubscription(ctx context.Context, appID, productID, planID int64) error
	DeleteSubscription(ctx context.Context, appID, productID int64) error
	// ConsumersForProduct returns the consumer usernames of every application
	// currently subscribed to the product (used to rebuild the route whitelist).
	ConsumersForProduct(ctx context.Context, productID int64) ([]string, error)
}

func consumerName(appID int64) string { return fmt.Sprintf("app_%d", appID) }
func routeID(productID int64) string  { return fmt.Sprintf("prod_%d", productID) }

// Service orchestrates subscribe/unsubscribe and the matching APISIX provisioning.
type Service struct {
	store  Store
	gw     apisix.Gateway
	genKey func() string
}

func NewService(store Store, gw apisix.Gateway, genKey func() string) *Service {
	return &Service{store: store, gw: gw, genKey: genKey}
}

// Subscribe provisions APISIX and persists the subscription, returning the app's credential.
func (s *Service) Subscribe(ctx context.Context, appID, productID, planID int64) (Credential, error) {
	prod, err := s.store.GetProduct(ctx, productID)
	if err != nil {
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
	// 1. consumer carries the key + this plan's limit
	if err := s.gw.EnsureConsumer(ctx, cred.ConsumerUsername, cred.APIKey,
		apisix.RateLimit{Count: plan.Count, WindowSeconds: plan.WindowSeconds}); err != nil {
		return Credential{}, err
	}
	// 2. persist the subscription, then rebuild the product route whitelist from the DB
	if err := s.store.SaveSubscription(ctx, appID, productID, planID); err != nil {
		return Credential{}, err
	}
	allowed, err := s.store.ConsumersForProduct(ctx, productID)
	if err != nil {
		return Credential{}, err
	}
	if err := s.gw.EnsureRoute(ctx, routeID(prod.ID), prod.ContextPath+"/*", prod.Upstream, allowed); err != nil {
		return Credential{}, err
	}
	return cred, nil
}

// Unsubscribe removes the subscription and updates the route whitelist.
func (s *Service) Unsubscribe(ctx context.Context, appID, productID int64) error {
	prod, err := s.store.GetProduct(ctx, productID)
	if err != nil {
		return err
	}
	if err := s.store.DeleteSubscription(ctx, appID, productID); err != nil {
		return err
	}
	allowed, err := s.store.ConsumersForProduct(ctx, productID)
	if err != nil {
		return err
	}
	return s.gw.EnsureRoute(ctx, routeID(prod.ID), prod.ContextPath+"/*", prod.Upstream, allowed)
}
```

- [ ] **Step 2: Write `internal/subscriptions/service_test.go`** (fakes the Store + uses `apisix.NewFake`)

```go
package subscriptions

import (
	"context"
	"testing"

	"apisix-portal/internal/apisix"
)

type memStore struct {
	creds    map[int64]Credential
	subs     map[int64][]string // productID -> consumer usernames
	products map[int64]ProductInfo
	plans    map[int64]PlanInfo
}

func newMemStore() *memStore {
	return &memStore{
		creds:    map[int64]Credential{},
		subs:     map[int64][]string{},
		products: map[int64]ProductInfo{3: {ID: 3, ContextPath: "/pizzashack", Upstream: "echo:8080"}},
		plans:    map[int64]PlanInfo{2: {ID: 2, Count: 100, WindowSeconds: 60}},
	}
}

func (m *memStore) GetOrCreateCredential(_ context.Context, appID int64, genKey func() string) (Credential, error) {
	if c, ok := m.creds[appID]; ok {
		return c, nil
	}
	c := Credential{ApplicationID: appID, APIKey: genKey(), ConsumerUsername: consumerName(appID)}
	m.creds[appID] = c
	return c, nil
}
func (m *memStore) GetProduct(_ context.Context, id int64) (ProductInfo, error) { return m.products[id], nil }
func (m *memStore) GetPlan(_ context.Context, id int64) (PlanInfo, error)       { return m.plans[id], nil }
func (m *memStore) SaveSubscription(_ context.Context, appID, productID, _ int64) error {
	m.subs[productID] = append(m.subs[productID], consumerName(appID))
	return nil
}
func (m *memStore) DeleteSubscription(_ context.Context, appID, productID int64) error {
	cur := m.subs[productID]
	out := cur[:0]
	for _, u := range cur {
		if u != consumerName(appID) {
			out = append(out, u)
		}
	}
	m.subs[productID] = out
	return nil
}
func (m *memStore) ConsumersForProduct(_ context.Context, productID int64) ([]string, error) {
	return m.subs[productID], nil
}

func TestSubscribeProvisionsConsumerAndRoute(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
	gw := apisix.NewFake()
	svc := NewService(store, gw, func() string { return "fixed-key" })

	cred, err := svc.Subscribe(ctx, 42, 3, 2)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if cred.APIKey != "fixed-key" || cred.ConsumerUsername != "app_42" {
		t.Fatalf("bad cred: %+v", cred)
	}
	// consumer provisioned with the plan limit
	c := gw.Consumers["app_42"]
	if c.APIKey != "fixed-key" || c.Limit.Count != 100 {
		t.Fatalf("consumer not provisioned: %+v", c)
	}
	// route provisioned with this consumer in the whitelist
	r := gw.Routes["prod_3"]
	if r.URI != "/pizzashack/*" || len(r.Allowed) != 1 || r.Allowed[0] != "app_42" {
		t.Fatalf("route not provisioned: %+v", r)
	}
}

func TestUnsubscribeRemovesFromWhitelist(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
	gw := apisix.NewFake()
	svc := NewService(store, gw, func() string { return "k" })
	_, _ = svc.Subscribe(ctx, 42, 3, 2)
	_, _ = svc.Subscribe(ctx, 43, 3, 2)
	if err := svc.Unsubscribe(ctx, 42, 3); err != nil {
		t.Fatalf("Unsubscribe: %v", err)
	}
	r := gw.Routes["prod_3"]
	if len(r.Allowed) != 1 || r.Allowed[0] != "app_43" {
		t.Fatalf("whitelist after unsubscribe: %+v", r.Allowed)
	}
}
```

- [ ] **Step 3: Run → fails, then implement is already written above → run → passes.** (`go test ./internal/subscriptions/ -run TestSubscribe -v` — no DB needed for service tests.)

- [ ] **Step 4: Write `internal/subscriptions/repo.go`** — the real `Store` backed by Postgres + a key generator.

```go
package subscriptions

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

// GenerateKey returns a random 32-hex-char API key.
func GenerateKey() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (r *Repo) GetOrCreateCredential(ctx context.Context, appID int64, genKey func() string) (Credential, error) {
	var c Credential
	err := r.pool.QueryRow(ctx,
		`SELECT application_id, api_key, consumer_username FROM credentials WHERE application_id=$1`, appID,
	).Scan(&c.ApplicationID, &c.APIKey, &c.ConsumerUsername)
	if err == nil {
		return c, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Credential{}, err
	}
	c = Credential{ApplicationID: appID, APIKey: genKey(), ConsumerUsername: consumerName(appID)}
	_, err = r.pool.Exec(ctx,
		`INSERT INTO credentials(application_id, api_key, consumer_username) VALUES($1,$2,$3)`,
		c.ApplicationID, c.APIKey, c.ConsumerUsername)
	return c, err
}

func (r *Repo) GetProduct(ctx context.Context, id int64) (ProductInfo, error) {
	var p ProductInfo
	err := r.pool.QueryRow(ctx,
		`SELECT id, context_path, upstream_url FROM api_products WHERE id=$1`, id,
	).Scan(&p.ID, &p.ContextPath, &p.Upstream)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProductInfo{}, ErrNotFound
	}
	return p, err
}

func (r *Repo) GetPlan(ctx context.Context, id int64) (PlanInfo, error) {
	var p PlanInfo
	err := r.pool.QueryRow(ctx,
		`SELECT id, rate_limit_count, rate_limit_window_s FROM plans WHERE id=$1`, id,
	).Scan(&p.ID, &p.Count, &p.WindowSeconds)
	if errors.Is(err, pgx.ErrNoRows) {
		return PlanInfo{}, ErrNotFound
	}
	return p, err
}

func (r *Repo) SaveSubscription(ctx context.Context, appID, productID, planID int64) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO subscriptions(application_id, api_product_id, plan_id) VALUES($1,$2,$3)
		 ON CONFLICT (application_id, api_product_id)
		 DO UPDATE SET plan_id=EXCLUDED.plan_id, status='active'`,
		appID, productID, planID)
	return err
}

func (r *Repo) DeleteSubscription(ctx context.Context, appID, productID int64) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM subscriptions WHERE application_id=$1 AND api_product_id=$2`, appID, productID)
	return err
}

func (r *Repo) ConsumersForProduct(ctx context.Context, productID int64) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT c.consumer_username FROM subscriptions s
		 JOIN credentials c ON c.application_id = s.application_id
		 WHERE s.api_product_id=$1 AND s.status='active'`, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
```

- [ ] **Step 5: Build + run service tests** — `go build ./...`, `go test ./internal/subscriptions/ -v` (service tests pass; repo has no dedicated unit test — it's covered by the integration test). `go vet ./...` clean.

- [ ] **Step 6: Commit**

```bash
git add internal/subscriptions/service.go internal/subscriptions/service_test.go internal/subscriptions/repo.go
git commit -m "feat: subscription service + postgres store, provisioning via Gateway (TDD)"
```

---

## Task 8: Subscriptions HTTP handler (TDD)

**Files:** `internal/subscriptions/handler.go`, `internal/subscriptions/handler_test.go`

Endpoints (all under RequireAuth in main): `POST /api/applications/{appID}/subscriptions` (body `{productId, planId}` → 201 with the credential), `DELETE /api/applications/{appID}/subscriptions/{productID}` → 204. The handler verifies the app belongs to the caller via an `OwnerCheck` func.

- [ ] **Step 1: Write `internal/subscriptions/handler_test.go`**

```go
package subscriptions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"apisix-portal/internal/apisix"
	"apisix-portal/internal/auth"
)

func newTestHandler() (*Handler, *apisix.Fake) {
	store := newMemStore()
	gw := apisix.NewFake()
	svc := NewService(store, gw, func() string { return "key-xyz" })
	// owner check: app 1 belongs to user 5
	owns := func(_ context.Context, appID, userID int64) (bool, error) { return appID == 1 && userID == 5, nil }
	return NewHandler(svc, owns), gw
}

func TestSubscribeEndpointProvisionsAndReturnsKey(t *testing.T) {
	h, gw := newTestHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/applications/1/subscriptions", strings.NewReader(`{"productId":3,"planId":2}`))
	req = req.WithContext(auth.WithUserID(req.Context(), 5))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"apiKey":"key-xyz"`) {
		t.Fatalf("missing api key in body: %s", rec.Body)
	}
	if _, ok := gw.Consumers["app_1"]; !ok {
		t.Fatal("consumer not provisioned")
	}
}

func TestSubscribeEndpointRejectsNonOwner(t *testing.T) {
	h, _ := newTestHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/applications/1/subscriptions", strings.NewReader(`{"productId":3,"planId":2}`))
	req = req.WithContext(auth.WithUserID(req.Context(), 999)) // not the owner
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d want 403", rec.Code)
	}
}
```

- [ ] **Step 2: Run → fails.**

- [ ] **Step 3: Write `internal/subscriptions/handler.go`**

```go
package subscriptions

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"apisix-portal/internal/auth"
	"apisix-portal/internal/httpx"
)

// OwnerCheck reports whether appID belongs to userID.
type OwnerCheck func(ctx context.Context, appID, userID int64) (bool, error)

type Handler struct {
	svc    *Service
	owns   OwnerCheck
	router chi.Router
}

func NewHandler(svc *Service, owns OwnerCheck) *Handler {
	h := &Handler{svc: svc, owns: owns, router: chi.NewRouter()}
	h.router.Post("/api/applications/{appID}/subscriptions", h.subscribe)
	h.router.Delete("/api/applications/{appID}/subscriptions/{productID}", h.unsubscribe)
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.router.ServeHTTP(w, r) }

func (h *Handler) authorize(w http.ResponseWriter, r *http.Request) (int64, bool) {
	appID, err := strconv.ParseInt(chi.URLParam(r, "appID"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "bad application id")
		return 0, false
	}
	ok, err := h.owns(r.Context(), appID, auth.UserID(r.Context()))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "ownership check failed")
		return 0, false
	}
	if !ok {
		httpx.Error(w, http.StatusForbidden, "not your application")
		return 0, false
	}
	return appID, true
}

func (h *Handler) subscribe(w http.ResponseWriter, r *http.Request) {
	appID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	var body struct {
		ProductID int64 `json:"productId"`
		PlanID    int64 `json:"planId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ProductID == 0 || body.PlanID == 0 {
		httpx.Error(w, http.StatusBadRequest, "productId and planId are required")
		return
	}
	cred, err := h.svc.Subscribe(r.Context(), appID, body.ProductID, body.PlanID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "subscription/provisioning failed: "+err.Error())
		return
	}
	httpx.JSON(w, http.StatusCreated, cred)
}

func (h *Handler) unsubscribe(w http.ResponseWriter, r *http.Request) {
	appID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	productID, err := strconv.ParseInt(chi.URLParam(r, "productID"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "bad product id")
		return
	}
	if err := h.svc.Unsubscribe(r.Context(), appID, productID); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "unsubscribe failed: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 4: Run → passes** (`go test ./internal/subscriptions/ -v`). `go vet ./...` clean.

- [ ] **Step 5: Commit**

```bash
git add internal/subscriptions/handler.go internal/subscriptions/handler_test.go
git commit -m "feat: subscribe/unsubscribe HTTP handlers with ownership check (TDD)"
```

---

## Task 9: Compose echo upstream + wire main + hermetic integration test

**Files:** `docker-compose.yml`, `cmd/portal/main.go`, `internal/apisix/client_it_test.go`

- [ ] **Step 1: Add an echo upstream to `docker-compose.yml`** (under `services:`)

```yaml
  echo:
    image: mendhak/http-https-echo:31
    environment:
      HTTP_PORT: "8080"
    # no host port needed; APISIX reaches it as echo:8080 on the compose network
```
Bring it up: `docker compose up -d echo` and confirm `docker compose ps` shows it running.

- [ ] **Step 2: Wire the new packages into `cmd/portal/main.go`**

Add imports `apisix-portal/internal/apisix`, `applications`, `plans`, `subscriptions`, and `auth` middleware usage. After constructing `pool` and the existing handlers, add:

```go
	tok := auth.NewTokenizer(cfg.JWTSecret)
	gw := apisix.NewClient(cfg.APISIXAdminURL, cfg.APISIXAdminKey)

	plansH := plans.NewHandler(plans.NewRepo(pool))
	appsRepo := applications.NewRepo(pool)
	appsH := applications.NewHandler(appsRepo)
	subRepo := subscriptions.NewRepo(pool)
	subSvc := subscriptions.NewService(subRepo, gw, subscriptions.GenerateKey)
	owns := func(ctx context.Context, appID, userID int64) (bool, error) {
		if _, err := appsRepo.Get(ctx, appID, userID); err != nil {
			if err == applications.ErrNotFound {
				return false, nil
			}
			return false, err
		}
		return true, nil
	}
	subH := subscriptions.NewHandler(subSvc, owns)

	requireAuth := auth.RequireAuth(tok)
```

Register routes on the existing `mux` (auth handler already built with the same tokenizer — reuse `tok`):
```go
	mux.Handle("/api/plans", plansH)
	mux.Handle("/api/applications", requireAuth(appsH))
	mux.Handle("/api/applications/", requireAuth(subH)) // subtree: /{id}/subscriptions[/{productID}]
```
Note: the existing `authH` (register/login) must be built with the SAME `tok` you just created — update its construction to `auth.NewHandler(auth.NewRepo(pool), tok)`. Keep `/api/products` and `/api/auth/` as before. The applications-list endpoint (`/api/applications`, exact) and the subscriptions subtree (`/api/applications/`) are both protected by `requireAuth`.

Verify: `go build ./...` clean; `go test ./...` (DB up) all green; run the server and smoke `GET /api/plans` returns 3 plans.

- [ ] **Step 3: Write the gated integration test `internal/apisix/client_it_test.go`**

```go
package apisix

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
)

// Runs only when RUN_APISIX_IT=1 and the compose stack (apisix + echo) is up.
// Verifies the full provisioning chain end-to-end against the real gateway.
func TestIntegrationProvisionAndCall(t *testing.T) {
	if os.Getenv("RUN_APISIX_IT") != "1" {
		t.Skip("set RUN_APISIX_IT=1 with the compose stack up to run")
	}
	adminURL := envOr("APISIX_ADMIN_URL", "http://localhost:19180")
	adminKey := envOr("APISIX_ADMIN_KEY", "edd1c9f034335f136f87ad84b625c8f1")
	gwURL := envOr("APISIX_GATEWAY_URL", "http://localhost:9080")

	ctx := context.Background()
	c := NewClient(adminURL, adminKey)

	user := fmt.Sprintf("it_app_%d", time.Now().UnixNano())
	key := fmt.Sprintf("itkey-%d", time.Now().UnixNano())
	routeID := "it_route"
	uri := "/itecho/*"

	if err := c.EnsureConsumer(ctx, user, key, RateLimit{Count: 3, WindowSeconds: 60}); err != nil {
		t.Fatalf("EnsureConsumer: %v", err)
	}
	t.Cleanup(func() { _ = c.DeleteConsumer(ctx, user) })
	if err := c.EnsureRoute(ctx, routeID, uri, "echo:8080", []string{user}); err != nil {
		t.Fatalf("EnsureRoute: %v", err)
	}
	t.Cleanup(func() { _ = c.do(ctx, http.MethodDelete, "/apisix/admin/routes/"+routeID, nil) })

	time.Sleep(500 * time.Millisecond) // let APISIX sync from etcd

	// no key → 401
	if code := call(t, gwURL+"/itecho/x", ""); code != http.StatusUnauthorized {
		t.Fatalf("no key: got %d want 401", code)
	}
	// valid key within limit → 200 (x3)
	for i := 0; i < 3; i++ {
		if code := call(t, gwURL+"/itecho/x", key); code != http.StatusOK {
			t.Fatalf("call %d: got %d want 200", i+1, code)
		}
	}
	// 4th call exceeds the limit-count of 3 → 429
	if code := call(t, gwURL+"/itecho/x", key); code != http.StatusTooManyRequests {
		t.Fatalf("over limit: got %d want 429", code)
	}
}

func call(t *testing.T, url, key string) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	if key != "" {
		req.Header.Set("apikey", key)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("call %s: %v", url, err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
```

- [ ] **Step 4: Run the integration test for real**

```bash
docker compose up -d            # postgres, etcd, apisix, echo
RUN_APISIX_IT=1 go test ./internal/apisix/ -run TestIntegration -v
```
Expected: PASS — no-key=401, three 200s, then 429. (This proves the issued key works at the live gateway and the plan limit is enforced.) Also run the full hermetic suite without the flag: `go test ./...` → all green, the integration test SKIPPED.

- [ ] **Step 5: Commit**

```bash
git add docker-compose.yml cmd/portal/main.go internal/apisix/client_it_test.go
git commit -m "feat: wire plans/apps/subscriptions; echo upstream; gated APISIX integration test"
```

---

## Self-review notes (author)

- **Spec coverage:** Application abstraction (Task 4) ✓; auto-approve subscriptions (status defaults 'active', Task 2/7) ✓; key-auth credential per app (Task 7) ✓; APISIX provisioning = consumer + `consumer-restriction` enforcement + `limit-count` plan (Tasks 5–9) ✓; protected endpoints via JWT middleware (Task 1, wired Task 9) ✓; the issued key works at the gateway and the limit is enforced (Task 9 integration test) ✓. Publishing routes is done lazily on first subscribe (route ensured during Subscribe), which is sufficient for V1; an explicit admin "publish" is Plan 4.
- **No placeholders:** every step has complete code; the integration test is gated by `RUN_APISIX_IT` so the default `go test ./...` stays hermetic.
- **Type consistency:** `apisix.Gateway`/`RateLimit` used identically by `Fake`, `Client`, and the `subscriptions.Service`; `consumerName`/`routeID` helpers centralize the naming used across provisioning and the whitelist rebuild; `auth.WithUserID`/`auth.UserID` are the single context contract used by middleware and every protected handler/test.
- **Deferred (by design):** the frontend SubscribeModal/Applications UI (Plan 3b) and an explicit admin publish flow (Plan 4). Seeded products have an empty `upstream_url`; real gateway calls need a real upstream — the integration test uses the `echo` service, and a later admin/publish step will set per-product upstreams.
```
