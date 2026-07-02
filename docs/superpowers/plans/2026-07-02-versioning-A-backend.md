# API Versioning / Changelog — Plan A (Backend) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a per-product lifecycle status (active/deprecated/sunset) that blocks new subscriptions when deprecated/sunset, plus an admin-authored changelog with a public read endpoint.

**Architecture:** A migration adds `lifecycle_status`/`sunset_date` to `api_products` + a `changelog_entries` table. The catalog exposes the status + a public changelog endpoint; the subscriptions service rejects new subs to deprecated/sunset products; the admin package persists the status fields + does changelog CRUD. All new routes nest inside the already-mounted catalog (`/api/products/`) and admin (`/api/admin/products/`) handlers — no server wiring.

**Tech Stack:** Go 1.25, pgx/pgxpool, chi, Postgres.

## Global Constraints

- Module `apisix-portal`. Touches `internal/db/migrations`, `internal/catalog`, `internal/subscriptions`, `internal/admin`.
- **Statuses:** `active` | `deprecated` | `sunset`. Both `deprecated` and `sunset` block **new** subscriptions (409); existing subscriptions + all other actions are status-agnostic.
- **Changelog entry:** `version` (text), `kind` ∈ `added|changed|fixed|removed|deprecated|security`, `notes` (text), `entry_date` (date). Admin-authored; public read is newest-first.
- Dates cross the API as ISO `YYYY-MM-DD` strings; scan DATE columns via `to_char(col,'YYYY-MM-DD')` so they read as nullable `*string` (NULL → nil), and write via `NULLIF($n,'')::date`.
- Deprecated/sunset products stay **published/listed** (badged by the frontend), not hidden. The changelog endpoint is **public** and only serves published products.
- Tests: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/... ./cmd/...`; `gofmt -w` every touched file; `go vet ./...`.

---

## Task 1: Migration `0014_versioning.sql`

**Files:**
- Create: `internal/db/migrations/0014_versioning.sql`
- Test: `internal/db/migrate_versioning_test.go`

**Interfaces:**
- Produces: `api_products.lifecycle_status TEXT NOT NULL DEFAULT 'active'` (CHECK in enum) + `sunset_date DATE` (nullable); table `changelog_entries(id, product_id→api_products ON DELETE CASCADE, version, kind CHECK, notes, entry_date, created_at)` + index `(product_id, entry_date DESC)`.

- [ ] **Step 1: Write the failing test**

Create `internal/db/migrate_versioning_test.go`:
```go
package db

import (
	"context"
	"os"
	"testing"
)

