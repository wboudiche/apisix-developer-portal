# Real Ratings & Reviews Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the static seeded `rating` with real developer ratings (1–5 stars + optional comment, one per user, approved-subscribers only) shown on the catalog and a Reviews section on the product detail page.

**Architecture:** A new `product_ratings` table; the existing `api_products.rating` becomes the real average plus a new `rating_count`, recomputed in a transaction on each write. A new `internal/ratings` package serves `/api/ratings/{slug}` — public GET (summary + reviews + the caller's own rating + canRate) and an authed PUT (approved-subscriber-gated upsert). The frontend shows avg/count on cards + detail and a Reviews section with a rating form.

**Tech Stack:** Go 1.25 (chi, pgx), React 19 + TS (Vite, vitest).

## Global Constraints

- Module `apisix-portal`. New package `internal/ratings`; catalog in `internal/catalog`.
- One rating per (product, user), upserted (editable). Stars 1–5; comment optional, ≤500 chars.
- Only approved subscribers may PUT (reuse `subscriptions.Repo.ApprovedAppsForProduct(userID, productID)` → non-empty); else 403.
- Displayed average/count reflect ONLY real ratings; the migration resets seeded `rating` to 0; products with none show "Pas encore noté".
- `api_products.rating` (avg) + `rating_count` are a denormalized cache recomputed in the same tx as each write; the catalog list/sort hot path keeps reading `rating`.
- `/api/ratings/{slug}`: GET public (optional token → `mine`/`canRate`), PUT authed (per-route `auth.RequireAuth`). Mounted at `/api/ratings/` with NO outer auth.
- pnpm for the frontend; French copy; reuse Atlas tokens + `formatRelative`.

---

## Task 1: Migration + catalog reads rating_count

**Files:**
- Create: `internal/db/migrations/0010_product_ratings.sql`
- Modify: `internal/catalog/product.go` (`RatingCount`), `internal/catalog/repo.go` (`baseSelect`, `scanProducts`)
- Test: `internal/catalog/repo_test.go`

**Interfaces:**
- Produces: `catalog.Product.RatingCount int` (json `ratingCount`); catalog reads expose it.

- [ ] **Step 1: Write the migration**

Create `internal/db/migrations/0010_product_ratings.sql`:
```sql
-- Real per-user ratings; the api_products.rating/rating_count are a denormalized
-- cache recomputed on each write. Seeded ratings reset to real-only.
CREATE TABLE IF NOT EXISTS product_ratings (
    id             BIGSERIAL PRIMARY KEY,
    api_product_id BIGINT NOT NULL REFERENCES api_products(id) ON DELETE CASCADE,
    user_id        BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    stars          SMALLINT NOT NULL CHECK (stars BETWEEN 1 AND 5),
    comment        TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (api_product_id, user_id)
);
CREATE INDEX IF NOT EXISTS product_ratings_product_idx ON product_ratings (api_product_id, created_at DESC);

ALTER TABLE api_products ADD COLUMN IF NOT EXISTS rating_count INT NOT NULL DEFAULT 0;
-- Real-only: drop the seeded static ratings.
UPDATE api_products SET rating = 0, rating_count = 0;
```

- [ ] **Step 2: Write the failing test**

In `internal/catalog/repo_test.go` add (use the file's `testPool(t) (ctx, *Repo)` helper; clean up):
```go
func TestListExposesRatingCount(t *testing.T) {
	ctx, repo := testPool(t)
	// A freshly migrated DB has rating_count defaulted to 0 on seeded products.
	products, _, err := repo.List(ctx, Query{}, paging.Params{Page: 1, Size: 50})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(products) == 0 {
		t.Skip("no published products seeded")
	}
	for _, p := range products {
		if p.RatingCount < 0 {
			t.Fatalf("RatingCount negative for %s", p.Slug)
		}
	}
}
```
(This compiles only once `RatingCount` exists and `scanProducts` reads it.)

- [ ] **Step 3: Run to verify it fails**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/catalog/ -run TestListExposesRatingCount -v`
Expected: FAIL — `RatingCount` undefined.

- [ ] **Step 4: Add the field + read it**

In `internal/catalog/product.go`, add to `Product` (after `Rating`):
```go
	RatingCount int `json:"ratingCount"`
```
In `internal/catalog/repo.go`, change `baseSelect` to include `rating_count`:
```go
const baseSelect = `SELECT id, name, slug, category, version, context_path, description, tags, icon, rating, rating_count
	FROM api_products WHERE published = true`
```
In `scanProducts`, add the scan target after `&p.Rating`:
```go
			&p.Rating,
			&p.RatingCount,
```

- [ ] **Step 5: Run to verify it passes**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/catalog/ && go vet ./internal/catalog/`
Expected: PASS. NOTE: if an existing catalog test asserts a specific non-zero `rating` on a seeded product, update it to expect `0` (the migration reset them) — the real average is now built from `product_ratings`.

- [ ] **Step 6: Commit**

```bash
git add internal/db/migrations/0010_product_ratings.sql internal/catalog/product.go internal/catalog/repo.go internal/catalog/repo_test.go
git commit -m "feat(catalog): product_ratings migration + rating_count on products"
```

---

## Task 2: Ratings repo (upsert + recompute, list, mine, summary)

**Files:**
- Create: `internal/ratings/repo.go`
- Test: `internal/ratings/repo_test.go`

**Interfaces:**
- Produces (on `*Repo` with `NewRepo(pool *pgxpool.Pool) *Repo`):
  - `type Review struct { Stars int; Comment string; Author string; CreatedAt time.Time }`
  - `type Summary struct { Average float64; Count int }`
  - `Upsert(ctx, productID, userID int64, stars int, comment string) error` — upsert + recompute the product's cached avg/count, in one tx.
  - `List(ctx, productID int64) ([]Review, error)` — newest first, author = `users.name` (fallback "Développeur").
  - `Mine(ctx, productID, userID int64) (*Review, error)` — the user's row or nil.
  - `SummaryFor(ctx, productID int64) (Summary, error)` — from the cached columns.

- [ ] **Step 1: Write the failing DB tests**

Create `internal/ratings/repo_test.go` (mirror `internal/applications/repo_test.go`'s live-DB setup: skip if no `DATABASE_URL`, seed via the pool, `t.Cleanup`):
```go
package ratings

import (
	"context"
	"os"
	"testing"
	"time"

	"apisix-portal/internal/db"
)

func testRepo(t *testing.T) (context.Context, *Repo, int64, int64) {
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
	suf := time.Now().Format("150405.000000000")
	var uid, pid int64
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,name) VALUES($1,'x',$2) RETURNING id`,
		"rater+"+suf+"@e.com", "Rater "+suf).Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO api_products(name,slug,category,context_path,published) VALUES($1,$2,'C','/r',true) RETURNING id`,
		"RateProd "+suf, "rateprod-"+suf).Scan(&pid); err != nil {
		t.Fatalf("seed product: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM api_products WHERE id=$1`, pid)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, uid)
	})
	return ctx, NewRepo(pool), uid, pid
}

