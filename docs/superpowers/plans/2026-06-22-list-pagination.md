# List Pagination Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add offset/page-based pagination to all six list endpoints, returning a `{items,total,page,pageSize}` envelope, and wire the React frontend (client + minimal prev/next UI) to consume it.

**Architecture:** A new neutral `internal/paging` package parses `?page`/`?pageSize` and builds a generic envelope. Each list repo gains a `paging.Params` argument and returns `(items, total, error)` — `total` from a `COUNT(*)` over the same filters, the page from a `LIMIT/OFFSET` query. Handlers wrap the result in `paging.New(...)`. The frontend client returns `Paginated<T>`; a shared `Pagination` component gives prev/next controls on the pages that render lists.

**Tech Stack:** Go 1.25 (generics), Chi router, pgx/pgxpool, React + TypeScript, Vitest.

## Global Constraints

- Go module path is `apisix-portal`; import the new package as `apisix-portal/internal/paging`.
- `pageSize`: default 20, floor 1, **cap 100**. `page`: default 1, floor 1. Non-numeric/garbage → default.
- Envelope JSON shape is exactly `{ "items": [...], "total": int, "page": int, "pageSize": int }`; `items` is never `null`.
- Out-of-range page returns empty `items` with the true `total`.
- Repos must NOT import `net/http` — `paging` depends only on `net/url` + `strconv`.
- Keep existing error handling, logging, monotonic-guard patterns, and French UI copy unchanged.
- Run `gofmt` on every Go file you touch. Backend tests: `go test ./...`. Frontend tests: `cd web && pnpm test`.

---

### Task 1: `paging` package

**Files:**
- Create: `internal/paging/paging.go`
- Test: `internal/paging/paging_test.go`

**Interfaces:**
- Produces: `paging.Params{Page,Size int}` with `Limit() int` / `Offset() int`; `paging.Parse(url.Values) Params`; `paging.Page[T]{Items []T; Total,Page,PageSize int}`; `paging.New[T](items []T, total int, p Params) Page[T]`.

- [ ] **Step 1: Write the failing test**

```go
package paging

import (
	"net/url"
	"testing"
)

func TestParseDefaults(t *testing.T) {
	p := Parse(url.Values{})
	if p.Page != 1 || p.Size != 20 {
		t.Fatalf("defaults: got page=%d size=%d, want 1/20", p.Page, p.Size)
	}
}

func TestParseClampsAndReads(t *testing.T) {
	cases := []struct{ page, size, wantPage, wantSize string }{}
	_ = cases
	got := Parse(url.Values{"page": {"3"}, "pageSize": {"50"}})
	if got.Page != 3 || got.Size != 50 {
		t.Fatalf("got %+v, want page=3 size=50", got)
	}
	if c := Parse(url.Values{"pageSize": {"500"}}); c.Size != 100 {
		t.Fatalf("size cap: got %d, want 100", c.Size)
	}
	if c := Parse(url.Values{"page": {"0"}, "pageSize": {"0"}}); c.Page != 1 || c.Size != 20 {
		t.Fatalf("floor: got %+v, want 1/20", c)
	}
	if c := Parse(url.Values{"page": {"abc"}, "pageSize": {"x"}}); c.Page != 1 || c.Size != 20 {
		t.Fatalf("garbage: got %+v, want 1/20", c)
	}
}

func TestLimitOffset(t *testing.T) {
	p := Params{Page: 3, Size: 20}
	if p.Limit() != 20 || p.Offset() != 40 {
		t.Fatalf("got limit=%d offset=%d, want 20/40", p.Limit(), p.Offset())
	}
}

func TestNewNormalizesNilAndMapsFields(t *testing.T) {
	pg := New[int](nil, 7, Params{Page: 2, Size: 20})
	if pg.Items == nil {
		t.Fatal("Items must be non-nil")
	}
	if pg.Total != 7 || pg.Page != 2 || pg.PageSize != 20 {
		t.Fatalf("got %+v", pg)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/paging/`
Expected: FAIL (package/functions undefined).

- [ ] **Step 3: Write minimal implementation**

```go
// Package paging provides offset/page-based pagination primitives shared by the
// list endpoints: request parsing, a Params value, and a JSON response envelope.
package paging

import (
	"net/url"
	"strconv"
)

const (
	DefaultPage     = 1
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// Params is a validated page request.
type Params struct {
	Page int // >= 1
	Size int // 1..MaxPageSize
}

// Limit is the SQL LIMIT for this page.
func (p Params) Limit() int { return p.Size }

// Offset is the SQL OFFSET for this page.
func (p Params) Offset() int { return (p.Page - 1) * p.Size }

// Parse reads ?page and ?pageSize, applying defaults, floors, and the size cap.
// Non-numeric or out-of-range values fall back to defaults.
func Parse(v url.Values) Params {
	page := DefaultPage
	if n, err := strconv.Atoi(v.Get("page")); err == nil && n >= 1 {
		page = n
	}
	size := DefaultPageSize
	if n, err := strconv.Atoi(v.Get("pageSize")); err == nil && n >= 1 {
		size = n
	}
	if size > MaxPageSize {
		size = MaxPageSize
	}
	return Params{Page: page, Size: size}
}

// Page is the JSON envelope returned by paginated list endpoints. Items is
// always a non-nil slice so the JSON renders "items": [] rather than null.
type Page[T any] struct {
	Items    []T `json:"items"`
	Total    int `json:"total"`
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
}

// New builds a Page envelope, normalizing a nil items slice to empty.
func New[T any](items []T, total int, p Params) Page[T] {
	if items == nil {
		items = []T{}
	}
	return Page[T]{Items: items, Total: total, Page: p.Page, PageSize: p.Size}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/paging/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/paging/paging.go internal/paging/paging_test.go
git add internal/paging
git commit -m "feat(paging): offset pagination params + JSON envelope"
```

