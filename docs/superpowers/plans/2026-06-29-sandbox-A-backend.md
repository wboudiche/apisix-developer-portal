# Sandbox Environment — Plan 1 (Backend + Infra) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up a dedicated sandbox APISIX gateway and the backend that lets an approved subscriber opt an app into sandbox (a separate per-app sandbox key) and reach a product's sandbox upstream — provisioning, endpoints, and the Try-it sandbox proxy.

**Architecture:** A second APISIX data-plane (sharing etcd under a distinct prefix) is the sandbox gateway. Products gain an optional `sandbox_upstream_url`; credentials gain an encrypted `sandbox_api_key`. `subscriptions.Service` gets a second `apisix.Gateway` and maintains the **sandbox-route whitelist invariant** (a product's sandbox route admits exactly its active subscribers whose app has a sandbox key) across enable/approve/reject/unsubscribe/plan-change/upstream-change. New endpoints enable + rotate the sandbox key; the Try-it proxy gains a sandbox variant.

**Tech Stack:** Go 1.25 (chi, pgx, crypto Cipher), APISIX 3.9.1, docker-compose.

## Global Constraints

- Module `apisix-portal`. Sandbox provisioning lives in `internal/subscriptions`; product sandbox upstream in `internal/admin`/`internal/catalog`; sandbox proxy in `internal/tryit`; config in `internal/config`.
- **Sandbox-route whitelist invariant:** a product's sandbox route (`prod_<id>` on the sandbox gateway) whitelists exactly the apps that are **active** subscribers of that product AND have a non-empty `sandbox_api_key`.
- Sandbox consumer = the app's existing `app_<id>` username on the **sandbox gateway**, with a **distinct** key; sandbox limit = the app's most-recent active subscription plan (reuse `ActivePlanForApp`).
- `sandbox_api_key` is **encrypted at rest** via the existing `crypto.Cipher` (exactly like `api_key`); `''` = sandbox not enabled for the app.
- Sandbox is active only when both `APISIX_SANDBOX_ADMIN_URL` and `APISIX_SANDBOX_GATEWAY_URL` are set; otherwise `sandboxGW` is nil and all sandbox paths are inert (endpoints return 409).
- Gateway-before-DB ordering on any key change (mirror `RotateKey`). Reuse `subscriptions.GenerateKey` via the service's `genKey`.
- Tests: backend `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/... ./cmd/...`; `gofmt -w` every touched Go file; `go vet ./...`.

---

## Task 1: Sandbox gateway infra + config

**Files:**
- Create: `deploy/apisix-sandbox/config.yaml`
- Modify: `docker-compose.yml`
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `Config.APISIXSandboxAdminURL`, `Config.APISIXSandboxGatewayURL`, `Config.APISIXSandboxAdminKey string`; `func (c Config) SandboxConfigured() bool`.

- [ ] **Step 1: Create the sandbox APISIX config**

Create `deploy/apisix-sandbox/config.yaml` (identical to `deploy/apisix/config.yaml` except the etcd **prefix** — the two control planes share one etcd cluster but must not collide):
```yaml
nginx_config:
  worker_processes: 1
apisix:
  node_listen: 9080
  enable_admin: true
plugin_attr:
  prometheus:
    export_addr:
      ip: "0.0.0.0"
      port: 9091
deployment:
  admin:
    admin_key:
      - name: admin
        key: edd1c9f034335f136f87ad84b625c8f1
        role: admin
    allow_admin:
      - 127.0.0.0/8
      - 172.16.0.0/12
      - 192.168.0.0/16
      - 10.0.0.0/8
  etcd:
    host:
      - "http://etcd:2379"
    prefix: /apisix-sandbox
    timeout: 30
```

- [ ] **Step 2: Add the sandbox gateway to docker-compose**

In `docker-compose.yml`, add after the `apisix:` service (same image, shared etcd, host ports `9081` data / `19280` admin):
```yaml
  apisix-sandbox:
    image: apache/apisix:3.9.1-debian
    depends_on: [etcd]
    volumes:
      - ./deploy/apisix-sandbox/config.yaml:/usr/local/apisix/conf/config.yaml:ro
    ports:
      - "9081:9080"
      - "127.0.0.1:19280:9180"
```

- [ ] **Step 3: Write the failing config test**

In `internal/config/config_test.go` add:
```go
func TestSandboxConfigDefaultsAndPredicate(t *testing.T) {
	t.Setenv("PORTAL_ENV", "dev")
	c := Load()
	if c.APISIXSandboxAdminURL != "http://localhost:19280" {
		t.Errorf("sandbox admin url = %q", c.APISIXSandboxAdminURL)
	}
	if c.APISIXSandboxGatewayURL != "http://localhost:9081" {
		t.Errorf("sandbox gateway url = %q", c.APISIXSandboxGatewayURL)
	}
	// Sandbox admin key defaults to the production admin key.
	if c.APISIXSandboxAdminKey != c.APISIXAdminKey {
		t.Errorf("sandbox admin key = %q, want = prod admin key", c.APISIXSandboxAdminKey)
	}
	if !c.SandboxConfigured() {
		t.Error("SandboxConfigured() = false, want true with both URLs set")
	}
	c.APISIXSandboxGatewayURL = ""
	if c.SandboxConfigured() {
		t.Error("SandboxConfigured() = true with gateway URL empty")
	}
}
```
(Match the test file's existing convention for invoking config load — if it calls `Load()` directly use that; if it uses a helper, mirror it.)

- [ ] **Step 4: Run to verify it fails**

Run: `go test ./internal/config/ -run TestSandboxConfig -v`
Expected: FAIL — fields/method undefined.

- [ ] **Step 5: Add the config fields + predicate**

In `internal/config/config.go`, add to the `Config` struct (near `APISIXGatewayURL`):
```go
	APISIXSandboxAdminURL   string
	APISIXSandboxGatewayURL string
	APISIXSandboxAdminKey   string
```
In `Load()` (near the other APISIX gets), add:
```go
		APISIXSandboxAdminURL:   get("APISIX_SANDBOX_ADMIN_URL", "http://localhost:19280"),
		APISIXSandboxGatewayURL: get("APISIX_SANDBOX_GATEWAY_URL", "http://localhost:9081"),
		APISIXSandboxAdminKey:   get("APISIX_SANDBOX_ADMIN_KEY", get("APISIX_ADMIN_KEY", DevAPISIXAdminKey)),
```
(The sandbox admin key falls back to the production admin key so dev needs no extra secret. If the codebase computes `APISIXAdminKey` via the same `get("APISIX_ADMIN_KEY", DevAPISIXAdminKey)`, keep these two consistent.) Add the predicate near the other `Config` methods:
```go
// SandboxConfigured reports whether the dedicated sandbox gateway is wired up.
// When false, the portal runs production-only and all sandbox features are inert.
func (c Config) SandboxConfigured() bool {
	return c.APISIXSandboxAdminURL != "" && c.APISIXSandboxGatewayURL != ""
}
```

- [ ] **Step 6: Run to verify it passes**

Run: `go test ./internal/config/ && go vet ./internal/config/`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add deploy/apisix-sandbox/config.yaml docker-compose.yml internal/config/config.go internal/config/config_test.go
git commit -m "feat(sandbox): dedicated sandbox APISIX gateway + config"
```

---

## Task 2: Migration 0011 + product sandbox_upstream_url

**Files:**
- Create: `internal/db/migrations/0011_sandbox.sql`
- Modify: `internal/admin/product.go` (`SandboxUpstreamURL` + validate), `internal/admin/repo.go` (`productCols`, scan, Create, Update)
- Modify: `internal/subscriptions/service.go` (`ProductInfo.SandboxUpstream`), `internal/subscriptions/repo.go` (`GetProduct` selects it)
- Test: `internal/admin/product_test.go`

**Interfaces:**
- Produces: `admin.Product.SandboxUpstreamURL string` (json `sandboxUpstreamUrl`); `subscriptions.ProductInfo.SandboxUpstream string`; both columns exist in DB.

- [ ] **Step 1: Write the migration**

Create `internal/db/migrations/0011_sandbox.sql`:
```sql
-- Per-product sandbox backend + per-app sandbox key (encrypted, '' = disabled).
ALTER TABLE api_products ADD COLUMN IF NOT EXISTS sandbox_upstream_url TEXT NOT NULL DEFAULT '';
ALTER TABLE credentials   ADD COLUMN IF NOT EXISTS sandbox_api_key      TEXT NOT NULL DEFAULT '';
```

- [ ] **Step 2: Write the failing admin test**

In `internal/admin/product_test.go` add (mirror the existing `Validate` test style):
```go
func TestValidateRejectsBadSandboxUpstream(t *testing.T) {
	p := Product{Name: "X", Slug: "x", Category: "C", ContextPath: "/x", SandboxUpstreamURL: "not a url"}
	if err := p.Validate(false); err == nil {
		t.Fatal("expected invalid sandbox upstream to fail validation")
	}
}

func TestValidateAcceptsEmptySandboxUpstream(t *testing.T) {
	p := Product{Name: "X", Slug: "x", Category: "C", ContextPath: "/x"} // sandbox optional
	if err := p.Validate(false); err != nil {
		t.Fatalf("empty sandbox upstream should be valid: %v", err)
	}
}
```

- [ ] **Step 3: Run to verify it fails**

Run: `go test ./internal/admin/ -run TestValidate -v`
Expected: FAIL — `SandboxUpstreamURL` undefined.

- [ ] **Step 4: Add the field, validation, and persistence**

In `internal/admin/product.go`, add to `Product` (after `UpstreamURL`):
```go
	SandboxUpstreamURL string `json:"sandboxUpstreamUrl"`
```
In `Validate`, after the existing `UpstreamURL` check, add the same guard for sandbox:
```go
	if p.SandboxUpstreamURL != "" && !ValidUpstream(p.SandboxUpstreamURL, allowPrivate) {
		return errors.New("invalid sandbox upstream")
	}
```
(Match the exact error-construction style already used in `Validate` — if it returns sentinel errors or `fmt.Errorf`, mirror that.)

In `internal/admin/repo.go`:
- Extend `productCols`:
```go
const productCols = `id, name, slug, category, version, context_path, description, tags, icon, upstream_url, sandbox_upstream_url, published`
```
- In `scanProduct`, add the scan target after `&p.UpstreamURL`:
```go
		&p.UpstreamURL, &p.SandboxUpstreamURL, &p.Published)
```
- In `Create`, add `sandbox_upstream_url` to the column list and a new positional placeholder, and `p.SandboxUpstreamURL` to the args (insert it right after `upstream_url`/`p.UpstreamURL`; renumber the `published`/`openapi_spec` placeholders accordingly).
- In `Update`, add `sandbox_upstream_url=$N` to the SET list (right after `upstream_url=$10`) and `p.SandboxUpstreamURL` to the args, renumbering the trailing placeholders.

(Read the current `Create`/`Update` SQL and renumber `$` placeholders carefully — the column was inserted before `published`.)

In `internal/subscriptions/service.go`, add to `ProductInfo`:
```go
	SandboxUpstream string // product's sandbox backend, "" = no sandbox
```
In `internal/subscriptions/repo.go` `GetProduct`, select the new column:
```go
		`SELECT id, context_path, upstream_url, sandbox_upstream_url, published FROM api_products WHERE id=$1`, id,
	).Scan(&p.ID, &p.ContextPath, &p.Upstream, &p.SandboxUpstream, &p.Published)