func TestUpsertRecomputesAndIsOnePerUser(t *testing.T) {
	ctx, repo, uid, pid := testRepo(t)
	if err := repo.Upsert(ctx, pid, uid, 4, "bien"); err != nil {
		t.Fatalf("upsert1: %v", err)
	}
	s, _ := repo.SummaryFor(ctx, pid)
	if s.Count != 1 || s.Average != 4 {
		t.Fatalf("after 1: %+v", s)
	}
	// Same user re-rates: updates, does not insert a second row.
	if err := repo.Upsert(ctx, pid, uid, 2, "finalement bof"); err != nil {
		t.Fatalf("upsert2: %v", err)
	}
	s, _ = repo.SummaryFor(ctx, pid)
	if s.Count != 1 || s.Average != 2 {
		t.Fatalf("after re-rate: %+v", s)
	}
	mine, err := repo.Mine(ctx, pid, uid)
	if err != nil || mine == nil || mine.Stars != 2 || mine.Comment != "finalement bof" {
		t.Fatalf("mine: %+v %v", mine, err)
	}
	list, err := repo.List(ctx, pid)
	if err != nil || len(list) != 1 || list[0].Author == "" {
		t.Fatalf("list: %+v %v", list, err)
	}
}

func TestMineNilWhenNoRating(t *testing.T) {
	ctx, repo, uid, pid := testRepo(t)
	mine, err := repo.Mine(ctx, pid, uid)
	if err != nil || mine != nil {
		t.Fatalf("mine = %+v, %v (want nil)", mine, err)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/ratings/ -v`
Expected: FAIL — package/`Repo` not defined.

- [ ] **Step 3: Implement `internal/ratings/repo.go`**

```go
// Package ratings stores per-user API ratings and keeps the product's cached
// average + count in sync.
package ratings

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Review struct {
	Stars     int       `json:"stars"`
	Comment   string    `json:"comment"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"createdAt"`
}

type Summary struct {
	Average float64 `json:"average"`
	Count   int     `json:"count"`
}

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

// Upsert writes (or updates) the user's rating and recomputes the product's
// cached average + count, atomically.
func (r *Repo) Upsert(ctx context.Context, productID, userID int64, stars int, comment string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`INSERT INTO product_ratings(api_product_id, user_id, stars, comment)
		 VALUES($1,$2,$3,$4)
		 ON CONFLICT (api_product_id, user_id)
		 DO UPDATE SET stars=EXCLUDED.stars, comment=EXCLUDED.comment, updated_at=now()`,
		productID, userID, stars, comment); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE api_products SET
		   rating = COALESCE((SELECT AVG(stars) FROM product_ratings WHERE api_product_id=$1), 0),
		   rating_count = (SELECT count(*) FROM product_ratings WHERE api_product_id=$1)
		 WHERE id=$1`, productID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repo) List(ctx context.Context, productID int64) ([]Review, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT pr.stars, pr.comment, COALESCE(NULLIF(u.name,''),'Développeur'), pr.created_at
		   FROM product_ratings pr JOIN users u ON u.id = pr.user_id
		 WHERE pr.api_product_id=$1 ORDER BY pr.created_at DESC`, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Review
	for rows.Next() {
		var rv Review
		if err := rows.Scan(&rv.Stars, &rv.Comment, &rv.Author, &rv.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, rv)
	}
	return out, rows.Err()
}

func (r *Repo) Mine(ctx context.Context, productID, userID int64) (*Review, error) {
	var rv Review
	err := r.pool.QueryRow(ctx,
		`SELECT pr.stars, pr.comment, COALESCE(NULLIF(u.name,''),'Développeur'), pr.created_at
		   FROM product_ratings pr JOIN users u ON u.id = pr.user_id
		 WHERE pr.api_product_id=$1 AND pr.user_id=$2`, productID, userID).
		Scan(&rv.Stars, &rv.Comment, &rv.Author, &rv.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rv, nil
}

func (r *Repo) SummaryFor(ctx context.Context, productID int64) (Summary, error) {
	var s Summary
	err := r.pool.QueryRow(ctx,
		`SELECT rating, rating_count FROM api_products WHERE id=$1`, productID).Scan(&s.Average, &s.Count)
	return s, err
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/ratings/ && go vet ./internal/ratings/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ratings/repo.go internal/ratings/repo_test.go
git commit -m "feat(ratings): product_ratings repo (upsert+recompute, list, mine, summary)"
```

---

## Task 3: Ratings handler (GET public + PUT authed, approved-subscriber gate)

**Files:**
- Create: `internal/ratings/handler.go`
- Test: `internal/ratings/handler_test.go`

**Interfaces:**
- Consumes: `Repo` (Task 2); `auth.RequireAuth`, `auth.UserID`, `*auth.Tokenizer`; `httpx`.
- Produces:
  - `type Products interface { ProductBySlug(ctx, slug string) (int64, error) }` (published-only; `ratings.ErrNotFound` when missing).
  - `type Subscribers interface { IsApprovedSubscriber(ctx, userID, productID int64) (bool, error) }`.
  - `type Store interface { Upsert(...); List(...); Mine(...); SummaryFor(...) }` (satisfied by `*Repo`).
  - `type RatingsView struct { Average float64; Count int; Items []Review; Mine *Review; CanRate bool }`.
  - `func NewHandler(store Store, products Products, subs Subscribers, tok *auth.Tokenizer) *Handler` with `ServeHTTP`; routes `GET /api/ratings/{slug}` (public) and `PUT /api/ratings/{slug}` (auth-wrapped).
  - `var ErrNotFound = errors.New("ratings: product not found")`.

- [ ] **Step 1: Write the failing handler tests**

Create `internal/ratings/handler_test.go`:
```go
package ratings

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
	mine    *Review
	items   []Review
	summary Summary
	gotPut  *putArgs
}
type putArgs struct{ productID, userID int64; stars int; comment string }
func (f *fakeStore) Upsert(_ context.Context, p, u int64, s int, c string) error {
	f.gotPut = &putArgs{p, u, s, c}
	f.summary = Summary{Average: float64(s), Count: 1}
	f.mine = &Review{Stars: s, Comment: c, Author: "Me"}
	return nil
}
func (f *fakeStore) List(context.Context, int64) ([]Review, error)        { return f.items, nil }
func (f *fakeStore) Mine(context.Context, int64, int64) (*Review, error)  { return f.mine, nil }
func (f *fakeStore) SummaryFor(context.Context, int64) (Summary, error)   { return f.summary, nil }

type fakeProducts struct{ id int64; err error }
func (f fakeProducts) ProductBySlug(context.Context, string) (int64, error) { return f.id, f.err }

type fakeSubs struct{ approved bool }
func (f fakeSubs) IsApprovedSubscriber(context.Context, int64, int64) (bool, error) { return f.approved, nil }

// a tokenizer that always parses to a fixed user — use the real auth.Tokenizer
// helper if available; otherwise build one with a test secret.
func testTok(t *testing.T) (*auth.Tokenizer, string) {
	tok := auth.NewTokenizer("test-secret", time.Hour) // adjust to the real constructor signature
	s, err := tok.Issue(7, "developer")                // adjust to the real Issue/Sign signature
	if err != nil { t.Fatalf("issue: %v", err) }
	return tok, s
}

func TestRatingsGetPublic(t *testing.T) {
	tok, _ := testTok(t)
	h := NewHandler(&fakeStore{summary: Summary{Average: 4, Count: 2}, items: []Review{{Stars: 4, Author: "A"}}},
		fakeProducts{id: 9}, fakeSubs{}, tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/ratings/orders", nil))
	if rec.Code != http.StatusOK { t.Fatalf("status=%d", rec.Code) }
	var v RatingsView
	_ = json.Unmarshal(rec.Body.Bytes(), &v)
	if v.Average != 4 || v.Count != 2 || len(v.Items) != 1 || v.CanRate { t.Fatalf("view=%+v", v) }
}

func TestRatingsPutApprovedSubscriber(t *testing.T) {
	tok, jwt := testTok(t)
	store := &fakeStore{}
	h := NewHandler(store, fakeProducts{id: 9}, fakeSubs{approved: true}, tok)
	req := httptest.NewRequest(http.MethodPut, "/api/ratings/orders", strings.NewReader(`{"stars":5,"comment":"top"}`))
	req.Header.Set("Authorization", "Bearer "+jwt)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK { t.Fatalf("status=%d body=%s", rec.Code, rec.Body) }
	if store.gotPut == nil || store.gotPut.stars != 5 || store.gotPut.userID != 7 { t.Fatalf("put=%+v", store.gotPut) }
}

func TestRatingsPutNonSubscriber403(t *testing.T) {
	tok, jwt := testTok(t)
	h := NewHandler(&fakeStore{}, fakeProducts{id: 9}, fakeSubs{approved: false}, tok)
	req := httptest.NewRequest(http.MethodPut, "/api/ratings/orders", strings.NewReader(`{"stars":5}`))
	req.Header.Set("Authorization", "Bearer "+jwt)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden { t.Fatalf("status=%d", rec.Code) }
}

func TestRatingsPutBadStars400(t *testing.T) {
	tok, jwt := testTok(t)
	h := NewHandler(&fakeStore{}, fakeProducts{id: 9}, fakeSubs{approved: true}, tok)
	req := httptest.NewRequest(http.MethodPut, "/api/ratings/orders", strings.NewReader(`{"stars":9}`))
	req.Header.Set("Authorization", "Bearer "+jwt)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest { t.Fatalf("status=%d", rec.Code) }
}

func TestRatingsPutAnon401(t *testing.T) {
	tok, _ := testTok(t)
	h := NewHandler(&fakeStore{}, fakeProducts{id: 9}, fakeSubs{approved: true}, tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/ratings/orders", strings.NewReader(`{"stars":5}`)))
	if rec.Code != http.StatusUnauthorized { t.Fatalf("status=%d", rec.Code) }
}
```
NOTE: `testTok` MUST match the real `auth` API. Inspect `internal/auth/token.go` for the actual constructor (e.g. `NewTokenizer`) and issue/sign method + `Claims` field names, and adjust `testTok` accordingly. Add the `time` import. The `auth.RequireAuth` middleware reads the `Authorization: Bearer <jwt>` header and puts the user id in context via `auth.UserID`.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/ratings/ -run TestRatings -v`
Expected: FAIL — handler/types not defined.

- [ ] **Step 3: Implement `internal/ratings/handler.go`**

```go
package ratings

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"apisix-portal/internal/auth"
	"apisix-portal/internal/httpx"
)

var ErrNotFound = errors.New("ratings: product not found")

type Products interface {
	ProductBySlug(ctx context.Context, slug string) (int64, error)
}
type Subscribers interface {
	IsApprovedSubscriber(ctx context.Context, userID, productID int64) (bool, error)
}
type Store interface {
	Upsert(ctx context.Context, productID, userID int64, stars int, comment string) error
	List(ctx context.Context, productID int64) ([]Review, error)
	Mine(ctx context.Context, productID, userID int64) (*Review, error)
	SummaryFor(ctx context.Context, productID int64) (Summary, error)
}

type RatingsView struct {
	Average float64  `json:"average"`
	Count   int      `json:"count"`
	Items   []Review `json:"items"`
	Mine    *Review  `json:"mine"`
	CanRate bool     `json:"canRate"`
}

const maxComment = 500

type Handler struct {
	store    Store
	products Products
	subs     Subscribers
	tok      *auth.Tokenizer
	router   chi.Router
}

func NewHandler(store Store, products Products, subs Subscribers, tok *auth.Tokenizer) *Handler {
	h := &Handler{store: store, products: products, subs: subs, tok: tok, router: chi.NewRouter()}
	h.router.Get("/api/ratings/{slug}", h.get)
	h.router.With(auth.RequireAuth(tok)).Put("/api/ratings/{slug}", h.put)
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.router.ServeHTTP(w, r) }

// optionalUserID returns the caller's id when a valid bearer token is present,
// else (0,false). The GET path is public but enriches the view when authed.
func (h *Handler) optionalUserID(r *http.Request) (int64, bool) {
	a := r.Header.Get("Authorization")
	if !strings.HasPrefix(a, "Bearer ") {
		return 0, false
	}
	claims, err := h.tok.Parse(strings.TrimPrefix(a, "Bearer "))
	if err != nil {
		return 0, false
	}
	return claims.UserID, true
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	pid, err := h.products.ProductBySlug(r.Context(), chi.URLParam(r, "slug"))
	if errors.Is(err, ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "product not found")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed")
		return
	}
	view, err := h.buildView(r.Context(), pid, r)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed")
		return
	}
	httpx.JSON(w, http.StatusOK, view)
}

func (h *Handler) put(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserID(r.Context())
	pid, err := h.products.ProductBySlug(r.Context(), chi.URLParam(r, "slug"))
	if errors.Is(err, ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "product not found")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed")
		return
	}
	approved, err := h.subs.IsApprovedSubscriber(r.Context(), userID, pid)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed")
		return
	}
	if !approved {
		httpx.Error(w, http.StatusForbidden, "abonnez-vous pour noter cette API")
		return
	}
	var body struct {
		Stars   int    `json:"stars"`
		Comment string `json:"comment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Stars < 1 || body.Stars > 5 {
		httpx.Error(w, http.StatusBadRequest, "stars must be 1..5")
		return
	}
	comment := strings.TrimSpace(body.Comment)
	if len(comment) > maxComment {
		comment = comment[:maxComment]
	}
	if err := h.store.Upsert(r.Context(), pid, userID, body.Stars, comment); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to save rating")
		return
	}
	view, err := h.buildView(r.Context(), pid, r)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed")
		return
	}
	httpx.JSON(w, http.StatusOK, view)
}