---

### Task 2: Paginate `GET /api/products` (catalog)

**Files:**
- Modify: `internal/catalog/repo.go` (List signature + filter helper)
- Modify: `internal/catalog/handler.go` (Lister interface + list handler)
- Test: `internal/catalog/handler_test.go`, `internal/catalog/repo_test.go` (update to new signature)

**Interfaces:**
- Consumes: `paging.Params`, `paging.New`.
- Produces: `catalog.Lister.List(ctx, q Query, p paging.Params) ([]Product, int, error)`.

- [ ] **Step 1: Update the handler test to assert the envelope**

In `internal/catalog/handler_test.go`, the fake repo's `List` must match the new signature and the test must assert the envelope. Replace the fake's `List` method and the list-path assertion with:

```go
// fake repo method:
func (f *fakeRepo) List(_ context.Context, _ catalog.Query, p paging.Params) ([]catalog.Product, int, error) {
	return f.items, len(f.items), f.err
}
```

```go
// in the list test, after decoding the body into a paging.Page[catalog.Product]:
var got paging.Page[catalog.Product]
json.NewDecoder(rec.Body).Decode(&got)
if got.Total != len(f.items) || got.Page != 1 || got.PageSize != 20 {
	t.Fatalf("envelope meta wrong: %+v", got)
}
if len(got.Items) != len(f.items) {
	t.Fatalf("got %d items, want %d", len(got.Items), len(f.items))
}
```

Add `"apisix-portal/internal/paging"` to the test imports. Adjust the exact fake/struct names to those already in the file.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/catalog/`
Expected: FAIL (signature mismatch / undefined `paging`).

- [ ] **Step 3: Update the repo**

In `internal/catalog/repo.go` add `"apisix-portal/internal/paging"` to imports, extract the filter builder, and rewrite `List`:

```go
// filterClause builds the shared WHERE tail (after "published = true") and its
// args, so the count and page queries always apply the same filters.
func filterClause(q Query) (string, []any) {
	sql := ""
	args := []any{}
	if q.Category != "" {
		args = append(args, q.Category)
		sql += fmt.Sprintf(" AND category = $%d", len(args))
	}
	if q.Tag != "" {
		args = append(args, q.Tag)
		sql += fmt.Sprintf(" AND $%d = ANY(tags)", len(args))
	}
	if q.Search != "" {
		args = append(args, "%"+q.Search+"%")
		n := len(args)
		sql += fmt.Sprintf(" AND (name ILIKE $%d OR description ILIKE $%d)", n, n)
	}
	return sql, args
}