```

- [ ] **Step 5: Run to verify it passes**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/admin/ ./internal/subscriptions/ && go vet ./internal/admin/ ./internal/subscriptions/`
Expected: PASS (the migration applies on connect; `GetProduct` compiles with the new scan).

- [ ] **Step 6: Commit**

```bash
git add internal/db/migrations/0011_sandbox.sql internal/admin/product.go internal/admin/repo.go internal/admin/product_test.go internal/subscriptions/service.go internal/subscriptions/repo.go
git commit -m "feat(sandbox): sandbox_upstream_url + sandbox_api_key columns; product sandbox upstream"
```

---

## Task 3: Credential sandbox-key repo methods + Store extension

**Files:**
- Modify: `internal/subscriptions/repo.go`
- Modify: `internal/subscriptions/service.go` (extend `Store` interface)
- Test: `internal/subscriptions/repo_sandbox_test.go` (new, DB-backed)

**Interfaces:**
- Produces on `*Repo` (and added to `Store`):
  - `GetSandboxKey(ctx, appID int64) (string, error)` — decrypted sandbox key; `""` when the column is empty; `ErrNotFound` when the app has no credential row.
  - `UpdateSandboxKey(ctx, appID int64, key string) error` — encrypts + stores; `ErrNotFound` when no credential row.
  - `SandboxConsumersForProduct(ctx, productID int64) ([]string, error)` — usernames of active subscribers whose app has a non-empty sandbox key (for the sandbox route whitelist).
  - `SandboxConsumersForPlan(ctx, planID int64) ([]Credential, error)` — credential (username + **sandbox** key) of active subscribers on the plan whose app has a sandbox key.
  - `SandboxProductsForApp(ctx, appID int64) ([]ProductInfo, error)` — products the app is actively subscribed to that have a non-empty `sandbox_upstream_url` (id, context_path, sandbox upstream).

- [ ] **Step 1: Write the failing DB test**