func (h *Handler) buildView(ctx context.Context, pid int64, r *http.Request) (RatingsView, error) {
	sum, err := h.store.SummaryFor(ctx, pid)
	if err != nil {
		return RatingsView{}, err
	}
	items, err := h.store.List(ctx, pid)
	if err != nil {
		return RatingsView{}, err
	}
	if items == nil {
		items = []Review{}
	}
	v := RatingsView{Average: sum.Average, Count: sum.Count, Items: items}
	if uid, ok := h.optionalUserID(r); ok {
		if mine, err := h.store.Mine(ctx, pid, uid); err == nil {
			v.Mine = mine
		}
		if can, err := h.subs.IsApprovedSubscriber(ctx, uid, pid); err == nil {
			v.CanRate = can
		}
	}
	return v, nil
}
```

- [ ] **Step 4: Run to verify it passes + vet**

Run: `go test ./internal/ratings/ && go vet ./internal/ratings/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ratings/handler.go internal/ratings/handler_test.go
git commit -m "feat(ratings): /api/ratings/{slug} GET (public) + PUT (approved-subscriber)"
```

---

## Task 4: Wire ratings into the server

**Files:**
- Modify: `internal/server/server.go` (adapters + mount)
- Create: `internal/server/ratings_adapters.go`

**Interfaces:**
- Consumes: `ratings.NewHandler`, `ratings.ErrNotFound`, `ratings.Products`, `ratings.Subscribers`; the existing `catRepo` (`*catalog.Repo`), `subRepo` (`*subscriptions.Repo`), `tok`, `pool`.
- Produces: `/api/ratings/` mounted (no outer auth; PUT auth is per-route inside the handler).

- [ ] **Step 1: Add adapters**

Create `internal/server/ratings_adapters.go`:
```go
package server