// List returns one page of published products plus the total matching the same
// filters.
func (r *Repo) List(ctx context.Context, q Query, p paging.Params) ([]Product, int, error) {
	filter, args := filterClause(q)

	var total int
	if err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM api_products WHERE published = true`+filter, args...,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	sql := baseSelect + filter
	if q.Sort == "alpha" {
		sql += " ORDER BY name ASC"
	} else {
		sql += " ORDER BY rating DESC, name ASC"
	}
	args = append(args, p.Limit(), p.Offset())
	sql += fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)-1, len(args))

	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items, err := scanProducts(rows)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}
```

`GetBySlug` is unchanged (`baseSelect` still ends at `published = true`).

- [ ] **Step 4: Update the handler**

In `internal/catalog/handler.go` add `"apisix-portal/internal/paging"` to imports, update the interface and handler:

```go
type Lister interface {
	List(ctx context.Context, q Query, p paging.Params) ([]Product, int, error)
	GetBySlug(ctx context.Context, slug string) (Product, error)
}
```

```go
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	q := Query{
		Search:   r.URL.Query().Get("search"),
		Category: r.URL.Query().Get("category"),
		Tag:      r.URL.Query().Get("tag"),
		Sort:     r.URL.Query().Get("sort"),
	}
	p := paging.Parse(r.URL.Query())
	items, total, err := h.repo.List(r.Context(), q, p)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to list products")
		return
	}
	httpx.JSON(w, http.StatusOK, paging.New(items, total, p))
}
```

- [ ] **Step 5: Update `repo_test.go` if it calls `List` directly**

If `internal/catalog/repo_test.go` calls `repo.List(ctx, q)`, change each call to `repo.List(ctx, q, paging.Params{Page: 1, Size: 20})` and capture the extra `total` return (`items, total, err := ...`). Add the `paging` import. Assert `total` where a count is meaningful.

- [ ] **Step 6: Run tests**

Run: `go test ./internal/catalog/`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
gofmt -w internal/catalog/*.go
git add internal/catalog
git commit -m "feat(catalog): paginate GET /api/products with envelope + total"
```

---

### Task 3: Paginate `GET /api/plans`

**Files:**
- Modify: `internal/plans/repo.go`, `internal/plans/handler.go`
- Test: `internal/plans/handler_test.go`, `internal/plans/repo_test.go`

**Interfaces:**
- Produces: `plans.Lister.List(ctx, p paging.Params) ([]Plan, int, error)`.

- [ ] **Step 1: Update handler test fake + assertion**

In `internal/plans/handler_test.go`, change the fake's `List` to:

```go
func (f *fakePlanRepo) List(_ context.Context, p paging.Params) ([]plans.Plan, int, error) {
	return f.plans, len(f.plans), f.err
}
```

and decode the body as `paging.Page[plans.Plan]`, asserting `Total`, `Page==1`, `PageSize==20`, and `len(Items)`. Add the `apisix-portal/internal/paging` import. Match existing fake/struct names.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/plans/`
Expected: FAIL.

- [ ] **Step 3: Update the repo**

In `internal/plans/repo.go` add the `paging` import and rewrite `List`:

```go
func (r *Repo) List(ctx context.Context, p paging.Params) ([]Plan, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM plans`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, rate_limit_count, rate_limit_window_s FROM plans
		 ORDER BY rate_limit_count ASC LIMIT $1 OFFSET $2`, p.Limit(), p.Offset())
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Plan
	for rows.Next() {
		var pl Plan
		if err := rows.Scan(&pl.ID, &pl.Name, &pl.RateLimit, &pl.WindowSeconds); err != nil {
			return nil, 0, err
		}
		out = append(out, pl)
	}
	return out, total, rows.Err()
}
```

- [ ] **Step 4: Update the handler**

In `internal/plans/handler.go` add the `paging` import and update:

```go
type Lister interface {
	List(ctx context.Context, p paging.Params) ([]Plan, int, error)
}
```

```go
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	p := paging.Parse(r.URL.Query())
	items, total, err := h.repo.List(r.Context(), p)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to list plans")
		return
	}
	httpx.JSON(w, http.StatusOK, paging.New(items, total, p))
}
```

- [ ] **Step 5: Fix `repo_test.go` direct calls** (same pattern as Task 2 Step 5).

- [ ] **Step 6: Run tests**

Run: `go test ./internal/plans/`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
gofmt -w internal/plans/*.go
git add internal/plans
git commit -m "feat(plans): paginate GET /api/plans"
```

---

### Task 4: Paginate `GET /api/applications`

**Files:**
- Modify: `internal/applications/repo.go`, `internal/applications/handler.go`
- Test: `internal/applications/handler_test.go`, `internal/applications/repo_test.go`

**Interfaces:**
- Produces: `applications.Store.ListByOwner(ctx, ownerID int64, p paging.Params) ([]Application, int, error)`.

- [ ] **Step 1: Update handler test fake + assertion**

In `internal/applications/handler_test.go`, change the fake store's `ListByOwner` to:

```go
func (f *fakeStore) ListByOwner(_ context.Context, _ int64, p paging.Params) ([]applications.Application, int, error) {
	return f.apps, len(f.apps), f.err
}
```

Decode the GET body as `paging.Page[applications.Application]` and assert `Total`/`Page`/`PageSize`/`len(Items)`. Add the `paging` import. Match existing names.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/applications/`
Expected: FAIL.

- [ ] **Step 3: Update the repo**

In `internal/applications/repo.go` add the `paging` import and rewrite `ListByOwner`:

```go
func (r *Repo) ListByOwner(ctx context.Context, ownerID int64, p paging.Params) ([]Application, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM applications WHERE owner_id=$1`, ownerID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id,owner_id,name,description,created_at FROM applications
		 WHERE owner_id=$1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		ownerID, p.Limit(), p.Offset())
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Application
	for rows.Next() {
		var a Application
		if err := rows.Scan(&a.ID, &a.OwnerID, &a.Name, &a.Description, &a.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, a)
	}
	return out, total, rows.Err()
}
```

- [ ] **Step 4: Update the handler**

In `internal/applications/handler.go` add the `paging` import and update:

```go
type Store interface {
	Create(ctx context.Context, ownerID int64, name, description string) (Application, error)
	ListByOwner(ctx context.Context, ownerID int64, p paging.Params) ([]Application, int, error)
}
```

```go
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	p := paging.Parse(r.URL.Query())
	apps, total, err := h.store.ListByOwner(r.Context(), auth.UserID(r.Context()), p)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to list applications")
		return
	}
	httpx.JSON(w, http.StatusOK, paging.New(apps, total, p))
}
```

- [ ] **Step 5: Fix `repo_test.go` direct calls** (same pattern as Task 2 Step 5).

- [ ] **Step 6: Run tests**

Run: `go test ./internal/applications/`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
gofmt -w internal/applications/*.go
git add internal/applications
git commit -m "feat(applications): paginate GET /api/applications"
```

---

### Task 5: Paginate `GET /api/admin/products`

**Files:**
- Modify: `internal/admin/repo.go` (`ListAll`), `internal/admin/service.go` (`Store` + `Service.List`), `internal/admin/handler.go` (`ProductService` + list handler)
- Test: `internal/admin/handler_test.go`, `internal/admin/service_test.go`, `internal/admin/repo_test.go` (if present)

**Interfaces:**
- Produces: `admin.Store.ListAll(ctx, p paging.Params) ([]Product, int, error)`; `admin.ProductService.List(ctx, p paging.Params) ([]Product, int, error)`.

- [ ] **Step 1: Update tests (fakes + assertions)**

In `internal/admin/service_test.go`, change the fake store's `ListAll` to `ListAll(ctx, p paging.Params) ([]Product, int, error)` returning `(f.products, len(f.products), nil)`. In `internal/admin/handler_test.go`, change the fake `ProductService.List` likewise and decode the list body as `paging.Page[admin.Product]`, asserting `Total`/`Page`/`PageSize`/`len(Items)`. Add the `paging` import to both. Match existing names.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/admin/`
Expected: FAIL.

- [ ] **Step 3: Update the repo**

In `internal/admin/repo.go` add the `paging` import and rewrite `ListAll`:

```go
func (r *Repo) ListAll(ctx context.Context, p paging.Params) ([]Product, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM api_products`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx,
		`SELECT `+productCols+` FROM api_products ORDER BY name ASC LIMIT $1 OFFSET $2`,
		p.Limit(), p.Offset())
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Product
	for rows.Next() {
		pr, err := scanProduct(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, pr)
	}
	return out, total, rows.Err()
}
```

- [ ] **Step 4: Update the service**

In `internal/admin/service.go` add the `paging` import, update the `Store` interface line and `Service.List`:

```go
// in Store interface:
ListAll(ctx context.Context, p paging.Params) ([]Product, int, error)
```

```go
func (s *Service) List(ctx context.Context, p paging.Params) ([]Product, int, error) {
	return s.store.ListAll(ctx, p)
}
```

- [ ] **Step 5: Update the handler**

In `internal/admin/handler.go` add the `paging` import, update `ProductService.List` and the list handler:

```go
// in ProductService interface:
List(ctx context.Context, p paging.Params) ([]Product, int, error)
```

```go
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	p := paging.Parse(r.URL.Query())
	items, total, err := h.svc.List(r.Context(), p)
	if err != nil {
		log.Printf("admin list products: %v", err)
		httpx.Error(w, http.StatusInternalServerError, "failed to list products")
		return
	}
	httpx.JSON(w, http.StatusOK, paging.New(items, total, p))
}
```

- [ ] **Step 6: Run tests**

Run: `go test ./internal/admin/`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
gofmt -w internal/admin/repo.go internal/admin/service.go internal/admin/handler.go internal/admin/*_test.go
git add internal/admin/repo.go internal/admin/service.go internal/admin/handler.go internal/admin/service_test.go internal/admin/handler_test.go
git commit -m "feat(admin): paginate GET /api/admin/products"
```

---

### Task 6: Paginate `GET /api/admin/plans`

**Files:**
- Modify: `internal/admin/plan_repo.go` (`ListPlans`), `internal/admin/plan_service.go` (`PlanStore` + `PlanService.List`), `internal/admin/plan_handler.go` (`PlanAdminService` + list handler)
- Test: `internal/admin/plan_handler_test.go`, `internal/admin/plan_service_test.go`

**Interfaces:**
- Produces: `admin.PlanStore.ListPlans(ctx, p paging.Params) ([]Plan, int, error)`; `admin.PlanAdminService.List(ctx, p paging.Params) ([]Plan, int, error)`.

- [ ] **Step 1: Update tests (fakes + assertions)**

In `internal/admin/plan_service_test.go`, change the fake store's `ListPlans` to `ListPlans(ctx, p paging.Params) ([]Plan, int, error)`. In `internal/admin/plan_handler_test.go`, change the fake `PlanAdminService.List` likewise and decode the list body as `paging.Page[admin.Plan]`, asserting envelope fields. Add the `paging` import. Match existing names.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/admin/`
Expected: FAIL.

- [ ] **Step 3: Update the repo**

In `internal/admin/plan_repo.go` add the `paging` import and rewrite `ListPlans`:

```go
func (r *PlanRepo) ListPlans(ctx context.Context, p paging.Params) ([]Plan, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM plans`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx,
		`SELECT `+planCols+` FROM plans ORDER BY rate_limit_count ASC LIMIT $1 OFFSET $2`,
		p.Limit(), p.Offset())
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Plan
	for rows.Next() {
		pl, err := scanPlan(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, pl)
	}
	return out, total, rows.Err()
}
```

- [ ] **Step 4: Update the service**

In `internal/admin/plan_service.go` add the `paging` import, update `PlanStore.ListPlans` and `PlanService.List`:

```go
// in PlanStore interface:
ListPlans(ctx context.Context, p paging.Params) ([]Plan, int, error)
```

```go
func (s *PlanService) List(ctx context.Context, p paging.Params) ([]Plan, int, error) {
	return s.store.ListPlans(ctx, p)
}
```

- [ ] **Step 5: Update the handler**

In `internal/admin/plan_handler.go` add the `paging` import, update `PlanAdminService.List` and the list handler:

```go
// in PlanAdminService interface:
List(ctx context.Context, p paging.Params) ([]Plan, int, error)
```

```go
func (h *PlanHandler) list(w http.ResponseWriter, r *http.Request) {
	p := paging.Parse(r.URL.Query())
	items, total, err := h.svc.List(r.Context(), p)
	if err != nil {
		log.Printf("admin list plans: %v", err)
		httpx.Error(w, http.StatusInternalServerError, "failed to list plans")
		return
	}
	httpx.JSON(w, http.StatusOK, paging.New(items, total, p))
}
```

- [ ] **Step 6: Run tests**

Run: `go test ./internal/admin/`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
gofmt -w internal/admin/plan_repo.go internal/admin/plan_service.go internal/admin/plan_handler.go internal/admin/plan_*_test.go
git add internal/admin/plan_repo.go internal/admin/plan_service.go internal/admin/plan_handler.go internal/admin/plan_handler_test.go internal/admin/plan_service_test.go
git commit -m "feat(admin): paginate GET /api/admin/plans"
```

---

### Task 7: Paginate `GET /api/admin/subscriptions`

**Files:**
- Modify: `internal/subscriptions/repo.go` (`AdminSubscriptions`), `internal/subscriptions/service.go` (`Store` interface + `Service.AdminSubscriptions`), `internal/subscriptions/admin_handler.go` (`AdminService` + list handler)
- Test: `internal/subscriptions/admin_handler_test.go`, `internal/subscriptions/repo_test.go`, and any fake `Store` in `internal/subscriptions/service_test.go`

**Interfaces:**
- Produces: `subscriptions.Store.AdminSubscriptions(ctx, statusFilter string, p paging.Params) ([]AdminSubscriptionView, int, error)`; `subscriptions.AdminService.AdminSubscriptions(ctx, statusFilter string, p paging.Params) ([]AdminSubscriptionView, int, error)`.

- [ ] **Step 1: Update tests (fakes + assertions)**

In `internal/subscriptions/admin_handler_test.go`, change the fake service's `AdminSubscriptions` to the new signature returning `(views, len(views), nil)`, and decode the list body as `paging.Page[subscriptions.AdminSubscriptionView]`, asserting `Total`/`Page`/`PageSize`/`len(Items)`. If `internal/subscriptions/service_test.go` defines a fake `Store`, add `, p paging.Params` and a `, int` return to its `AdminSubscriptions`. Add the `paging` import to both. Match existing names.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/subscriptions/`
Expected: FAIL.

- [ ] **Step 3: Update the repo**

In `internal/subscriptions/repo.go` add the `paging` import and rewrite `AdminSubscriptions`:

```go
func (r *Repo) AdminSubscriptions(ctx context.Context, statusFilter string, p paging.Params) ([]AdminSubscriptionView, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM subscriptions s WHERE ($1 = '' OR s.status = $1)`, statusFilter,
	).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx,
		`SELECT s.id, a.name, u.email, p.name, p.version, pl.name, s.status, s.created_at
		 FROM subscriptions s
		 JOIN applications a ON a.id = s.application_id
		 JOIN users u ON u.id = a.owner_id
		 JOIN api_products p ON p.id = s.api_product_id
		 JOIN plans pl ON pl.id = s.plan_id
		 WHERE ($1 = '' OR s.status = $1)
		 ORDER BY s.created_at DESC LIMIT $2 OFFSET $3`, statusFilter, p.Limit(), p.Offset())
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []AdminSubscriptionView
	for rows.Next() {
		var v AdminSubscriptionView
		if err := rows.Scan(&v.ID, &v.ApplicationName, &v.OwnerEmail, &v.ProductName, &v.Version, &v.PlanName, &v.Status, &v.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, v)
	}
	return out, total, rows.Err()
}
```

- [ ] **Step 4: Update the service (Store interface + method)**

In `internal/subscriptions/service.go`, update the `Store` interface line and `Service.AdminSubscriptions`:

```go
// in Store interface (add the paging import to the file):
AdminSubscriptions(ctx context.Context, statusFilter string, p paging.Params) ([]AdminSubscriptionView, int, error)
```

```go
func (s *Service) AdminSubscriptions(ctx context.Context, statusFilter string, p paging.Params) ([]AdminSubscriptionView, int, error) {
	return s.store.AdminSubscriptions(ctx, statusFilter, p)
}
```

- [ ] **Step 5: Update the handler**

In `internal/subscriptions/admin_handler.go` add the `paging` import, update `AdminService.AdminSubscriptions` and the list handler:

```go
// in AdminService interface:
AdminSubscriptions(ctx context.Context, statusFilter string, p paging.Params) ([]AdminSubscriptionView, int, error)
```

```go
func (h *AdminHandler) list(w http.ResponseWriter, r *http.Request) {
	p := paging.Parse(r.URL.Query())
	items, total, err := h.svc.AdminSubscriptions(r.Context(), r.URL.Query().Get("status"), p)
	if err != nil {
		log.Printf("admin list subscriptions: %v", err)
		httpx.Error(w, http.StatusInternalServerError, "failed to list subscriptions")
		return
	}
	httpx.JSON(w, http.StatusOK, paging.New(items, total, p))
}
```

- [ ] **Step 6: Fix `repo_test.go` direct calls** to `AdminSubscriptions` (add `paging.Params{Page:1,Size:20}` arg, capture `total`).

- [ ] **Step 7: Run tests + full backend build**

Run: `go test ./...`
Expected: PASS across all packages (confirms no other caller of the changed signatures was missed).

- [ ] **Step 8: Commit**

```bash
gofmt -w internal/subscriptions/repo.go internal/subscriptions/service.go internal/subscriptions/admin_handler.go internal/subscriptions/*_test.go
git add internal/subscriptions
git commit -m "feat(subscriptions): paginate GET /api/admin/subscriptions"
```

---

### Task 8: Frontend types + client

**Files:**
- Modify: `web/src/api/types.ts` (add `Paginated<T>`)
- Modify: `web/src/api/client.ts` (list fns return envelope, accept page opts)
- Test: `web/src/api/client.test.ts`

**Interfaces:**
- Produces: `Paginated<T>{ items: T[]; total: number; page: number; pageSize: number }`; updated signatures:
  - `getProducts(q: ProductQuery, page?: PageOpts): Promise<Paginated<Product>>`
  - `getPlans(page?: PageOpts): Promise<Paginated<Plan>>`
  - `getApplications(token: string, page?: PageOpts): Promise<Paginated<Application>>`
  - `adminGetProducts(token: string, page?: PageOpts): Promise<Paginated<AdminProduct>>`
  - `adminGetPlans(token: string, page?: PageOpts): Promise<Paginated<Plan>>`
  - `adminGetSubscriptions(token: string, status?: string, page?: PageOpts): Promise<Paginated<AdminSubscription>>`
  - where `PageOpts = { page?: number; pageSize?: number }`.

- [ ] **Step 1: Update client tests**

In `web/src/api/client.test.ts`, update each list test to (a) mock `fetch` resolving an envelope body `{ items: [...], total: N, page: 1, pageSize: 20 }`, and (b) assert the returned value's `.items`. Example for products:

```ts
it('GETs /api/products and returns the items array', async () => {
  ;(globalThis.fetch as any) = vi.fn().mockResolvedValue({
    ok: true, json: async () => ({ items: [{ id: 1 }], total: 1, page: 1, pageSize: 20 }),
  })
  const res = await getProducts({})
  expect((globalThis.fetch as any).mock.calls[0][0]).toBe('/api/products')
  expect(res.items).toHaveLength(1)
  expect(res.total).toBe(1)
})
```

Add a test that page opts append query params:

```ts
it('getProducts forwards page + pageSize', async () => {
  ;(globalThis.fetch as any) = vi.fn().mockResolvedValue({
    ok: true, json: async () => ({ items: [], total: 0, page: 2, pageSize: 10 }),
  })
  await getProducts({}, { page: 2, pageSize: 10 })
  const url = (globalThis.fetch as any).mock.calls[0][0] as string
  expect(url).toContain('page=2')
  expect(url).toContain('pageSize=10')
})
```

Apply the same envelope shape to the plans/applications/admin tests in the file. Keep the existing `adminGetSubscriptions` `status=pending` URL assertion, but its body is now an envelope.

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && pnpm test src/api/client.test.ts`
Expected: FAIL (functions still return arrays).

- [ ] **Step 3: Add the `Paginated` type**

In `web/src/api/types.ts` append:

```ts
// Envelope returned by every paginated list endpoint.
export interface Paginated<T> {
  items: T[]
  total: number
  page: number
  pageSize: number
}
```

- [ ] **Step 4: Update the client**

In `web/src/api/client.ts`, add `Paginated` to the type import and a shared helper + updated list functions:

```ts
import type {
  Product, AuthResponse, ProductQuery, Plan, Application, Credential, AppDetail,
  AdminProduct, AdminSubscription, Usage, UsageRange, Paginated,
} from './types'

export interface PageOpts { page?: number; pageSize?: number }

// appendPage adds page/pageSize to an existing URLSearchParams when provided.
function appendPage(params: URLSearchParams, page?: PageOpts): void {
  if (page?.page != null) params.set('page', String(page.page))
  if (page?.pageSize != null) params.set('pageSize', String(page.pageSize))
}
```

```ts
export async function getProducts(q: ProductQuery, page?: PageOpts): Promise<Paginated<Product>> {
  const params = new URLSearchParams()
  if (q.search) params.set('search', q.search)
  if (q.category) params.set('category', q.category)
  if (q.tag) params.set('tag', q.tag)
  if (q.sort) params.set('sort', q.sort)
  appendPage(params, page)
  const qs = params.toString()
  const url = qs ? `/api/products?${qs}` : '/api/products'
  const res = await fetch(url)
  return parse<Paginated<Product>>(res, url)
}

export async function getPlans(page?: PageOpts): Promise<Paginated<Plan>> {
  const params = new URLSearchParams()
  appendPage(params, page)
  const qs = params.toString()
  const url = qs ? `/api/plans?${qs}` : '/api/plans'
  return parse<Paginated<Plan>>(await fetch(url), url)
}

export async function getApplications(token: string, page?: PageOpts): Promise<Paginated<Application>> {
  const params = new URLSearchParams()
  appendPage(params, page)
  const qs = params.toString()
  const url = qs ? `/api/applications?${qs}` : '/api/applications'
  return parse<Paginated<Application>>(await fetch(url, { headers: authHeaders(token) }), url)
}

export async function adminGetProducts(token: string, page?: PageOpts): Promise<Paginated<AdminProduct>> {
  const params = new URLSearchParams()
  appendPage(params, page)
  const qs = params.toString()
  const url = qs ? `/api/admin/products?${qs}` : '/api/admin/products'
  return parse<Paginated<AdminProduct>>(await fetch(url, { headers: authHeaders(token) }), url)
}

export async function adminGetPlans(token: string, page?: PageOpts): Promise<Paginated<Plan>> {
  const params = new URLSearchParams()
  appendPage(params, page)
  const qs = params.toString()
  const url = qs ? `/api/admin/plans?${qs}` : '/api/admin/plans'
  return parse<Paginated<Plan>>(await fetch(url, { headers: authHeaders(token) }), url)
}

export async function adminGetSubscriptions(token: string, status?: string, page?: PageOpts): Promise<Paginated<AdminSubscription>> {
  const params = new URLSearchParams()
  if (status) params.set('status', status)
  appendPage(params, page)
  const qs = params.toString()
  const url = qs ? `/api/admin/subscriptions?${qs}` : '/api/admin/subscriptions'
  return parse<Paginated<AdminSubscription>>(await fetch(url, { headers: authHeaders(token) }), url)
}
```

Leave `getApplicationDetail`, `createApplication`, mutations, and auth functions unchanged.

- [ ] **Step 5: Run client tests**

Run: `cd web && pnpm test src/api/client.test.ts`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
cd web && git add src/api/types.ts src/api/client.ts src/api/client.test.ts
git commit -m "feat(web): paginated client returning Paginated<T> envelope"
```

---

### Task 9: Shared `Pagination` component

**Files:**
- Create: `web/src/components/Pagination.tsx`
- Test: `web/src/components/Pagination.test.tsx`

**Interfaces:**
- Produces: `Pagination` component, props `{ page: number; pageSize: number; total: number; onPage: (page: number) => void }`. Renders nothing when `total <= pageSize`. Shows "Page X · N total" with Préc./Suiv. buttons; prev disabled on page 1, next disabled when `page * pageSize >= total`.

- [ ] **Step 1: Write the failing test**

```tsx
import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { Pagination } from './Pagination'

describe('Pagination', () => {
  it('renders nothing when everything fits on one page', () => {
    const { container } = render(<Pagination page={1} pageSize={20} total={20} onPage={() => {}} />)
    expect(container.firstChild).toBeNull()
  })

  it('shows page info and total', () => {
    render(<Pagination page={2} pageSize={20} total={45} onPage={() => {}} />)
    expect(screen.getByText(/Page 2/)).toBeInTheDocument()
    expect(screen.getByText(/45/)).toBeInTheDocument()
  })

  it('disables Préc. on first page and advances on Suiv.', () => {
    const onPage = vi.fn()
    render(<Pagination page={1} pageSize={20} total={45} onPage={onPage} />)
    expect(screen.getByRole('button', { name: /Préc/ })).toBeDisabled()
    fireEvent.click(screen.getByRole('button', { name: /Suiv/ }))
    expect(onPage).toHaveBeenCalledWith(2)
  })

  it('disables Suiv. on the last page', () => {
    render(<Pagination page={3} pageSize={20} total={45} onPage={() => {}} />)
    expect(screen.getByRole('button', { name: /Suiv/ })).toBeDisabled()
  })
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && pnpm test src/components/Pagination.test.tsx`
Expected: FAIL (module not found).

- [ ] **Step 3: Implement the component**

```tsx
interface PaginationProps {
  page: number
  pageSize: number
  total: number
  onPage: (page: number) => void
}

// Minimal prev/next pager. Renders nothing when the full set fits on one page.
export function Pagination({ page, pageSize, total, onPage }: PaginationProps) {
  if (total <= pageSize) return null
  const lastPage = Math.max(1, Math.ceil(total / pageSize))
  const btn: React.CSSProperties = {
    fontSize: 13, padding: '5px 12px', borderRadius: 8,
    border: '1px solid var(--border-2)', background: 'var(--surface)',
    color: 'var(--fg)', cursor: 'pointer',
  }
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 12, justifyContent: 'center', margin: '20px 0' }}>
      <button style={btn} onClick={() => onPage(page - 1)} disabled={page <= 1}>Préc.</button>
      <span style={{ fontSize: 13, color: 'var(--muted)' }}>Page {page} · {total} au total</span>
      <button style={btn} onClick={() => onPage(page + 1)} disabled={page >= lastPage}>Suiv.</button>
    </div>
  )
}
```

If `React` types are not auto-imported in this project, replace `React.CSSProperties` with `import type { CSSProperties } from 'react'` and use `CSSProperties`.

- [ ] **Step 4: Run to verify it passes**

Run: `cd web && pnpm test src/components/Pagination.test.tsx`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd web && git add src/components/Pagination.tsx src/components/Pagination.test.tsx
git commit -m "feat(web): shared Pagination prev/next component"
```

---

### Task 10: Wire CatalogPage + ApplicationsIndex

**Files:**
- Modify: `web/src/pages/CatalogPage.tsx`
- Modify: `web/src/pages/application/ApplicationsIndex.tsx`
- Test: `web/src/pages/CatalogPage.test.tsx`

**Interfaces:**
- Consumes: `getProducts(q, {page,pageSize})`, `getApplications(token, opts)`, `Pagination`.

**Note:** `ApplicationsIndex` never renders a list — it redirects to the first app or shows the create form — so it only needs to read `.items` from the envelope; **no Pagination control there**. `CatalogPage`'s mount-only "all products" fetch (used for category/tag counts) requests a large page so the rail counts stay meaningful.

- [ ] **Step 1: Update CatalogPage test for the envelope**

In `web/src/pages/CatalogPage.test.tsx`, change every mocked `getProducts` resolution from an array to an envelope. Find the mock (e.g. `vi.mock('../api/client', ...)` or a spy) and make `getProducts` resolve `{ items: PRODUCTS, total: PRODUCTS.length, page: 1, pageSize: 20 }`. Keep existing assertions about rendered cards. If the test imports a `Product[]` fixture, reuse it as `items`.

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && pnpm test src/pages/CatalogPage.test.tsx`
Expected: FAIL (page reads `.items`, mock returns array → cards render 0 / runtime error).

- [ ] **Step 3: Update CatalogPage**

Add imports and page state, and read `.items`:

```tsx
import { Pagination } from '../components/Pagination'
```

Add state near the other `useState` calls:

```tsx
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const pageSize = 20
```

Mount-only "all products" fetch — request a large page so category/tag counts cover the catalog:

```tsx
  // Mount-only: fetch unfiltered catalog (large page) for stable category counts
  useEffect(() => {
    getProducts({}, { pageSize: 100 }).then(r => setAllProducts(r.items)).catch(() => { /* silent */ })
  }, [])