Create `internal/subscriptions/repo_sandbox_test.go` (mirror the DB-setup convention of the existing subscriptions repo tests — connect via `db.Connect`, `db.Migrate`, seed user/app/product/plan/credential/subscription, `t.Cleanup`):
```go
package subscriptions

import (
	"context"
	"os"
	"testing"
	"time"

	"apisix-portal/internal/crypto"
	"apisix-portal/internal/db"
)

func sandboxTestRepo(t *testing.T) (context.Context, *Repo, int64, int64) {
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
	cipher, err := crypto.NewCipher(crypto.DevKey) // use the real dev cipher key constant/help; adjust to the actual constructor
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	repo := NewRepo(pool, cipher)
	suf := time.Now().Format("150405.000000000")
	var uid, appID, pid, planID int64
	pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,name) VALUES($1,'x','U') RETURNING id`, "sb+"+suf+"@e.com").Scan(&uid)
	pool.QueryRow(ctx, `INSERT INTO applications(owner_id,name) VALUES($1,'App') RETURNING id`, uid).Scan(&appID)
	pool.QueryRow(ctx, `INSERT INTO api_products(name,slug,category,context_path,sandbox_upstream_url,published) VALUES($1,$2,'C','/sb','echo:8080',true) RETURNING id`, "SbProd "+suf, "sbprod-"+suf).Scan(&pid)
	pool.QueryRow(ctx, `INSERT INTO plans(name,rate_limit_count,rate_limit_window_s) VALUES($1,5,60) RETURNING id`, "SbPlan "+suf).Scan(&planID)
	pool.Exec(ctx, `INSERT INTO subscriptions(application_id,api_product_id,plan_id,status) VALUES($1,$2,$3,'active')`, appID, pid, planID)
	if _, err := repo.GetOrCreateCredential(ctx, appID, GenerateKey); err != nil {
		t.Fatalf("seed credential: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM applications WHERE id=$1`, appID)
		pool.Exec(ctx, `DELETE FROM api_products WHERE id=$1`, pid)
		pool.Exec(ctx, `DELETE FROM plans WHERE id=$1`, planID)
		pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, uid)
	})
	return ctx, repo, appID, pid
}

func TestSandboxKeyRoundTripAndWhitelist(t *testing.T) {
	ctx, repo, appID, pid := sandboxTestRepo(t)

	// No sandbox key yet → not in the product's sandbox whitelist.
	if k, err := repo.GetSandboxKey(ctx, appID); err != nil || k != "" {
		t.Fatalf("GetSandboxKey before = %q, %v (want empty)", k, err)
	}
	names, err := repo.SandboxConsumersForProduct(ctx, pid)
	if err != nil || len(names) != 0 {
		t.Fatalf("whitelist before = %v, %v (want empty)", names, err)
	}

	if err := repo.UpdateSandboxKey(ctx, appID, "sbkey-123"); err != nil {
		t.Fatalf("UpdateSandboxKey: %v", err)
	}
	if k, err := repo.GetSandboxKey(ctx, appID); err != nil || k != "sbkey-123" {
		t.Fatalf("GetSandboxKey after = %q, %v", k, err)
	}
	names, err = repo.SandboxConsumersForProduct(ctx, pid)
	if err != nil || len(names) != 1 {
		t.Fatalf("whitelist after = %v, %v (want 1)", names, err)
	}
	prods, err := repo.SandboxProductsForApp(ctx, appID)
	if err != nil || len(prods) != 1 || prods[0].SandboxUpstream != "echo:8080" {
		t.Fatalf("SandboxProductsForApp = %+v, %v", prods, err)
	}
	creds, err := repo.SandboxConsumersForPlan(ctx, prods[0].ID) // not the plan id; replace below
	_ = creds
	_ = err
}
```
NOTE: fix the last two lines — call `SandboxConsumersForPlan` with the **plan id** (capture `planID` from the helper by returning it, or query it). Adjust the helper to also return `planID` and assert `SandboxConsumersForPlan(ctx, planID)` returns one credential whose `APIKey == "sbkey-123"`. Also confirm the real `crypto` cipher constructor + dev key symbol from `internal/crypto` and adjust the `crypto.NewCipher(...)` line accordingly.

- [ ] **Step 2: Run to verify it fails**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/subscriptions/ -run TestSandboxKeyRoundTrip -v`
Expected: FAIL — methods undefined.

- [ ] **Step 3: Implement the repo methods**

Add to `internal/subscriptions/repo.go`:
```go
// GetSandboxKey returns the application's decrypted sandbox key ("" when not
// enabled), or ErrNotFound when the app has no credential row.
func (r *Repo) GetSandboxKey(ctx context.Context, appID int64) (string, error) {
	var stored string
	err := r.pool.QueryRow(ctx,
		`SELECT sandbox_api_key FROM credentials WHERE application_id=$1`, appID).Scan(&stored)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if stored == "" {
		return "", nil
	}
	return r.cipher.Decrypt(stored)
}

// UpdateSandboxKey encrypts and stores the application's sandbox key.
func (r *Repo) UpdateSandboxKey(ctx context.Context, appID int64, key string) error {
	enc, err := r.cipher.Encrypt(key)
	if err != nil {
		return err
	}
	tag, err := r.pool.Exec(ctx,
		`UPDATE credentials SET sandbox_api_key=$2 WHERE application_id=$1`, appID, enc)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SandboxConsumersForProduct returns the usernames of active subscribers whose
// app has a sandbox key (the product's sandbox-route whitelist).
func (r *Repo) SandboxConsumersForProduct(ctx context.Context, productID int64) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT c.consumer_username FROM subscriptions s
		   JOIN credentials c ON c.application_id = s.application_id
		 WHERE s.api_product_id=$1 AND s.status='active' AND c.sandbox_api_key <> ''`, productID)
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