import (
	"context"
	"errors"

	"apisix-portal/internal/catalog"
	"apisix-portal/internal/ratings"
	"apisix-portal/internal/subscriptions"
)

type ratingsProductsAdapter struct{ repo *catalog.Repo }

func (a ratingsProductsAdapter) ProductBySlug(ctx context.Context, slug string) (int64, error) {
	id, _, err := a.repo.ProductBySlug(ctx, slug)
	if errors.Is(err, catalog.ErrNotFound) {
		return 0, ratings.ErrNotFound
	}
	return id, err
}

type ratingsSubsAdapter struct{ subs *subscriptions.Repo }

func (a ratingsSubsAdapter) IsApprovedSubscriber(ctx context.Context, userID, productID int64) (bool, error) {
	apps, err := a.subs.ApprovedAppsForProduct(ctx, userID, productID)
	if err != nil {
		return false, err
	}
	return len(apps) > 0, nil
}
```

- [ ] **Step 2: Wire + mount in server.go**

After the tryit wiring, add:
```go
	ratingsH := ratings.NewHandler(
		ratings.NewRepo(pool),
		ratingsProductsAdapter{repo: catRepo},
		ratingsSubsAdapter{subs: subRepo},
		tok,
	)
```
And in the mux block:
```go
	mux.Handle("/api/ratings/", ratingsH)