```

Reset to page 1 when filters/sort change, and include `page` in the filtered fetch:

```tsx
  // Filters/sort change → go back to page 1.
  useEffect(() => { setPage(1) }, [search, category, tag, sort])

  useEffect(() => {
    let alive = true
    setLoading(true)
    setError('')
    getProducts({ search: search || undefined, category: category || undefined, tag: tag || undefined, sort }, { page, pageSize })
      .then(r => { if (alive) { setProducts(r.items); setTotal(r.total) } })
      .catch(() => { if (alive) { setProducts([]); setTotal(0); setError('Impossible de charger le catalogue. Vérifiez que le service est démarré.') } })
      .finally(() => { if (alive) setLoading(false) })
    return () => { alive = false }
  }, [search, category, tag, sort, page])
```

Change the result-count line to use `total`, and render the pager under the grid:

```tsx
              <p className="rescount"><b>{total}</b> API{total > 1 ? 's' : ''}</p>
```

```tsx
          {!loading && !error && products.length === 0 && <p className="rescount">Aucune API ne correspond.</p>}
          <Pagination page={page} pageSize={pageSize} total={total} onPage={setPage} />
        </main>
```

- [ ] **Step 4: Update ApplicationsIndex (consume envelope, no control)**

In `web/src/pages/application/ApplicationsIndex.tsx`, the fetch must read `.items`:

```tsx
  useEffect(() => {
    if (!token) return
    getApplications(token).then(r => setApps(r.items)).catch(() => setErr('Impossible de charger les applications.'))
  }, [token])