// SandboxConsumersForPlan returns the sandbox credential (username + sandbox key)
// of active subscribers on the plan whose app has a sandbox key.
func (r *Repo) SandboxConsumersForPlan(ctx context.Context, planID int64) ([]Credential, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT DISTINCT c.application_id, c.sandbox_api_key, c.consumer_username
		   FROM subscriptions s
		   JOIN credentials c ON c.application_id = s.application_id
		 WHERE s.plan_id=$1 AND s.status='active' AND c.sandbox_api_key <> ''`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Credential
	for rows.Next() {
		var c Credential
		var stored string
		if err := rows.Scan(&c.ApplicationID, &stored, &c.ConsumerUsername); err != nil {
			return nil, err
		}
		if c.APIKey, err = r.cipher.Decrypt(stored); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// SandboxProductsForApp returns the products the app is ACTIVELY subscribed to
// that have a sandbox upstream configured.
func (r *Repo) SandboxProductsForApp(ctx context.Context, appID int64) ([]ProductInfo, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT DISTINCT p.id, p.context_path, p.sandbox_upstream_url
		   FROM subscriptions s
		   JOIN api_products p ON p.id = s.api_product_id
		 WHERE s.application_id=$1 AND s.status='active' AND p.sandbox_upstream_url <> ''`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ProductInfo
	for rows.Next() {
		var p ProductInfo
		if err := rows.Scan(&p.ID, &p.ContextPath, &p.SandboxUpstream); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Extend the Store interface**

In `internal/subscriptions/service.go`, add to the `Store` interface:
```go
	GetSandboxKey(ctx context.Context, appID int64) (string, error)
	UpdateSandboxKey(ctx context.Context, appID int64, key string) error
	SandboxConsumersForProduct(ctx context.Context, productID int64) ([]string, error)
	SandboxConsumersForPlan(ctx context.Context, planID int64) ([]Credential, error)
	SandboxProductsForApp(ctx context.Context, appID int64) ([]ProductInfo, error)
```

- [ ] **Step 5: Run to verify it passes**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/subscriptions/ -run TestSandboxKeyRoundTrip -v && go vet ./internal/subscriptions/`
Expected: PASS. (The fake `Store` in `service_test.go` will fail to compile until Task 4 updates it — that's expected; run only this DB test here. If `var _ Store = (*Repo)(nil)` is in repo.go it must still compile, which it will once the methods exist.)

- [ ] **Step 6: Commit**

```bash
git add internal/subscriptions/repo.go internal/subscriptions/service.go internal/subscriptions/repo_sandbox_test.go
git commit -m "feat(sandbox): credential sandbox-key repo methods + Store extension"
```

---

## Task 4: Service second-gateway plumbing + EnableSandbox

**Files:**
- Modify: `internal/subscriptions/service.go`
- Modify: `internal/events/events.go` (new kinds)
- Modify: `internal/subscriptions/service_test.go` (update fake `Store` + `NewService` calls)
- Test: `internal/subscriptions/service_test.go`

**Interfaces:**
- Consumes: `Store` sandbox methods (Task 3); `apisix.Gateway`.
- Produces:
  - `NewService(store, gw, sandboxGW apisix.Gateway, genKey, eventLog)` — **new 3rd param** `sandboxGW` (nil = sandbox disabled).
  - `Service.EnableSandbox(ctx, appID int64) (string, error)`.
  - `Service.reprovisionSandboxRoute(ctx, productID int64, extra ...string) error` (unexported helper).
  - Errors `ErrSandboxNotConfigured`, `ErrNoSandboxEligibleSubscription`, `ErrNoSandboxKey`.
  - `events.KindSandboxEnabled = "sandbox_enabled"`, `events.KindSandboxKeyRotated = "sandbox_key_rotated"`.

- [ ] **Step 1: Add the event kinds**

In `internal/events/events.go`, add to the kind constants:
```go
	KindSandboxEnabled    = "sandbox_enabled"
	KindSandboxKeyRotated = "sandbox_key_rotated"
```

- [ ] **Step 2: Write the failing service test**

In `internal/subscriptions/service_test.go`:
1. Update the existing fake `Store` to implement the five Task-3 methods (add fields it needs):
```go
// add to the fake Store struct:
//   sandboxKeys map[int64]string            // appID -> sandbox key
//   sandboxProducts map[int64][]ProductInfo // appID -> sandbox-enabled active products
//   sandboxWhitelist map[int64][]string     // productID -> usernames with sandbox key
func (f *fakeStore) GetSandboxKey(_ context.Context, appID int64) (string, error) {
	return f.sandboxKeys[appID], nil
}
func (f *fakeStore) UpdateSandboxKey(_ context.Context, appID int64, key string) error {
	if f.sandboxKeys == nil { f.sandboxKeys = map[int64]string{} }
	f.sandboxKeys[appID] = key
	return nil
}
func (f *fakeStore) SandboxConsumersForProduct(_ context.Context, productID int64) ([]string, error) {
	return f.sandboxWhitelist[productID], nil
}
func (f *fakeStore) SandboxConsumersForPlan(_ context.Context, planID int64) ([]Credential, error) {
	return nil, nil
}
func (f *fakeStore) SandboxProductsForApp(_ context.Context, appID int64) ([]ProductInfo, error) {
	return f.sandboxProducts[appID], nil
}
```
(Match the actual fake's name/shape in the file — it may be a struct literal with method receivers; adapt field plumbing accordingly. Where the fake currently hard-codes `GetProduct`/`ActivePlanForApp`/`GetCredential`, ensure those return a credential + active plan for the test app so EnableSandbox's guards pass.)
2. Update **every** `NewService(...)` call in the test to pass a sandbox fake gateway as the new 3rd arg (use a second `apisix.NewFake()` / the package's fake constructor).
3. Add the test:
```go
func TestEnableSandboxProvisionsConsumerAndRoutes(t *testing.T) {
	store := newFakeStore() // app 42 has a credential (consumer app_42) + an active plan
	store.sandboxProducts = map[int64][]ProductInfo{42: {{ID: 9, ContextPath: "/sb", SandboxUpstream: "echo:8080"}}}
	store.sandboxWhitelist = map[int64][]string{9: {"app_42"}}
	prodGW, sbGW := apisix.NewFake(), apisix.NewFake()
	svc := NewService(store, prodGW, sbGW, func() string { return "sbkey" }, nil)

	key, err := svc.EnableSandbox(context.Background(), 42)
	if err != nil || key != "sbkey" {
		t.Fatalf("EnableSandbox = %q, %v", key, err)
	}
	if _, ok := sbGW.Consumers["app_42"]; !ok {
		t.Error("sandbox consumer app_42 not provisioned on the sandbox gateway")
	}
	if _, ok := prodGW.Consumers["app_42"]; ok {
		t.Error("production gateway must not be touched by EnableSandbox")
	}
	if store.sandboxKeys[42] != "sbkey" {
		t.Error("sandbox key not persisted")
	}
	// A sandbox route exists for product 9 on the sandbox gateway.
	if _, ok := sbGW.Routes[RouteID(9)]; !ok {
		t.Error("sandbox route for product 9 not provisioned")
	}
}

func TestEnableSandbox409WhenNoEligibleSubscription(t *testing.T) {
	store := newFakeStore() // app with credential + active plan but NO sandbox-enabled products
	store.sandboxProducts = map[int64][]ProductInfo{}
	svc := NewService(store, apisix.NewFake(), apisix.NewFake(), func() string { return "k" }, nil)
	if _, err := svc.EnableSandbox(context.Background(), 42); !errors.Is(err, ErrNoSandboxEligibleSubscription) {
		t.Fatalf("err = %v, want ErrNoSandboxEligibleSubscription", err)
	}
}
```
(Confirm the fake gateway exposes `Consumers`/`Routes` maps — the existing `apisix.Fake` already does per `service_test.go`; mirror the exact field names it uses.)

- [ ] **Step 3: Run to verify it fails**

Run: `go test ./internal/subscriptions/ -run TestEnableSandbox -v`
Expected: FAIL — `EnableSandbox`/`NewService` arity/errors undefined.

- [ ] **Step 4: Implement plumbing + EnableSandbox**

In `internal/subscriptions/service.go`:
- Add errors near the others:
```go
var ErrSandboxNotConfigured = errors.New("subscriptions: sandbox gateway not configured")
var ErrNoSandboxEligibleSubscription = errors.New("subscriptions: no active subscription to a sandbox-enabled product")
var ErrNoSandboxKey = errors.New("subscriptions: application has no sandbox key")
```
- Add `sandboxGW apisix.Gateway` to `Service` and the constructor:
```go
type Service struct {
	store     Store
	gw        apisix.Gateway
	sandboxGW apisix.Gateway
	genKey    func() string
	events    EventLogger
}

func NewService(store Store, gw, sandboxGW apisix.Gateway, genKey func() string, eventLog EventLogger) *Service {
	return &Service{store: store, gw: gw, sandboxGW: sandboxGW, genKey: genKey, events: eventLog}
}

func (s *Service) sandboxEnabled() bool { return s.sandboxGW != nil }
```
- Add the sandbox route helper (mirrors `reprovisionRoute` but uses the sandbox upstream + whitelist + sandbox gateway):
```go
// reprovisionSandboxRoute rebuilds the product's route on the SANDBOX gateway
// from its sandbox upstream and the set of active subscribers that have a
// sandbox key (plus any extras not yet reflected in the store). No-op when
// sandbox is disabled or the product has no sandbox upstream.
func (s *Service) reprovisionSandboxRoute(ctx context.Context, productID int64, extra ...string) error {
	if !s.sandboxEnabled() {
		return nil
	}
	prod, err := s.store.GetProduct(ctx, productID)
	if err != nil {
		return err
	}
	if prod.SandboxUpstream == "" {
		return s.sandboxGW.DeleteRoute(ctx, RouteID(prod.ID))
	}
	allowed, err := s.store.SandboxConsumersForProduct(ctx, productID)
	if err != nil {
		return err
	}
	for _, e := range extra {
		present := false
		for _, a := range allowed {
			if a == e {
				present = true
				break
			}
		}
		if !present {
			allowed = append(allowed, e)
		}
	}
	if len(allowed) == 0 {
		return s.sandboxGW.DeleteRoute(ctx, RouteID(prod.ID))
	}
	return s.sandboxGW.EnsureRoute(ctx, RouteID(prod.ID), prod.ContextPath, prod.SandboxUpstream, allowed)
}
```
- Add `EnableSandbox`:
```go
// EnableSandbox opts an application into sandbox: it generates (once) the app's
// sandbox key, provisions the sandbox consumer with the app's active plan limit
// on the sandbox gateway, and grants it on the sandbox route of every
// sandbox-enabled product the app actively subscribes to. Idempotent — a second
// call returns the existing key and re-asserts the gateway state.
func (s *Service) EnableSandbox(ctx context.Context, appID int64) (string, error) {
	if !s.sandboxEnabled() {
		return "", ErrSandboxNotConfigured
	}
	cred, err := s.store.GetCredential(ctx, appID)
	if errors.Is(err, ErrNotFound) {
		return "", ErrNoSandboxEligibleSubscription
	}
	if err != nil {
		return "", err
	}
	plan, err := s.store.ActivePlanForApp(ctx, appID)
	if errors.Is(err, ErrNoActiveSubscription) {
		return "", ErrNoSandboxEligibleSubscription
	}
	if err != nil {
		return "", err
	}
	prods, err := s.store.SandboxProductsForApp(ctx, appID)
	if err != nil {
		return "", err
	}
	if len(prods) == 0 {
		return "", ErrNoSandboxEligibleSubscription
	}
	key, err := s.store.GetSandboxKey(ctx, appID)
	if err != nil {
		return "", err
	}
	if key == "" {
		key = s.genKey()
	}
	if err := s.sandboxGW.EnsureConsumer(ctx, cred.ConsumerUsername, key,
		apisix.RateLimit{Count: plan.Count, WindowSeconds: plan.WindowSeconds}); err != nil {
		return "", err
	}
	if err := s.store.UpdateSandboxKey(ctx, appID, key); err != nil {
		return "", err
	}
	for _, p := range prods {
		if err := s.reprovisionSandboxRoute(ctx, p.ID, cred.ConsumerUsername); err != nil {
			return "", err
		}
	}
	s.logEvent(ctx, appID, events.KindSandboxEnabled, nil, nil)
	return key, nil
}
```

- [ ] **Step 5: Run to verify it passes**

Run: `go test ./internal/subscriptions/ && go vet ./internal/subscriptions/`
Expected: PASS (all existing tests compile with the new `NewService` arity; new tests pass).

- [ ] **Step 6: Commit**

```bash
git add internal/subscriptions/service.go internal/subscriptions/service_test.go internal/events/events.go
git commit -m "feat(sandbox): Service sandbox gateway plumbing + EnableSandbox"
```

---

## Task 5: RotateSandboxKey + lifecycle hooks

**Files:**
- Modify: `internal/subscriptions/service.go` (RotateSandboxKey; sandbox hooks in Approve/Reject/Unsubscribe/ReprovisionPlan)
- Modify: `internal/admin/service.go` (sandbox reprovision on sandbox-upstream change) + `internal/admin/service_test.go`
- Test: `internal/subscriptions/service_test.go`

**Interfaces:**
- Produces: `Service.RotateSandboxKey(ctx, appID int64) (string, error)`; sandbox provisioning maintained across Approve/Reject/Unsubscribe/ReprovisionPlan; `admin.Provisioner` gains `ReprovisionSandboxRoute(ctx, productID int64) error`.

- [ ] **Step 1: Write the failing tests**

In `internal/subscriptions/service_test.go` add:
```go
func TestRotateSandboxKey(t *testing.T) {
	store := newFakeStore() // app 42, credential, active plan, sandbox key "old"
	store.sandboxKeys = map[int64]string{42: "old"}
	sbGW := apisix.NewFake()
	sbGW.Consumers["app_42"] = apisix.FakeConsumer{} // pre-existing; adapt to the fake's type
	svc := NewService(store, apisix.NewFake(), sbGW, func() string { return "new" }, nil)

	key, err := svc.RotateSandboxKey(context.Background(), 42)
	if err != nil || key != "new" {
		t.Fatalf("RotateSandboxKey = %q, %v", key, err)
	}
	if store.sandboxKeys[42] != "new" {
		t.Error("sandbox key not updated in store")
	}
}

func TestRotateSandboxKey409WhenNoKey(t *testing.T) {
	store := newFakeStore()
	store.sandboxKeys = map[int64]string{} // no sandbox key
	svc := NewService(store, apisix.NewFake(), apisix.NewFake(), func() string { return "x" }, nil)
	if _, err := svc.RotateSandboxKey(context.Background(), 42); !errors.Is(err, ErrNoSandboxKey) {
		t.Fatalf("err = %v, want ErrNoSandboxKey", err)
	}
}
```
(Adapt the fake-consumer seeding to the real `apisix.Fake` API — if `EnsureConsumer` is all that's needed, you can drop the pre-seed line; the assertion is on the stored key + that no error occurs.)

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/subscriptions/ -run TestRotateSandbox -v`
Expected: FAIL — `RotateSandboxKey` undefined.

- [ ] **Step 3: Implement RotateSandboxKey + lifecycle hooks**

In `internal/subscriptions/service.go` add:
```go
// RotateSandboxKey issues a fresh sandbox key, installs it on the sandbox
// gateway consumer (old key 401s immediately), then persists it (gateway before
// DB). The limit is preserved from the app's active plan. 409 if the app has no
// sandbox key (ErrNoSandboxKey) or sandbox is disabled (ErrSandboxNotConfigured).
func (s *Service) RotateSandboxKey(ctx context.Context, appID int64) (string, error) {
	if !s.sandboxEnabled() {
		return "", ErrSandboxNotConfigured
	}
	cred, err := s.store.GetCredential(ctx, appID)
	if errors.Is(err, ErrNotFound) {
		return "", ErrNoSandboxKey
	}
	if err != nil {
		return "", err
	}
	existing, err := s.store.GetSandboxKey(ctx, appID)
	if err != nil {
		return "", err
	}
	if existing == "" {
		return "", ErrNoSandboxKey
	}
	plan, err := s.store.ActivePlanForApp(ctx, appID)
	if err != nil {
		return "", err
	}
	newKey := s.genKey()
	if err := s.sandboxGW.EnsureConsumer(ctx, cred.ConsumerUsername, newKey,
		apisix.RateLimit{Count: plan.Count, WindowSeconds: plan.WindowSeconds}); err != nil {
		return "", err
	}
	if err := s.store.UpdateSandboxKey(ctx, appID, newKey); err != nil {
		return "", err
	}
	s.logEvent(ctx, appID, events.KindSandboxKeyRotated, nil, nil)
	return newKey, nil
}
```
Now wire the sandbox hooks into the existing lifecycle methods (all guarded by `sandboxEnabled()`; the helper itself no-ops when the product has no sandbox upstream):
- In `Approve`, after `SetSubscriptionStatus(active)` succeeds and before the `logEvent`, add:
```go
	if s.sandboxEnabled() {
		if sk, err := s.store.GetSandboxKey(ctx, rec.AppID); err == nil && sk != "" {
			if err := s.reprovisionSandboxRoute(ctx, rec.ProductID, cred.ConsumerUsername); err != nil {
				return err
			}
		}
	}
```
- In `Reject`, after `ReprovisionRoute` — change the final `return s.ReprovisionRoute(...)` to capture the error, then also rebuild the sandbox route:
```go
	if err := s.ReprovisionRoute(ctx, rec.ProductID); err != nil {
		return err
	}
	return s.reprovisionSandboxRoute(ctx, rec.ProductID)
```
- In `Unsubscribe`, similarly chain the sandbox route after the prod reprovision:
```go
	if err := s.ReprovisionRoute(ctx, productID); err != nil {
		return err
	}
	return s.reprovisionSandboxRoute(ctx, productID)
```
- In `ReprovisionPlan`, after the existing prod consumer loop, add a sandbox loop:
```go
	if s.sandboxEnabled() {
		sbConsumers, err := s.store.SandboxConsumersForPlan(ctx, planID)
		if err != nil {
			return err
		}
		for _, c := range sbConsumers {
			if err := s.sandboxGW.EnsureConsumer(ctx, c.ConsumerUsername, c.APIKey,
				apisix.RateLimit{Count: plan.Count, WindowSeconds: plan.WindowSeconds}); err != nil {
				return err
			}
		}
	}
```
Add a public wrapper for the admin provisioner:
```go
// ReprovisionSandboxRoute rebuilds a product's sandbox route (used by admin on a
// sandbox-upstream change). No-op when sandbox is disabled.
func (s *Service) ReprovisionSandboxRoute(ctx context.Context, productID int64) error {
	return s.reprovisionSandboxRoute(ctx, productID)
}
```

- [ ] **Step 4: Admin reprovision on sandbox-upstream change**

In `internal/admin/service.go`:
- Extend the `Provisioner` interface:
```go
type Provisioner interface {
	ReprovisionRoute(ctx context.Context, productID int64) error
	ReprovisionSandboxRoute(ctx context.Context, productID int64) error
	DeprovisionRoute(ctx context.Context, productID int64) error
}
```
- In `Update`, after the existing `UpstreamURL`-change block, add a sandbox equivalent:
```go
	if updated.SandboxUpstreamURL != old.SandboxUpstreamURL {
		if err := s.prov.ReprovisionSandboxRoute(ctx, p.ID); err != nil {
			return Product{}, err
		}
	}
```
- In `internal/admin/service_test.go`, add `ReprovisionSandboxRoute` to the fake provisioner (return nil / record the call), and add a test asserting it is called when `SandboxUpstreamURL` changes and not when it is unchanged. (Mirror the existing `ReprovisionRoute`-on-upstream-change test.)

- [ ] **Step 5: Run to verify it passes**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/subscriptions/ ./internal/admin/ && go vet ./internal/subscriptions/ ./internal/admin/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/subscriptions/service.go internal/subscriptions/service_test.go internal/admin/service.go internal/admin/service_test.go
git commit -m "feat(sandbox): RotateSandboxKey + lifecycle hooks (approve/reject/unsubscribe/plan/upstream)"
```

---

## Task 6: HTTP endpoints (enable/rotate) + app-detail sandbox fields

**Files:**
- Modify: `internal/subscriptions/handler.go` (routes + handlers; `Reader` extension; detail fields)
- Modify: `internal/subscriptions/view.go` (`AppDetail`/`SubscriptionView` sandbox fields)
- Modify: `internal/subscriptions/repo.go` (`SubscriptionsForApp` returns `SandboxAvailable`)
- Test: `internal/subscriptions/handler_test.go`

**Interfaces:**
- Produces:
  - `POST /api/applications/{appID}/sandbox/enable` → `200 {sandboxApiKey}`; `409`.
  - `POST /api/applications/{appID}/sandbox/rotate` → `200 {sandboxApiKey}`; `409`.
  - `AppDetail.SandboxEnabled bool`, `AppDetail.SandboxGatewayUrl string`; `SubscriptionView.SandboxAvailable bool`.
  - `Reader` gains `GetSandboxKey(ctx, appID int64) (string, error)`.
  - `NewHandler(svc, reader, eventReader, owns, sandboxGatewayURL string)` — new trailing param.

- [ ] **Step 1: Write the failing handler tests**

In `internal/subscriptions/handler_test.go` add (mirror the existing rotateKey handler test + fakes; the fake `svc` is the real `*Service` built with fake store+gateways, or a thin seam — match how the file currently builds the handler):
```go
func TestSandboxEnableEndpoint(t *testing.T) {
	// build a handler whose service EnableSandbox returns "sbkey" for the owned app
	h := newTestHandlerSandboxOK(t) // helper: owns=true, service enables to "sbkey"
	req := authedReq(t, http.MethodPost, "/api/applications/42/sandbox/enable", nil) // mirror existing authed request helper
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}
	var out map[string]string
	json.Unmarshal(rec.Body.Bytes(), &out)
	if out["sandboxApiKey"] != "sbkey" {
		t.Fatalf("body=%s", rec.Body)
	}
}

func TestSandboxEnable409WhenIneligible(t *testing.T) {
	h := newTestHandlerSandboxIneligible(t) // service returns ErrNoSandboxEligibleSubscription
	req := authedReq(t, http.MethodPost, "/api/applications/42/sandbox/enable", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status=%d", rec.Code)
	}
}
```
(Use the file's existing helpers for building an authed request + a handler with a fake owner check. If the existing tests construct the handler with a real `*Service` over a fake store, set the fake store's sandbox fields so `EnableSandbox` succeeds/fails as needed, and pass a sandbox fake gateway + a sandbox gateway URL like `"http://localhost:9081"` to `NewHandler`/`NewService`.)

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/subscriptions/ -run TestSandbox -v`
Expected: FAIL — routes/handlers/arity undefined.

- [ ] **Step 3: Implement the endpoints + detail fields**

In `internal/subscriptions/view.go`:
```go
// add to SubscriptionView:
	SandboxAvailable bool `json:"sandboxAvailable"`
// add to AppDetail:
	SandboxEnabled    bool   `json:"sandboxEnabled"`
	SandboxGatewayUrl string `json:"sandboxGatewayUrl"`
```
In `internal/subscriptions/repo.go` `SubscriptionsForApp`, add `p.sandbox_upstream_url <> ''` as a selected boolean and scan it:
```go
		`SELECT s.api_product_id, p.name, p.version, p.context_path, s.plan_id, pl.name, s.status, (p.sandbox_upstream_url <> '')
		 FROM subscriptions s ...`)
// and in Scan:
		..., &v.Status, &v.SandboxAvailable)
```
In `internal/subscriptions/handler.go`:
- Add `GetSandboxKey` to the `Reader` interface.
- Add a `sandboxGatewayURL string` field to `Handler`; extend `NewHandler` to accept it as the trailing param and store it; register routes:
```go
	h.router.Post("/api/applications/{appID}/sandbox/enable", h.enableSandbox)
	h.router.Post("/api/applications/{appID}/sandbox/rotate", h.rotateSandbox)
```
- Handlers:
```go
func (h *Handler) enableSandbox(w http.ResponseWriter, r *http.Request) {
	appID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	key, err := h.svc.EnableSandbox(r.Context(), appID)
	if errors.Is(err, ErrNoSandboxEligibleSubscription) || errors.Is(err, ErrSandboxNotConfigured) {
		httpx.Error(w, http.StatusConflict, "sandbox unavailable — subscribe to a sandbox-enabled API first")
		return
	}
	if err != nil {
		log.Printf("enable sandbox failed (app=%d): %v", appID, err)
		httpx.Error(w, http.StatusInternalServerError, "enable sandbox failed")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"sandboxApiKey": key})
}

func (h *Handler) rotateSandbox(w http.ResponseWriter, r *http.Request) {
	appID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	key, err := h.svc.RotateSandboxKey(r.Context(), appID)
	if errors.Is(err, ErrNoSandboxKey) || errors.Is(err, ErrSandboxNotConfigured) {
		httpx.Error(w, http.StatusConflict, "no sandbox key to rotate — enable sandbox first")
		return
	}
	if err != nil {
		log.Printf("rotate sandbox key failed (app=%d): %v", appID, err)
		httpx.Error(w, http.StatusInternalServerError, "rotation failed")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"sandboxApiKey": key})
}
```
- In `detail`, after loading the credential, populate the sandbox fields:
```go
	if h.sandboxGatewayURL != "" {
		out.SandboxGatewayUrl = h.sandboxGatewayURL
		if sk, err := h.reader.GetSandboxKey(r.Context(), appID); err == nil && sk != "" {
			out.SandboxEnabled = true
		}
	}
```

- [ ] **Step 4: Run to verify it passes + full subscriptions suite**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/subscriptions/ && go vet ./internal/subscriptions/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/subscriptions/handler.go internal/subscriptions/view.go internal/subscriptions/repo.go internal/subscriptions/handler_test.go
git commit -m "feat(sandbox): enable/rotate endpoints + app-detail sandbox fields"
```

---

## Task 7: Try-it sandbox proxy

**Files:**
- Modify: `internal/tryit/tryit.go` (`Access.SandboxKey`; `Products` sandbox check)
- Modify: `internal/tryit/handler.go` (sandbox routes + proxy variant + context fields)
- Test: `internal/tryit/handler_test.go`

**Interfaces:**
- Produces:
  - `Access` gains `SandboxKey(ctx, appID int64) (string, error)` and `Products` gains a way to know the product has a sandbox upstream: extend `ProductBySlug` is risky (used widely) → add `SandboxUpstream(ctx, slug string) (bool, error)` to `Products`.
  - Routes `ANY /api/try/{slug}/{appId}/sandbox` and `/sandbox/*` → proxy to the sandbox gateway injecting the sandbox key.
  - `context` response gains `sandboxAvailable bool`.
  - `NewHandler(p, a, gatewayURL, sandboxGatewayURL string)` — new trailing param (`""` disables sandbox proxying).

- [ ] **Step 1: Write the failing test**

In `internal/tryit/handler_test.go` add (mirror the existing prod-proxy test that stands up an httptest backend as the "gateway" and asserts the injected `apikey`):
```go
func TestSandboxProxyInjectsSandboxKeyAndTargetsSandboxGateway(t *testing.T) {
	var gotKey, gotPath string
	sandbox := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("apikey")
		gotPath = r.URL.Path
		w.WriteHeader(200)
	}))
	defer sandbox.Close()
	// access: owns app, active sub, sandbox key "SB"; products: slug→(id,/echo), sandbox available
	h := NewHandler(fakeProducts{id: 9, ctx: "/echo", sandbox: true},
		fakeAccess{owns: true, status: "active", sandboxKey: "SB"},
		"http://prod.invalid", sandbox.URL)
	req := authedReq(t, http.MethodGet, "/api/try/orders/42/sandbox/ping", nil) // mirror existing authed helper
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status=%d", rec.Code)
	}
	if gotKey != "SB" {
		t.Errorf("apikey=%q want SB", gotKey)
	}
	if gotPath != "/echo/ping" {
		t.Errorf("path=%q want /echo/ping", gotPath)
	}
}
```
(Adapt `fakeProducts`/`fakeAccess` to the test file's existing fakes — add a `sandbox bool` to the products fake and a `sandboxKey string` + `SandboxKey`/`SandboxUpstream` methods. Reuse the file's authed-request helper.)

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/tryit/ -run TestSandboxProxy -v`
Expected: FAIL — sandbox route/method/arity undefined.

- [ ] **Step 3: Implement the sandbox proxy**

In `internal/tryit/tryit.go`:
```go
// add to Products:
	SandboxUpstream(ctx context.Context, slug string) (bool, error)
// add to Access:
	SandboxKey(ctx context.Context, appID int64) (string, error)
```
In `internal/tryit/handler.go`:
- Add `sandbox string` (the sandbox gateway base) to `Handler`; extend `NewHandler` to take `sandboxGatewayURL` (trailing) and store `strings.TrimRight(sandboxGatewayURL, "/")`.
- Register sandbox routes BEFORE the catch-all prod routes so they match first:
```go
	h.router.Handle("/api/try/{slug}/{appId}/sandbox", http.HandlerFunc(h.sandboxProxy))
	h.router.Handle("/api/try/{slug}/{appId}/sandbox/*", http.HandlerFunc(h.sandboxProxy))
	h.router.Handle("/api/try/{slug}/{appId}", http.HandlerFunc(h.proxy))
	h.router.Handle("/api/try/{slug}/{appId}/*", http.HandlerFunc(h.proxy))
```
- Refactor the shared proxy body into a helper `do(w, r, gatewayBase, key string, contextPath, rest string)` (extract from the current `proxy` — the part from "Build the gateway target" onward), and have both `proxy` and `sandboxProxy` call it. `sandboxProxy` differs only in: it requires `h.sandbox != ""` (else 404 "sandbox not available"), checks `h.products.SandboxUpstream(slug)` is true (else 404), resolves `key` via `h.access.SandboxKey(appID)` (403 when empty), and passes `h.sandbox` as the gateway base. The owner + active-subscription gates are identical to `proxy`. The wildcard remainder for sandbox is `chi.URLParam(r, "*")` exactly as prod.
- In `context`, add `sandboxAvailable` to the JSON: call `h.products.SandboxUpstream(slug)` (only when `h.sandbox != ""`, else false) and include it: `{"apps": apps, "sandboxAvailable": sbAvail}`.

(Keep all SSRF/header-stripping/timeout/body-cap behavior identical — the helper reuses the existing `stripHeaders`, `maxBodyBytes`, and `out.Header.Set("apikey", key)`.)

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/tryit/ && go vet ./internal/tryit/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tryit/tryit.go internal/tryit/handler.go internal/tryit/handler_test.go
git commit -m "feat(sandbox): try-it sandbox proxy variant"
```

---

## Task 8: Wire server/main + full suite + live verification

**Files:**
- Modify: `cmd/portal/main.go` (build sandbox admin client)
- Modify: `internal/server/server.go` (sandbox gateway + URLs into Service/handlers/tryit; new adapter methods)
- Modify: `internal/server/tryit_adapters.go` (SandboxKey + SandboxUpstream)
- Test: full backend suite + live

**Interfaces:**
- Consumes: everything above.
- Produces: a wired portal where sandbox is active when `cfg.SandboxConfigured()`.

- [ ] **Step 1: Build the sandbox client in main**

In `cmd/portal/main.go`, the sandbox admin client is built inside `server.New` from `cfg` (preferred — `New` already takes `cfg`). So leave `main.go`'s `gw` as is; no change needed unless `New`'s signature requires the client. (If you instead choose to build it in main, construct `var sandboxGW apisix.Gateway; if cfg.SandboxConfigured() { sandboxGW = apisix.NewClient(cfg.APISIXSandboxAdminURL, cfg.APISIXSandboxAdminKey) }` and pass it through.)

- [ ] **Step 2: Wire in server.go**

In `internal/server/server.go` `New`, after `gw` is available, build the sandbox gateway from cfg:
```go
	var sandboxGW apisix.Gateway
	if cfg.SandboxConfigured() {
		sandboxGW = apisix.NewClient(cfg.APISIXSandboxAdminURL, cfg.APISIXSandboxAdminKey)
	}
```
- Update `subscriptions.NewService(subRepo, gw, sandboxGW, subscriptions.GenerateKey, eventRepo)` (new 3rd arg).
- Update `subscriptions.NewHandler(subSvc, subRepo, eventRepo, owns, sandboxGatewayURL)` where `sandboxGatewayURL := ""; if cfg.SandboxConfigured() { sandboxGatewayURL = cfg.APISIXSandboxGatewayURL }`.
- Update `tryit.NewHandler(tryProducts, tryAccess, cfg.APISIXGatewayURL, sandboxGatewayURL)`.

In `internal/server/tryit_adapters.go`:
- Add `SandboxKey(ctx, appID)` to the access adapter → `subRepo.GetSandboxKey(appID)` (map `ErrNotFound`→ return `"", nil` so a missing credential reads as "no sandbox key", yielding 403 at the proxy).
- Add `SandboxUpstream(ctx, slug)` to the products adapter → resolve the product by slug and report `sandbox_upstream_url <> ''`. Add a catalog read for this: `catalog.Repo` gains `SandboxUpstreamBySlug(ctx, slug) (bool, error)` (published-only; false when missing), or extend the existing `ProductBySlug` adapter path. Implement the catalog method:
```go
// in internal/catalog/repo.go
func (r *Repo) SandboxUpstreamBySlug(ctx context.Context, slug string) (bool, error) {
	var has bool
	err := r.pool.QueryRow(ctx,
		`SELECT sandbox_upstream_url <> '' FROM api_products WHERE slug=$1 AND published=true`, slug).Scan(&has)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return has, err
}
```

- [ ] **Step 3: Build + full backend suite**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go build ./... && go test ./internal/... ./cmd/... && go vet ./...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add cmd/portal/main.go internal/server/server.go internal/server/tryit_adapters.go internal/catalog/repo.go
git commit -m "feat(sandbox): wire sandbox gateway into server + try-it adapters"
```

- [ ] **Step 5: Live verification**

1. `docker compose up -d` (now includes `apisix-sandbox`); restart the portal so migration 0011 applies and the sandbox client is built.
2. As admin, set a product's sandbox upstream (e.g. `echo:8080` on `pizzashackapi`) via `PUT /api/admin/products/{id}` with `sandboxUpstreamUrl`.
3. As an approved subscriber of that product:
   - `POST /api/applications/{id}/sandbox/enable` → `200 {sandboxApiKey}`.
   - Call the sandbox gateway with the sandbox key:
     `curl -H "apikey: <SANDBOX_KEY>" http://localhost:9081/<context_path>/get` → 200 from the sandbox backend.
   - The **production** key on the sandbox gateway → 401 (distinct credentials).
   - `POST /api/applications/{id}/sandbox/rotate` → old sandbox key 401s, new one 200.
   - `GET /api/applications/{id}` shows `sandboxEnabled:true`, `sandboxGatewayUrl`, and `sandboxAvailable:true` on the subscription.
4. Confirm production is untouched: the production gateway (`:9080`) still serves the production key and never the sandbox upstream.

---

## Self-Review notes

- **Spec coverage:** dedicated sandbox gateway + config (T1) ✅; `sandbox_upstream_url` + `sandbox_api_key` + product sandbox upstream (T2) ✅; sandbox credential repo + whitelist queries (T3) ✅; second-gateway plumbing + EnableSandbox (T4) ✅; RotateSandboxKey + lifecycle hooks incl. admin upstream-change (T5) ✅; enable/rotate endpoints + app-detail sandbox fields (T6) ✅; try-it sandbox proxy (T7) ✅; wiring + live (T8) ✅. Frontend (Credentials sandbox card, Try-it toggle, admin Composer field) is **Plan 2**, authored after this ships.
- **Type consistency:** `NewService(store, gw, sandboxGW, genKey, eventLog)` used consistently (T4 defines, T8 calls); `EnableSandbox`/`RotateSandboxKey(ctx, appID)(string,error)`; `reprovisionSandboxRoute(ctx, productID, extra...)`; Store sandbox methods identical between repo (T3) and interface (T3) and fakes (T4/T5); `Reader.GetSandboxKey` (T6); tryit `Access.SandboxKey`/`Products.SandboxUpstream` (T7); `admin.Provisioner.ReprovisionSandboxRoute` (T5) satisfied by `Service.ReprovisionSandboxRoute` (T5).
- **Implementer notes:** confirm the real `apisix.Fake` field names (`Consumers`/`Routes`) and `crypto` cipher constructor/dev-key symbol before writing tests (T3/T4/T5). Renumber `$` placeholders carefully in admin `Create`/`Update` (T2). Register sandbox try-it routes before the prod catch-all (T7). The sandbox admin key defaulting to the prod key keeps the existing dev-secrets guard unchanged (T1).