```
Add the `apisix-portal/internal/ratings` import.

- [ ] **Step 3: Build + full backend suite**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go build ./... && go test ./internal/... ./cmd/... && go vet ./...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/server/server.go internal/server/ratings_adapters.go
git commit -m "feat(server): mount /api/ratings/ with catalog+subscriptions adapters"
```

---

## Task 5: Frontend client + types + catalog card count

**Files:**
- Modify: `web/src/api/types.ts` (`Product.ratingCount`, `Review`, `RatingsView`)
- Modify: `web/src/api/client.ts` (`getRatings`, `submitRating`)
- Modify: `web/src/components/ApiCard.tsx` (count / "Pas encore noté")
- Test: `web/src/api/client.ratings.test.ts` (new), `web/src/components/ApiCard.test.tsx`

**Interfaces:**
- Produces:
  - `Product.ratingCount: number`; `type Review = { stars: number; comment: string; author: string; createdAt: string }`; `type RatingsView = { average: number; count: number; items: Review[]; mine: Review | null; canRate: boolean }`.
  - `getRatings(slug: string, token?: string): Promise<RatingsView>` → `GET /api/ratings/{slug}` (sends bearer when token given).
  - `submitRating(token: string, slug: string, body: { stars: number; comment: string }): Promise<RatingsView>` → `PUT /api/ratings/{slug}`.

- [ ] **Step 1: Write the failing tests**