```

Everything else on this page stays as-is (it redirects when `apps` is non-empty).

- [ ] **Step 5: Run tests**

Run: `cd web && pnpm test src/pages/CatalogPage.test.tsx`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
cd web && git add src/pages/CatalogPage.tsx src/pages/application/ApplicationsIndex.tsx src/pages/CatalogPage.test.tsx
git commit -m "feat(web): paginate catalog grid; consume app list envelope"
```

---

### Task 11: Wire admin ProductsPage, PlansPage, ApprovalsPage

**Files:**
- Modify: `web/src/pages/admin/ProductsPage.tsx`, `web/src/pages/admin/PlansPage.tsx`, `web/src/pages/admin/ApprovalsPage.tsx`
- Test: `web/src/pages/admin/ProductsPage.test.tsx`, `web/src/pages/admin/PlansPage.test.tsx`, `web/src/pages/admin/ApprovalsPage.test.tsx`

**Interfaces:**
- Consumes: `adminGetProducts(token, opts)`, `adminGetPlans(token, opts)`, `adminGetSubscriptions(token, status, opts)`, `Pagination`.

**Note:** each admin page uses a `reqSeq` monotonic guard and a `reload` callback. Add `page`/`total` state, include `page` in `reload`'s deps, read `.items`/`.total`, and render `<Pagination>` after the rows. The existing client-side `filter`/`shown` in ProductsPage now filters within the current page only — acceptable for this minimal pass.