func TestVersioningSchema(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://portal:portal@localhost:5432/portal?sslmode=disable"
	}
	ctx := context.Background()
	pool, err := Connect(ctx, url)
	if err != nil {
		t.Skipf("no database: %v", err)
	}
	defer pool.Close()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// lifecycle_status default + CHECK
	var def string
	if err := pool.QueryRow(ctx,
		`SELECT column_default FROM information_schema.columns WHERE table_name='api_products' AND column_name='lifecycle_status'`).Scan(&def); err != nil {
		t.Fatalf("lifecycle_status column: %v", err)
	}
	// CHECK rejects a bad status
	if _, err := pool.Exec(ctx, `UPDATE api_products SET lifecycle_status='bogus' WHERE id=(SELECT id FROM api_products LIMIT 1)`); err == nil {
		t.Error("expected CHECK to reject bogus lifecycle_status")
	}
	// changelog_entries exists + kind CHECK
	if _, err := pool.Exec(ctx,
		`INSERT INTO changelog_entries(product_id, version, kind, notes, entry_date)
		 SELECT id, 'v1', 'boguskind', '', '2026-01-01' FROM api_products LIMIT 1`); err == nil {
		t.Error("expected CHECK to reject bogus kind")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/db/ -run TestVersioningSchema -v`
Expected: FAIL — `lifecycle_status`/`changelog_entries` don't exist.

- [ ] **Step 3: Write the migration**

Create `internal/db/migrations/0014_versioning.sql`:
```sql
ALTER TABLE api_products
  ADD COLUMN lifecycle_status TEXT NOT NULL DEFAULT 'active'
    CHECK (lifecycle_status IN ('active','deprecated','sunset')),
  ADD COLUMN sunset_date DATE;

CREATE TABLE changelog_entries (
    id         BIGSERIAL PRIMARY KEY,
    product_id BIGINT NOT NULL REFERENCES api_products(id) ON DELETE CASCADE,
    version    TEXT NOT NULL,
    kind       TEXT NOT NULL CHECK (kind IN ('added','changed','fixed','removed','deprecated','security')),
    notes      TEXT NOT NULL DEFAULT '',
    entry_date DATE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_changelog_product ON changelog_entries(product_id, entry_date DESC);
```

- [ ] **Step 4: Run to verify it passes**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/db/ -run TestVersioningSchema -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/db/migrations/0014_versioning.sql internal/db/migrate_versioning_test.go
git commit -m "feat(versioning): migration — lifecycle status + changelog_entries"
```

---

## Task 2: Catalog — status fields + public changelog endpoint

**Files:**
- Modify: `internal/catalog/product.go`, `internal/catalog/repo.go`, `internal/catalog/handler.go`
- Test: `internal/catalog/repo_test.go`, `internal/catalog/handler_test.go` (extend)

**Interfaces:**
- Produces:
  - `Product` gains `LifecycleStatus string` (`json:"lifecycleStatus"`) + `SunsetDate *string` (`json:"sunsetDate"`).
  - `ChangelogEntry { Version, Kind, Notes, Date string }` (`json:"version|kind|notes|date"`).
  - `Repo.ListChangelogBySlug(ctx, slug string) ([]ChangelogEntry, error)` — newest-first; `ErrNotFound` when the published product doesn't exist.
  - Route `GET /api/products/{slug}/changelog`.

- [ ] **Step 1: Write the failing tests**

Extend `internal/catalog/repo_test.go`:
```go
func TestListChangelogBySlug(t *testing.T) {
	ctx, repo := testRepo(t) // existing helper in this package's tests
	var pid int64
	if err := repo.pool.QueryRow(ctx,
		`INSERT INTO api_products(name,slug,category,context_path,published,lifecycle_status)
		 VALUES('CL','cl-slug','C','/cl',true,'deprecated') RETURNING id`).Scan(&pid); err != nil {
		t.Fatalf("seed product: %v", err)
	}
	t.Cleanup(func() { _, _ = repo.pool.Exec(ctx, `DELETE FROM api_products WHERE id=$1`, pid) })
	_, _ = repo.pool.Exec(ctx,
		`INSERT INTO changelog_entries(product_id,version,kind,notes,entry_date) VALUES
		 ($1,'v1.0','added','first','2026-01-01'),($1,'v1.1','fixed','patch','2026-02-01')`, pid)
	entries, err := repo.ListChangelogBySlug(ctx, "cl-slug")
	if err != nil || len(entries) != 2 {
		t.Fatalf("entries=%d err=%v", len(entries), err)
	}
	if entries[0].Version != "v1.1" { // newest-first
		t.Errorf("order wrong: %+v", entries)
	}
	if _, err := repo.ListChangelogBySlug(ctx, "no-such-slug"); err != ErrNotFound {
		t.Errorf("unknown slug err = %v, want ErrNotFound", err)
	}
}
```
(If `testRepo(t)` isn't the existing helper name in `repo_test.go`, use whatever that file uses to get a `*Repo` + ctx; read the file first.)

Extend `internal/catalog/handler_test.go` — the fake `Lister` gains `ListChangelogBySlug`, and a test hits `GET /api/products/{slug}/changelog`:
```go
func TestChangelogEndpoint(t *testing.T) {
	h := NewHandler(fakeLister{changelog: map[string][]ChangelogEntry{
		"cl-slug": {{Version: "v1.1", Kind: "fixed", Notes: "patch", Date: "2026-02-01"}},
	}})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/products/cl-slug/changelog", nil))
	if rec.Code != 200 {
		t.Fatalf("code=%d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "v1.1") {
		t.Errorf("body missing entry: %s", rec.Body.String())
	}
}
```
(Add a `changelog map[string][]ChangelogEntry` field + `ListChangelogBySlug` method to the existing `fakeLister` in that file; return `ErrNotFound` for a missing slug.)

- [ ] **Step 2: Run to verify it fails**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/catalog/ -run 'TestListChangelog|TestChangelog' -v`
Expected: FAIL — type/method/route undefined.

- [ ] **Step 3: Add the fields, repo methods, and route**

In `internal/catalog/product.go`, add to `Product`:
```go
	LifecycleStatus string  `json:"lifecycleStatus"`
	SunsetDate      *string `json:"sunsetDate"`
```
And add the changelog type:
```go
type ChangelogEntry struct {
	Version string `json:"version"`
	Kind    string `json:"kind"`
	Notes   string `json:"notes"`
	Date    string `json:"date"`
}
```

In `internal/catalog/repo.go`:
- Extend `baseSelect` (append the two columns, keeping column order aligned with the scan):
```go
const baseSelect = `SELECT id, name, slug, category, version, context_path, description, tags, icon, rating, rating_count, auth_type, lifecycle_status, to_char(sunset_date,'YYYY-MM-DD')
	FROM api_products WHERE published = true`
```
- In `scanProducts`, add the two scan targets at the END (matching the appended columns):
```go
			&p.AuthType,
			&p.LifecycleStatus,
			&p.SunsetDate,
```
- Add the changelog method:
```go
func (r *Repo) ListChangelogBySlug(ctx context.Context, slug string) ([]ChangelogEntry, error) {
	var exists bool
	if err := r.pool.QueryRow(ctx,
		`SELECT true FROM api_products WHERE slug=$1 AND published=true`, slug).Scan(&exists); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	rows, err := r.pool.Query(ctx,
		`SELECT ce.version, ce.kind, ce.notes, to_char(ce.entry_date,'YYYY-MM-DD')
		 FROM changelog_entries ce JOIN api_products p ON p.id = ce.product_id
		 WHERE p.slug=$1 ORDER BY ce.entry_date DESC, ce.id DESC`, slug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ChangelogEntry
	for rows.Next() {
		var e ChangelogEntry
		if err := rows.Scan(&e.Version, &e.Kind, &e.Notes, &e.Date); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
```

In `internal/catalog/handler.go`:
- Add `ListChangelogBySlug(ctx, slug string) ([]ChangelogEntry, error)` to the `Lister` interface.
- Register the route in `NewHandler`: `h.router.Get("/api/products/{slug}/changelog", h.getChangelog)`.
- Add the handler (mirror `getSpec`):
```go
func (h *Handler) getChangelog(w http.ResponseWriter, r *http.Request) {
	entries, err := h.repo.ListChangelogBySlug(r.Context(), chi.URLParam(r, "slug"))
	if errors.Is(err, ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "product not found")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not load changelog")
		return
	}
	if entries == nil {
		entries = []ChangelogEntry{}
	}
	httpx.JSON(w, http.StatusOK, entries)
}
```
(Confirm the real `httpx.JSON`/`httpx.Error` + chi import already used in this file.)

- [ ] **Step 4: Run to verify it passes**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/catalog/ && go vet ./internal/catalog/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/catalog/
git commit -m "feat(versioning): catalog exposes lifecycle status + public changelog endpoint"
```

---

## Task 3: Subscribe — block deprecated/sunset products

**Files:**
- Modify: `internal/subscriptions/repo.go` (ProductInfo + GetProduct), `internal/subscriptions/service.go` (Subscribe), `internal/subscriptions/handler.go` (409 mapping), `internal/subscriptions/errors.go` (or wherever sentinels live)
- Test: `internal/subscriptions/service_test.go`

**Interfaces:**
- Consumes: `lifecycle_status` column (Task 1).
- Produces: `ProductInfo.LifecycleStatus string`; `ErrProductDeprecated`; `Subscribe` returns it for deprecated/sunset; handler maps → 409.

- [ ] **Step 1: Write the failing test**

In `internal/subscriptions/service_test.go` add (mirror the existing subscribe tests; `memStore.products` holds `ProductInfo`):
```go
func TestSubscribeRejectsDeprecated(t *testing.T) {
	store := newMemStore()
	store.products[3] = ProductInfo{ID: 3, ContextPath: "/p", Upstream: "echo:8080", Published: true, LifecycleStatus: "deprecated"}
	svc := NewService(store, apisix.NewFake(), nil, func() string { return "k" }, nil)
	if _, err := svc.Subscribe(context.Background(), 1, 3, 2); !errors.Is(err, ErrProductDeprecated) {
		t.Fatalf("subscribe deprecated err = %v, want ErrProductDeprecated", err)
	}
	store.products[3] = ProductInfo{ID: 3, ContextPath: "/p", Upstream: "echo:8080", Published: true, LifecycleStatus: "sunset"}
	if _, err := svc.Subscribe(context.Background(), 1, 3, 2); !errors.Is(err, ErrProductDeprecated) {
		t.Fatalf("subscribe sunset err = %v, want ErrProductDeprecated", err)
	}
	store.products[3] = ProductInfo{ID: 3, ContextPath: "/p", Upstream: "echo:8080", Published: true, LifecycleStatus: "active"}
	if _, err := svc.Subscribe(context.Background(), 1, 3, 2); err != nil {
		t.Fatalf("subscribe active err = %v, want nil", err)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/subscriptions/ -run TestSubscribeRejectsDeprecated -v`
Expected: FAIL — `LifecycleStatus`/`ErrProductDeprecated` undefined.

- [ ] **Step 3: Add the field, sentinel, check, and 409**

- In `internal/subscriptions/repo.go`, add `LifecycleStatus string` to the `ProductInfo` struct, and select it in `GetProduct`:
```go
		`SELECT id, context_path, upstream_url, sandbox_upstream_url, published, auth_type, lifecycle_status FROM api_products WHERE id=$1`, id,
	).Scan(&p.ID, &p.ContextPath, &p.Upstream, &p.SandboxUpstream, &p.Published, &p.AuthType, &p.LifecycleStatus)
```
- Add the sentinel next to the other `Err*` in the package (e.g. `errors.go` / `service.go`):
```go
// ErrProductDeprecated is returned when a NEW subscription is attempted on a
// deprecated or sunset product. Existing subscriptions are unaffected.
var ErrProductDeprecated = errors.New("product no longer accepts new subscriptions")
```
- In `service.go` `Subscribe`, right after the `!prod.Published` check:
```go
	if prod.LifecycleStatus == "deprecated" || prod.LifecycleStatus == "sunset" {
		return Credential{}, ErrProductDeprecated
	}
```
- In `handler.go`, wherever `Subscribe`'s error is mapped to a status, add a case mapping `ErrProductDeprecated` → `http.StatusConflict` with the message `"This API no longer accepts new subscriptions."` (put it alongside the existing `ErrAlreadySubscribed`→409 / `ErrNotFound`→404 handling — read the current mapping and extend it).

- [ ] **Step 4: Run to verify it passes**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/subscriptions/ && go vet ./internal/subscriptions/`
Expected: PASS (existing subscribe tests unaffected — their fixtures have `LifecycleStatus: ""`, which is neither deprecated nor sunset, so allowed).

- [ ] **Step 5: Commit**

```bash
git add internal/subscriptions/
git commit -m "feat(versioning): block new subscriptions to deprecated/sunset products"
```

---

## Task 4: Admin — status fields + changelog CRUD

**Files:**
- Modify: `internal/admin/product.go`, `internal/admin/repo.go`, `internal/admin/handler.go`, `internal/admin/service.go`
- Test: `internal/admin/repo_test.go`, `internal/admin/handler_test.go` (extend)

**Interfaces:**
- Produces:
  - `admin.Product` gains `LifecycleStatus string` (`json:"lifecycleStatus"`) + `SunsetDate *string` (`json:"sunsetDate"`); persisted by Create/Update.
  - `ChangelogEntry { ID int64; Version, Kind, Notes, Date string }`.
  - `Repo.AddChangelog(ctx, productID int64, e ChangelogEntry) (ChangelogEntry, error)`; `Repo.DeleteChangelog(ctx, productID, entryID int64) error` (`ErrNotFound` when absent).
  - Routes `POST /api/admin/products/{id}/changelog`, `DELETE /api/admin/products/{id}/changelog/{entryId}`.

- [ ] **Step 1: Write the failing tests**

Extend `internal/admin/repo_test.go`:
```go
func TestProductLifecycleRoundTrip(t *testing.T) {
	ctx, repo := adminTestRepo(t) // existing helper; read the file for the real name
	sunset := "2026-12-31"
	p, err := repo.Create(ctx, Product{Name: "Lc", Slug: "lc-" + uniqueSuffix(t), Category: "C", ContextPath: "/lc", Published: true, LifecycleStatus: "sunset", SunsetDate: &sunset})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { _, _ = repo.pool.Exec(ctx, `DELETE FROM api_products WHERE id=$1`, p.ID) })
	if p.LifecycleStatus != "sunset" || p.SunsetDate == nil || *p.SunsetDate != "2026-12-31" {
		t.Fatalf("lifecycle round-trip: %+v", p)
	}
}

func TestChangelogAddDelete(t *testing.T) {
	ctx, repo := adminTestRepo(t)
	p, _ := repo.Create(ctx, Product{Name: "Cl", Slug: "cl-" + uniqueSuffix(t), Category: "C", ContextPath: "/cl2", Published: true})
	t.Cleanup(func() { _, _ = repo.pool.Exec(ctx, `DELETE FROM api_products WHERE id=$1`, p.ID) })
	e, err := repo.AddChangelog(ctx, p.ID, ChangelogEntry{Version: "v2", Kind: "changed", Notes: "n", Date: "2026-03-01"})
	if err != nil || e.ID == 0 {
		t.Fatalf("add: %+v %v", e, err)
	}
	if err := repo.DeleteChangelog(ctx, p.ID, e.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := repo.DeleteChangelog(ctx, p.ID, e.ID); err != ErrNotFound {
		t.Fatalf("delete missing err = %v, want ErrNotFound", err)
	}
}
```
(Use the file's real repo-test helper + unique-suffix idiom — read `repo_test.go` first.)

Extend `internal/admin/handler_test.go` with a POST changelog test (mirror the existing product-create handler test's auth/JSON setup) asserting a 201/200 and that the service was called; and a create/update test asserting `lifecycleStatus` is accepted.

- [ ] **Step 2: Run to verify it fails**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/admin/ -run 'TestProductLifecycle|TestChangelog' -v`
Expected: FAIL — fields/methods/routes undefined.

- [ ] **Step 3: Implement**

In `internal/admin/product.go`, add to `Product`:
```go
	LifecycleStatus string  `json:"lifecycleStatus"`
	SunsetDate      *string `json:"sunsetDate"`
```
Add a `ChangelogEntry`:
```go
type ChangelogEntry struct {
	ID      int64  `json:"id"`
	Version string `json:"version"`
	Kind    string `json:"kind"`
	Notes   string `json:"notes"`
	Date    string `json:"date"`
}
```

In `internal/admin/repo.go`:
- Extend `productCols` (append, matching `scanProduct`): `... auth_type, lifecycle_status, to_char(sunset_date,'YYYY-MM-DD')`.
- In `scanProduct`, add `&p.LifecycleStatus, &p.SunsetDate` at the end (a `*string` scan target for the nullable date).
- In `Create`, add the two columns + params. The status defaults to `active`; the date is nullable:
```go
		`INSERT INTO api_products(name, slug, category, version, context_path, description, tags, icon, upstream_url, sandbox_upstream_url, published, openapi_spec, auth_type, lifecycle_status, sunset_date)
		 VALUES($1,$2,$3,COALESCE(NULLIF($4,''),'1.0.0'),$5,$6,$7,$8,$9,$10,$11,$12,COALESCE(NULLIF($13,''),'key-auth'),COALESCE(NULLIF($14,''),'active'),NULLIF($15,'')::date)
		 RETURNING `+productCols,
		p.Name, p.Slug, p.Category, p.Version, p.ContextPath, p.Description, p.Tags, p.Icon, p.UpstreamURL, p.SandboxUpstreamURL, p.Published, p.OpenAPISpec, p.AuthType, p.LifecycleStatus, derefStr(p.SunsetDate)))
```
- In `Update`, add `lifecycle_status=COALESCE(NULLIF($15,''),'active'), sunset_date=NULLIF($16,'')::date` and the two params (`p.LifecycleStatus, derefStr(p.SunsetDate)`).
- Add the helper + changelog methods:
```go
func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (r *Repo) AddChangelog(ctx context.Context, productID int64, e ChangelogEntry) (ChangelogEntry, error) {
	err := r.pool.QueryRow(ctx,
		`INSERT INTO changelog_entries(product_id, version, kind, notes, entry_date)
		 VALUES($1,$2,$3,$4,$5::date)
		 RETURNING id, version, kind, notes, to_char(entry_date,'YYYY-MM-DD')`,
		productID, e.Version, e.Kind, e.Notes, e.Date).
		Scan(&e.ID, &e.Version, &e.Kind, &e.Notes, &e.Date)
	return e, err
}

func (r *Repo) DeleteChangelog(ctx context.Context, productID, entryID int64) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM changelog_entries WHERE id=$1 AND product_id=$2`, entryID, productID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
```

In `internal/admin/service.go` + `internal/admin/handler.go`:
- Validate `LifecycleStatus` on product create/update: empty or one of the enum; else 400. Validate `SunsetDate` (when present) parses as `YYYY-MM-DD`; else 400. (Add to the existing product validation.)
- Add the changelog routes + handlers (mirror the existing admin product routes; both admin-only via the existing `requireAdmin` mount):
  - `POST /api/admin/products/{id}/changelog` — decode `{version, kind, notes, date}`, validate `kind` ∈ enum + `date` parses + `version` non-empty (400 otherwise), call `AddChangelog`, return 201 + the entry.
  - `DELETE /api/admin/products/{id}/changelog/{entryId}` — parse ids, call `DeleteChangelog`, 404 on `ErrNotFound`, else 204.
  Wire these into the admin handler's router alongside the product routes. (Read `handler.go` to match its chi router + JSON/error helpers.)

- [ ] **Step 4: Run to verify it passes**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/admin/ && go vet ./internal/admin/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/admin/
git commit -m "feat(versioning): admin status fields + changelog CRUD"
```

---

## Task 5: Full suite + live verification

**Files:** none (verification only, unless a wiring gap surfaces).

- [ ] **Step 1: Build + full backend suite**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go build ./... && go test ./internal/... ./cmd/... && go vet ./...`
Expected: all green. (The changelog routes nest inside the already-mounted `/api/products/` catalog handler and `/api/admin/products/` admin handler, so no `server.go` change is expected — if `go build`/a route test reveals a mount gap, add the minimal mount and note it.)

- [ ] **Step 2: Live verification**

Bring the stack up (`docker compose up -d postgres apisix`), run the portal. With `curl` (register a developer; log in as an admin — e.g. an existing admin account):
1. As admin, `PUT /api/admin/products/{id}` (or create) setting `{"lifecycleStatus":"deprecated"}` on a published product → 200 with `lifecycleStatus:"deprecated"`.
2. `POST /api/admin/products/{id}/changelog {"version":"v1.2","kind":"deprecated","notes":"…","date":"2026-07-01"}` → 201; add a second entry.
3. As a developer, `GET /api/products/{slug}` → `lifecycleStatus:"deprecated"`; `GET /api/products/{slug}/changelog` → the two entries **newest-first**.
4. As a developer, `POST /api/applications/{appId}/subscriptions {productId, planId}` for the deprecated product → **409** ("no longer accepts new subscriptions").
5. Set another product `{"lifecycleStatus":"sunset","sunsetDate":"2026-12-31"}` → `GET …/{slug}` shows the status + date; subscribing → 409.
6. An **active** product still subscribes normally (2xx).
7. `DELETE /api/admin/products/{id}/changelog/{entryId}` → 204; the changelog endpoint drops it.
**Look at the output.**

- [ ] **Step 3: No commit** (verification only; note results in the progress ledger).

---

## Self-Review notes

- **Spec coverage:** migration (T1) ✅; catalog `lifecycleStatus`/`sunsetDate` + public changelog endpoint (T2) ✅; subscribe blocks deprecated & sunset → 409 (T3) ✅; admin status fields + changelog CRUD (T4) ✅; full suite + live (T5) ✅. Deferred items (multi-version, auto-retire, deprecation emails, edit-in-place, markdown) intentionally absent.
- **Type consistency:** the catalog `ChangelogEntry {version,kind,notes,date}` (read view) vs the admin `ChangelogEntry {id,version,kind,notes,date}` (write view + id) are deliberately separate types in their own packages — the frontend read shape matches catalog. `LifecycleStatus string` + `SunsetDate *string` names match across catalog `Product` (T2), subscriptions `ProductInfo` (T3, status only), and admin `Product` (T4). `ErrProductDeprecated` is defined once (T3) and mapped in the same task's handler. Date columns are consistently `to_char(...,'YYYY-MM-DD')` on read and `NULLIF($n,'')::date`/`$n::date` on write.
- **Implementer notes:** read each file for the real test-helper names (`testRepo`/`adminTestRepo`/`uniqueSuffix`) + the real `httpx`/chi/JSON-decode helpers before adapting — the code blocks use conventional names but the repo is the source of truth. Existing subscribe tests pass because their `ProductInfo` fixtures have `LifecycleStatus: ""` (allowed). No `server.go` wiring expected (routes nest in mounted handlers).