Create `web/src/api/client.ratings.test.ts`:
```ts
import { it, expect, vi, afterEach } from 'vitest'
import { getRatings, submitRating } from './client'

afterEach(() => vi.restoreAllMocks())

it('getRatings GETs the endpoint (no auth header when no token)', async () => {
  const f = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
    new Response(JSON.stringify({ average: 4, count: 1, items: [], mine: null, canRate: false }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
  const out = await getRatings('orders')
  expect(out.average).toBe(4)
  expect(f.mock.calls[0][0]).toBe('/api/ratings/orders')
})

it('getRatings sends the bearer token when given', async () => {
  const f = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
    new Response(JSON.stringify({ average: 0, count: 0, items: [], mine: null, canRate: true }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
  await getRatings('orders', 'jwt')
  expect((f.mock.calls[0][1] as RequestInit).headers).toMatchObject({ Authorization: 'Bearer jwt' })
})

it('submitRating PUTs stars+comment with auth', async () => {
  const f = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
    new Response(JSON.stringify({ average: 5, count: 1, items: [], mine: { stars: 5, comment: 'top', author: 'Me', createdAt: '' }, canRate: true }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
  const out = await submitRating('jwt', 'orders', { stars: 5, comment: 'top' })
  expect(out.average).toBe(5)
  const [url, init] = f.mock.calls[0]
  expect(url).toBe('/api/ratings/orders')
  expect((init as RequestInit).method).toBe('PUT')
  expect(JSON.parse((init as RequestInit).body as string)).toEqual({ stars: 5, comment: 'top' })
})
```
Add to `web/src/components/ApiCard.test.tsx` (cards are rendered inside `MemoryRouter` after a prior task — keep that):
```tsx
  it('shows "Pas encore noté" when ratingCount is 0', () => {
    const p = { id: 1, name: 'X', slug: 'x', category: 'C', version: '1', contextPath: '/x', description: '', tags: [], icon: '', rating: 0, ratingCount: 0 }
    render(<MemoryRouter><ApiCard p={p} onSubscribe={() => {}} /></MemoryRouter>)
    expect(screen.getByText(/Pas encore noté/i)).toBeInTheDocument()
  })
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd web && pnpm exec vitest run src/api/client.ratings.test.ts src/components/ApiCard.test.tsx`
Expected: FAIL — fns/field/text missing.

- [ ] **Step 3: Implement types + client**

`web/src/api/types.ts`: add `ratingCount: number` to `Product` (after `rating`); add:
```ts
export interface Review { stars: number; comment: string; author: string; createdAt: string }
export interface RatingsView { average: number; count: number; items: Review[]; mine: Review | null; canRate: boolean }
```
`web/src/api/client.ts` (import the two types; add near `getProduct`):
```ts
export async function getRatings(slug: string, token?: string): Promise<RatingsView> {
  const url = `/api/ratings/${encodeURIComponent(slug)}`
  const headers = token ? authHeaders(token) : undefined
  return parse<RatingsView>(await fetch(url, headers ? { headers } : undefined), url)
}
export async function submitRating(token: string, slug: string, body: { stars: number; comment: string }): Promise<RatingsView> {
  const url = `/api/ratings/${encodeURIComponent(slug)}`
  return parse<RatingsView>(await fetch(url, { method: 'PUT', headers: authHeaders(token), body: JSON.stringify(body) }), url)
}
```

- [ ] **Step 4: ApiCard count / "Pas encore noté"**

In `web/src/components/ApiCard.tsx`, where `<Stars rating={p.rating} />` renders, show the count or the empty state. Replace that area with:
```tsx
          {p.ratingCount > 0
            ? <span className="ratewrap"><Stars rating={p.rating} /> <span className="ratecount">({p.ratingCount})</span></span>
            : <span className="ratecount norate">Pas encore noté</span>}
```
Add minimal CSS to `web/src/styles/catalog.css` (near the existing `.stars` rules):
```css
.ratewrap{display:inline-flex;align-items:center;gap:5px}
.ratecount{font-size:12px;color:var(--muted)}
.ratecount.norate{font-style:italic}
```

- [ ] **Step 5: Run to verify they pass + gate**

Run: `cd web && pnpm exec vitest run --exclude 'e2e/**' && pnpm exec tsc --noEmit && pnpm build`
Expected: all green.

- [ ] **Step 6: Commit**

```bash
git add web/src/api/types.ts web/src/api/client.ts web/src/components/ApiCard.tsx web/src/components/ApiCard.test.tsx web/src/styles/catalog.css web/src/api/client.ratings.test.ts
git commit -m "feat(web): ratings client fns + card count / no-rating state"
```

---

## Task 6: Reviews section on the product detail page

**Files:**
- Create: `web/src/pages/application/Reviews.tsx` (or `web/src/components/Reviews.tsx`)
- Modify: `web/src/pages/ProductDetailPage.tsx`
- Modify: `web/src/styles/productdetail.css`
- Test: `web/src/components/Reviews.test.tsx`

**Interfaces:**
- Consumes: `getRatings`, `submitRating` (Task 5); `Review`/`RatingsView`; `useAuth` (token); `formatRelative` from `../pages/application/activity` (adjust import path to where `Reviews` lives).
- Produces: `Reviews({ slug, token }: { slug: string; token: string | null })` rendered on `ProductDetailPage`.

- [ ] **Step 1: Write the failing tests**