- [ ] **Step 1: Update the three admin page tests for the envelope**

In each test file, change the mocked list function to resolve an envelope. For example in `ProductsPage.test.tsx`, make `adminGetProducts` resolve `{ items: PRODUCTS, total: PRODUCTS.length, page: 1, pageSize: 20 }`; in `PlansPage.test.tsx` do the same for `adminGetPlans`; in `ApprovalsPage.test.tsx` for `adminGetSubscriptions`. Keep existing row-render assertions.

- [ ] **Step 2: Run to verify they fail**

Run: `cd web && pnpm test src/pages/admin/ProductsPage.test.tsx src/pages/admin/PlansPage.test.tsx src/pages/admin/ApprovalsPage.test.tsx`
Expected: FAIL.

- [ ] **Step 3: Update ProductsPage**

Add `import { Pagination } from '../../components/Pagination'`, add `page`/`total` state, update `reload`, and render the pager.

```tsx
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const pageSize = 20
```

```tsx
  const reload = useCallback(() => {
    if (!token) return
    const seq = ++reqSeq.current
    adminGetProducts(token, { page, pageSize })
      .then(r => { if (seq === reqSeq.current) { setProducts(r.items); setTotal(r.total) } })
      .catch(() => { if (seq === reqSeq.current) setErr('Impossible de charger les produits.') })
  }, [token, page])
  useEffect(reload, [reload])
```

