# Offset Pagination for List Endpoints — Design

Date: 2026-06-22
Status: Approved

## Goal

Add offset/page-based pagination to all six collection-returning API endpoints,
returning a consistent envelope object, and wire the React frontend (client +
minimal prev/next UI) to consume it.

## Affected endpoints

All six list endpoints currently return bare JSON arrays:

| Endpoint | Handler | Repo |
| --- | --- | --- |
| `GET /api/products` | `internal/catalog/handler.go` | `internal/catalog/repo.go` |
| `GET /api/plans` | `internal/plans/handler.go` | `internal/plans/repo.go` |
| `GET /api/applications` | `internal/applications/handler.go` | `internal/applications/repo.go` |
| `GET /api/admin/products` | `internal/admin/handler.go` | `internal/admin/repo.go` |
| `GET /api/admin/plans` | `internal/admin/plan_handler.go` | `internal/admin/plan_repo.go` |
| `GET /api/admin/subscriptions` | `internal/subscriptions/admin_handler.go` | `internal/subscriptions/repo.go` (via `Service`) |

Single-resource endpoints (GET-by-id/slug, create/update/delete, subscribe,
usage) and the hardcoded 20-entry activity feed in
`GET /api/applications/{id}` are **out of scope**.

## Decisions

- **Strategy:** offset/page-based (`?page=&pageSize=`). Composes with existing
  filters/sort; lets the UI show a total. Cursor pagination rejected as overkill
  for these table sizes.
- **Response shape:** envelope object (breaking change; all in-repo consumers
  updated in the same change).
- **Defaults:** `page` default 1 (floor 1); `pageSize` default 20, floor 1,
  cap 100. Garbage/non-numeric values fall back to defaults.
- **Out-of-range page:** returns empty `items` with the true `total` so the UI
  can disable "Next".
- **Frontend:** wire client + minimal prev/next controls with "Page X · N total".
  No numbered page buttons.

## Backend design

### New package `internal/paging`

Neutral package — depends only on `net/url` and generics, **not** `net/http`, so
repos can import it without pulling in the web layer.

```go
package paging

type Params struct {
    Page int // >= 1
    Size int // 1..100
}

func (p Params) Limit() int  { return p.Size }
func (p Params) Offset() int { return (p.Page - 1) * p.Size }

// Parse reads ?page and ?pageSize, clamping to defaults/bounds.
func Parse(v url.Values) Params

// Page is the JSON envelope. Items is always a non-nil slice.
type Page[T any] struct {
    Items    []T `json:"items"`
    Total    int `json:"total"`
    Page     int `json:"page"`
    PageSize int `json:"pageSize"`
}

func New[T any](items []T, total int, p Params) Page[T]
```

Constants: `defaultSize = 20`, `maxSize = 100`, `defaultPage = 1`.

`New` normalizes a nil `items` slice to `[]T{}` so the JSON is always `"items": []`.

### Repo layer

Each `List` method:
1. Gains a `paging.Params` argument.
2. Returns `(items []T, total int, err error)`.
3. Computes `total` via `COUNT(*)` using the **same** WHERE filters, then runs
   the page query with `LIMIT $n OFFSET $m` appended after `ORDER BY`.

For `catalog`, the count and page queries share the filter-building logic so the
total always matches the active `search`/`category`/`tag` filters.

The `subscriptions` admin path threads `paging.Params` through
`AdminService.AdminSubscriptions` and the underlying repo query, returning the
total alongside the rows.

### Handler layer

Each list handler:
```go
p := paging.Parse(r.URL.Query())
items, total, err := h.repo.List(r.Context(), q, p) // err handling unchanged
httpx.JSON(w, http.StatusOK, paging.New(items, total, p))
```

Existing error handling and the `nil`→`[]` guard are preserved (the latter now
lives in `paging.New`).

## Frontend design

### Types (`web/src/api/types.ts`)

```ts
export interface Paginated<T> {
  items: T[]
  total: number
  page: number
  pageSize: number
}
```

### Client (`web/src/api/client.ts`)

List functions gain an optional `{ page?, pageSize? }` arg, append `page` /
`pageSize` query params, and return `Paginated<T>` instead of `T[]`:
`getProducts`, `getPlans`, `getApplications`, `adminGetProducts`,
`adminGetPlans`, `adminGetSubscriptions`.

### Pages

Consuming pages read `.items` and render a minimal pagination control
(prev/next buttons + "Page X · N total"; prev disabled on page 1, next disabled
when `page * pageSize >= total`):
`CatalogPage`, `application/ApplicationsIndex`, `admin/ProductsPage`,
`admin/PlansPage`, `admin/ApprovalsPage`. Existing filter/sort UI is retained.

## Testing (TDD)

- **`paging` package:** `Parse` defaults, clamping (floor 1, cap 100), garbage
  input; `New` nil-slice normalization and field mapping.
- **Handlers (Go):** assert envelope shape, correct `total`, and that
  `page`/`pageSize` round-trip; update existing fakes/handler tests to the new
  `List` signatures.
- **Repos:** `LIMIT/OFFSET` and `COUNT` behavior where repo-level tests exist.
- **Frontend:** client tests assert query params sent and envelope parsed; page
  tests cover prev/next enable/disable and "N total" rendering.

## Out of scope

- Cursor pagination.
- Numbered page buttons / jump-to-page.
- Pagination for single-resource endpoints or the activity feed.
- Persisting page state in the URL (can be a follow-up).