Create `web/src/components/Reviews.test.tsx`:
```tsx
import { it, expect, vi, afterEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Reviews } from './Reviews'
import * as api from '../api/client'

afterEach(() => vi.restoreAllMocks())

it('renders the summary and review list', async () => {
  vi.spyOn(api, 'getRatings').mockResolvedValue({
    average: 4.5, count: 2, canRate: false, mine: null,
    items: [{ stars: 5, comment: 'top', author: 'Alice', createdAt: '2026-06-01T00:00:00Z' }],
  })
  render(<Reviews slug="orders" token={null} />)
  expect(await screen.findByText(/2 avis/)).toBeInTheDocument()
  expect(screen.getByText('top')).toBeInTheDocument()
  expect(screen.getByText('Alice')).toBeInTheDocument()
  // no form for a non-subscriber
  expect(screen.queryByRole('button', { name: /Publier|Envoyer|Noter/i })).not.toBeInTheDocument()
})

it('lets an approved subscriber submit a rating', async () => {
  vi.spyOn(api, 'getRatings').mockResolvedValue({ average: 0, count: 0, canRate: true, mine: null, items: [] })
  const submit = vi.spyOn(api, 'submitRating').mockResolvedValue({ average: 5, count: 1, canRate: true, mine: { stars: 5, comment: 'super', author: 'Me', createdAt: '' }, items: [{ stars: 5, comment: 'super', author: 'Me', createdAt: '' }] })
  render(<Reviews slug="orders" token="jwt" />)
  // pick 5 stars (the form exposes star buttons labelled "Noter N étoiles")
  await userEvent.click(await screen.findByRole('button', { name: /Noter 5/i }))
  await userEvent.type(screen.getByPlaceholderText(/commentaire/i), 'super')
  await userEvent.click(screen.getByRole('button', { name: /Publier|Envoyer/i }))
  await waitFor(() => expect(submit).toHaveBeenCalledWith('jwt', 'orders', { stars: 5, comment: 'super' }))
})

it('shows a subscribe prompt when authed but cannot rate', async () => {
  vi.spyOn(api, 'getRatings').mockResolvedValue({ average: 0, count: 0, canRate: false, mine: null, items: [] })
  render(<Reviews slug="orders" token="jwt" />)
  expect(await screen.findByText(/Abonnez-vous pour noter/i)).toBeInTheDocument()
})
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd web && pnpm exec vitest run src/components/Reviews.test.tsx`
Expected: FAIL — cannot find `./Reviews`.

- [ ] **Step 3: Implement `Reviews.tsx`**

Create `web/src/components/Reviews.tsx`:
```tsx
import { useEffect, useState } from 'react'
import { getRatings, submitRating } from '../api/client'
import type { RatingsView } from '../api/types'
import { formatRelative } from '../pages/application/activity'

function StarRow({ value }: { value: number }) {
  return <span className="rv-stars" aria-label={`${value}/5`}>{'★★★★★'.slice(0, value)}{'☆☆☆☆☆'.slice(0, 5 - value)}</span>
}

export function Reviews({ slug, token }: { slug: string; token: string | null }) {
  const [view, setView] = useState<RatingsView | null>(null)
  const [stars, setStars] = useState(0)
  const [comment, setComment] = useState('')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')

  function load() {
    getRatings(slug, token ?? undefined).then(v => {
      setView(v)
      if (v.mine) { setStars(v.mine.stars); setComment(v.mine.comment) }
    }).catch(() => setErr('Impossible de charger les avis.'))
  }
  useEffect(load, [slug, token])

  async function onSubmit() {
    if (!token || stars < 1 || busy) return
    setBusy(true); setErr('')
    try {
      const v = await submitRating(token, slug, { stars, comment: comment.trim() })
      setView(v); if (v.mine) { setStars(v.mine.stars); setComment(v.mine.comment) }
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Échec de l’envoi.')
    } finally { setBusy(false) }
  }

  if (!view) return null
  return (
    <section className="reviews">
      <div className="rv-head">
        <h3>Avis</h3>
        <span className="rv-summary">{view.count > 0 ? <><StarRow value={Math.round(view.average)} /> {view.average.toFixed(1)} · {view.count} avis</> : 'Pas encore noté'}</span>
      </div>

      {token && view.canRate && (
        <div className="rv-form">
          <div className="rv-pick" role="group" aria-label="Votre note">
            {[1, 2, 3, 4, 5].map(n => (
              <button key={n} type="button" aria-label={`Noter ${n} étoile${n > 1 ? 's' : ''}`}
                className={`rv-star ${n <= stars ? 'on' : ''}`} onClick={() => setStars(n)}>★</button>
            ))}
          </div>
          <textarea placeholder="Votre commentaire (optionnel)" value={comment} maxLength={500}
            onChange={e => setComment(e.target.value)} />
          <button className="btn btn-primary" disabled={busy || stars < 1} onClick={onSubmit}>
            {view.mine ? 'Mettre à jour' : 'Publier'}
          </button>
        </div>
      )}
      {token && !view.canRate && <p className="rv-note">Abonnez-vous pour noter cette API.</p>}
      {!token && <p className="rv-note">Connectez-vous pour noter cette API.</p>}
      {err && <p className="autherr" role="alert">{err}</p>}

      <ul className="rv-list">
        {view.items.map((rv, i) => (
          <li key={i} className="rv-item">
            <div className="rv-meta"><StarRow value={rv.stars} /> <b>{rv.author}</b> <span className="rv-when">{formatRelative(rv.createdAt)}</span></div>
            {rv.comment && <p className="rv-comment">{rv.comment}</p>}
          </li>
        ))}
      </ul>
    </section>
  )
}
```