Render the pager right after the `</div>` that closes `<div className="rows">`:

```tsx
      </div>
      <Pagination page={page} pageSize={pageSize} total={total} onPage={setPage} />
```

(Keep `counts={{ products: products.length }}` as-is, or change to `total` if you prefer a global count; `products.length` reflects the current page.)

- [ ] **Step 4: Update PlansPage**

Same shape with `adminGetPlans`:

```tsx
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const pageSize = 20
```

```tsx
  const reload = useCallback(() => {
    if (!token) return
    const seq = ++reqSeq.current
    adminGetPlans(token, { page, pageSize })
      .then(r => { if (seq === reqSeq.current) { setPlans(r.items); setTotal(r.total) } })
      .catch(() => { if (seq === reqSeq.current) setErr('Impossible de charger les plans.') })
  }, [token, page])
  useEffect(reload, [reload])
```

Add `import { Pagination } from '../../components/Pagination'` and render after the rows `</div>`:

```tsx
      </div>
      <Pagination page={page} pageSize={pageSize} total={total} onPage={setPage} />
```

- [ ] **Step 5: Update ApprovalsPage**

Same shape with `adminGetSubscriptions(token, 'pending', {page,pageSize})`:

```tsx
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const pageSize = 20
```

```tsx
  const reload = useCallback(() => {
    if (!token) return
    const seq = ++reqSeq.current
    adminGetSubscriptions(token, 'pending', { page, pageSize })
      .then(r => { if (seq === reqSeq.current) { setSubs(r.items); setTotal(r.total); setLoaded(true) } })
      .catch(() => { if (seq === reqSeq.current) setErr('Impossible de charger les abonnements.') })
  }, [token, page])
  useEffect(reload, [reload])
```

Add `import { Pagination } from '../../components/Pagination'` and render the pager just before the closing `</AdminShell>`:

```tsx
      <Pagination page={page} pageSize={pageSize} total={total} onPage={setPage} />
      <Toast msg={toast?.msg ?? null} kind={toast?.kind} />
    </AdminShell>
```

- [ ] **Step 6: Run the full frontend suite**

Run: `cd web && pnpm test`
Expected: PASS (all suites).

- [ ] **Step 7: Lint + typecheck**

Run: `cd web && pnpm lint && pnpm build`
Expected: no type errors.

- [ ] **Step 8: Commit**

```bash
cd web && git add src/pages/admin/ProductsPage.tsx src/pages/admin/PlansPage.tsx src/pages/admin/ApprovalsPage.tsx src/pages/admin/ProductsPage.test.tsx src/pages/admin/PlansPage.test.tsx src/pages/admin/ApprovalsPage.test.tsx
git commit -m "feat(web): pagination controls on admin products/plans/approvals"
```

---

## Final verification

- [ ] `go test ./...` passes.
- [ ] `cd web && pnpm test` passes; `pnpm lint && pnpm build` clean.
- [ ] Manual smoke (optional, needs DB+gateway up): `GET /api/products?page=1&pageSize=2` returns `{items,total,page,pageSize}` with ≤2 items; `?pageSize=999` caps at 100; `?page=999` returns empty `items` with the real `total`.