- [ ] **Step 4: Add styles**

Append to `web/src/styles/productdetail.css`:
```css
.apidetail .reviews{margin-top:28px;border-top:1px solid var(--border-2);padding-top:22px}
.apidetail .rv-head{display:flex;align-items:baseline;justify-content:space-between;gap:12px;margin-bottom:14px}
.apidetail .rv-head h3{font-family:var(--font-display);font-size:18px;font-weight:700}
.apidetail .rv-summary{font-size:13px;color:var(--muted)}
.apidetail .rv-stars{color:var(--accent);letter-spacing:1px}
.apidetail .rv-form{display:flex;flex-direction:column;gap:10px;background:var(--surface);border:1px solid var(--border-2);border-radius:12px;padding:14px;margin-bottom:18px}
.apidetail .rv-pick{display:flex;gap:4px}
.apidetail .rv-star{font-size:22px;line-height:1;color:var(--border-2);background:none;border:none;cursor:pointer}
.apidetail .rv-star.on{color:var(--accent)}
.apidetail .rv-form textarea{min-height:64px;padding:10px 12px;border:1px solid var(--border-2);border-radius:10px;background:var(--bg);color:var(--fg);font:inherit;resize:vertical}
.apidetail .rv-note{font-size:13px;color:var(--muted);margin-bottom:16px}
.apidetail .rv-list{display:flex;flex-direction:column;gap:14px;list-style:none}
.apidetail .rv-item{border:1px solid var(--border-2);border-radius:12px;padding:12px 14px}
.apidetail .rv-meta{display:flex;align-items:center;gap:8px;font-size:13px}
.apidetail .rv-when{color:var(--faint);font-size:12px}
.apidetail .rv-comment{font-size:14px;color:var(--ink-soft,var(--fg));margin-top:6px}
```

- [ ] **Step 5: Render it on `ProductDetailPage`**

In `web/src/pages/ProductDetailPage.tsx`, import `Reviews` and the auth token, and render `<Reviews slug={slug} token={token} />` after the docs/placeholder block (inside the `product && (...)` fragment). `slug` is already from `useParams`; get `token` from `useAuth()` (the page already uses `useAuth` for `user` — also pull `token`).

- [ ] **Step 6: Run tests + full gate**

Run: `cd web && pnpm exec vitest run --exclude 'e2e/**' && pnpm exec tsc --noEmit && pnpm build`
Expected: all green.

- [ ] **Step 7: Commit**

```bash
git add web/src/components/Reviews.tsx web/src/components/Reviews.test.tsx web/src/pages/ProductDetailPage.tsx web/src/styles/productdetail.css
git commit -m "feat(web): Reviews section + rating form on the product detail page"
```

---

## Task 7: Live verification

- [ ] **Step 1: Stack + portal + vite running**

Dev `docker compose` up; portal on `:8090` (restart so migration 0010 + the `/api/ratings/` mount load); Vite on `:5173`.

- [ ] **Step 2: Rate as an approved subscriber**

Reuse the try-it/echo product's approved app owner (run-demo). With that user's token:
```bash
curl -s -X PUT http://localhost:8090/api/ratings/<slug> -H "Authorization: Bearer <TOKEN>" -H 'Content-Type: application/json' -d '{"stars":5,"comment":"excellent"}'
```
Expected: `200` with `{average:5, count:1, mine:{stars:5,...}, canRate:true, items:[...]}`.
Then GET it (no token) → public summary + items; a non-subscriber PUT → 403; bad stars → 400.

- [ ] **Step 3: Browser**

Open `/catalog/<slug>`: the header/card shows the real average + count; the Reviews section lists the review; as the subscriber the form is present and pre-filled; re-submitting edits in place (count stays 1). A non-subscribed/anon view shows the read-only list + the appropriate prompt. **Look at the screenshot.**

---

## Self-Review notes

- **Spec coverage:** product_ratings + denormalized avg/count + seed reset (T1) ✅; repo upsert/recompute/list/mine/summary (T2) ✅; GET public with mine/canRate + PUT approved-subscriber/stars-validation/anon-401 (T3) ✅; wiring/mount (T4) ✅; card avg+count/"Pas encore noté" + client fns (T5) ✅; detail Reviews list + form + prompts, edit-in-place via mine (T6) ✅; live (T7) ✅.
- **Type consistency:** `Review{Stars,Comment,Author,CreatedAt}` / `RatingsView{Average,Count,Items,Mine,CanRate}` consistent Go↔TS; `getRatings(slug,token?)`, `submitRating(token,slug,{stars,comment})`, `ProductBySlug→(id,err)`, `IsApprovedSubscriber`, `Upsert/List/Mine/SummaryFor` consistent across tasks.
- **Implementer notes:** T3's `testTok` must match the REAL `auth.Tokenizer` constructor + issue/parse API and `Claims` field names (read `internal/auth/token.go`) — adjust the test helper accordingly. T1's seed-reset may require updating any catalog test that asserts a non-zero seeded `rating`. `Reviews.tsx`'s import of `formatRelative` must match its final file location.
