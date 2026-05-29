# APISIX Developer Portal — Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up a runnable, tested backend for the APISIX Developer Portal: local infra (Postgres + etcd + APISIX), database with migrations, a seeded API catalog with a read API, and local-account auth.

**Architecture:** Go HTTP service (chi router) backed by PostgreSQL (pgx), is the source of truth for the catalog and users. Apache APISIX + etcd run alongside for later phases. Migrations and seed data are embedded SQL run at startup. Auth is local accounts (bcrypt + JWT). No frontend in this plan (verified via `go test` and `curl`).

**Tech Stack:** Go 1.22+, chi v5, pgx v5 (pgxpool), golang-jwt v5, bcrypt (golang.org/x/crypto), Docker Compose, Apache APISIX 3.x, etcd, PostgreSQL 16.

This plan is part 1 of 4 (Foundation → Frontend → Core subscribe loop → Admin). See `docs/superpowers/specs/2026-05-29-apisix-developer-portal-design.md`.

---

## File structure (created by this plan)

```
apisix-developper-portal/
├── go.mod
├── Makefile
├── .gitignore
├── .env.example
├── docker-compose.yml
├── deploy/apisix/config.yaml
├── cmd/portal/main.go            # entrypoint: config, pool, migrate, router, serve
├── internal/
│   ├── config/config.go          # env-based config
│   ├── db/db.go                  # pgxpool connect
│   ├── db/migrate.go             # embedded migration runner
│   ├── db/migrations/*.sql       # schema + seed
│   ├── catalog/product.go        # ApiProduct type + query params
│   ├── catalog/repo.go           # ProductRepository (pgx)
│   ├── catalog/repo_test.go
│   ├── catalog/handler.go        # GET /api/products, /api/products/{slug}
│   ├── catalog/handler_test.go
│   ├── auth/user.go              # User type, password hashing
│   ├── auth/user_test.go
│   ├── auth/repo.go              # UserRepository (pgx)
│   ├── auth/token.go             # JWT issue/verify
│   ├── auth/token_test.go
│   ├── auth/handler.go           # POST /api/auth/register, /api/auth/login
│   ├── auth/handler_test.go
│   └── httpx/respond.go          # JSON helpers
```

Each file has one responsibility. `catalog` and `auth` are independent packages with their own repo + handler + tests. `httpx` holds shared JSON helpers. `db` owns connection + migrations only.

---

## Task 1: Repo scaffold, git, Go module

**Files:**
- Create: `.gitignore`, `go.mod`, `Makefile`, `internal/httpx/respond.go`

- [ ] **Step 1: Initialize git and Go module**

Run:
```bash
cd /home/walidboudiche/working/apisix-developper-portal
git init
go mod init apisix-portal
```
Expected: `go.mod` created with `module apisix-portal` and a `go 1.2x` line.

- [ ] **Step 2: Write `.gitignore`**

```gitignore
# build / env
/bin/
*.env
.env
# go
/vendor/
# tooling
.superpowers/
.tmp/
node_modules/
# keep the design mockup and docs
!index.html
```

- [ ] **Step 3: Write `internal/httpx/respond.go`**

```go
package httpx

import (
	"encoding/json"
	"net/http"
)

// JSON writes v as a JSON response with the given status code.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Error writes a {"error": msg} body with the given status code.
func Error(w http.ResponseWriter, status int, msg string) {
	JSON(w, status, map[string]string{"error": msg})
}
```

- [ ] **Step 4: Write `Makefile`**

```makefile
.PHONY: up down run test tidy
up:          ; docker compose up -d
down:        ; docker compose down
run:         ; go run ./cmd/portal
test:        ; go test ./...
tidy:        ; go mod tidy
```

- [ ] **Step 5: Commit**

```bash
git add .gitignore go.mod Makefile internal/httpx/respond.go index.html docs/
git commit -m "chore: scaffold Go module, gitignore, makefile, json helpers"
```

---

## Task 2: Local infrastructure (docker-compose)

**Files:**
- Create: `docker-compose.yml`, `deploy/apisix/config.yaml`, `.env.example`

- [ ] **Step 1: Write `deploy/apisix/config.yaml`**

```yaml
apisix:
  node_listen: 9080
  enable_admin: true
deployment:
  admin:
    admin_key:
      - name: admin
        key: edd1c9f034335f136f87ad84b625c8f1
        role: admin
    allow_admin:
      - 0.0.0.0/0
  etcd:
    host:
      - "http://etcd:2379"
    prefix: /apisix
    timeout: 30
```

- [ ] **Step 2: Write `.env.example`**

```bash
DATABASE_URL=postgres://portal:portal@localhost:5432/portal?sslmode=disable
PORTAL_ADDR=:8080
JWT_SECRET=dev-secret-change-me
APISIX_ADMIN_URL=http://localhost:9180
APISIX_ADMIN_KEY=edd1c9f034335f136f87ad84b625c8f1
```

- [ ] **Step 3: Write `docker-compose.yml`**

```yaml
name: apisix-portal
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: portal
      POSTGRES_PASSWORD: portal
      POSTGRES_DB: portal
    ports: ["5432:5432"]
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U portal"]
      interval: 5s
      timeout: 3s
      retries: 10

  etcd:
    image: bitnami/etcd:3.5
    environment:
      ALLOW_NONE_AUTHENTICATION: "yes"
      ETCD_ADVERTISE_CLIENT_URLS: http://etcd:2379
    ports: ["2379:2379"]

  apisix:
    image: apache/apisix:3.9.1-debian
    depends_on: [etcd]
    volumes:
      - ./deploy/apisix/config.yaml:/usr/local/apisix/conf/config.yaml:ro
    ports:
      - "9080:9080"   # gateway data plane
      - "9180:9180"   # admin API
```

- [ ] **Step 4: Bring it up and verify**

Run:
```bash
docker compose up -d
sleep 8
docker compose ps
curl -s http://127.0.0.1:9180/apisix/admin/routes -H 'X-API-KEY: edd1c9f034335f136f87ad84b625c8f1' | head -c 120
```
Expected: all services `running`/`healthy`; the admin curl returns a JSON body (a `{"list":...}` / `{"node":...}` shape), not a connection error.

- [ ] **Step 5: Commit**

```bash
git add docker-compose.yml deploy/apisix/config.yaml .env.example
git commit -m "feat: docker-compose for postgres, etcd, apisix"
```

---

## Task 3: Config + database pool

**Files:**
- Create: `internal/config/config.go`, `internal/db/db.go`

- [ ] **Step 1: Add dependencies**

Run:
```bash
go get github.com/jackc/pgx/v5/pgxpool@latest
go get github.com/go-chi/chi/v5@latest
```
Expected: `go.mod`/`go.sum` updated.

- [ ] **Step 2: Write `internal/config/config.go`**

```go
package config

import "os"

type Config struct {
	DatabaseURL   string
	Addr          string
	JWTSecret     string
	APISIXAdminURL string
	APISIXAdminKey string
}

func get(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Load reads configuration from the environment, applying dev defaults.
func Load() Config {
	return Config{
		DatabaseURL:    get("DATABASE_URL", "postgres://portal:portal@localhost:5432/portal?sslmode=disable"),
		Addr:           get("PORTAL_ADDR", ":8080"),
		JWTSecret:      get("JWT_SECRET", "dev-secret-change-me"),
		APISIXAdminURL: get("APISIX_ADMIN_URL", "http://localhost:9180"),
		APISIXAdminKey: get("APISIX_ADMIN_KEY", "edd1c9f034335f136f87ad84b625c8f1"),
	}
}
```

- [ ] **Step 3: Write `internal/db/db.go`**

```go
package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect opens a pgx pool and verifies connectivity with a ping.
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}
```

- [ ] **Step 4: Verify it builds**

Run: `go build ./...`
Expected: no output (success).

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum internal/config/config.go internal/db/db.go
git commit -m "feat: config loader and postgres pool"
```

---

## Task 4: Schema + seed migrations

**Files:**
- Create: `internal/db/migrate.go`, `internal/db/migrations/0001_init.sql`, `internal/db/migrations/0002_seed.sql`

- [ ] **Step 1: Write `internal/db/migrations/0001_init.sql`**

```sql
CREATE TABLE IF NOT EXISTS users (
    id            BIGSERIAL PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    name          TEXT NOT NULL DEFAULT '',
    role          TEXT NOT NULL DEFAULT 'developer',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS plans (
    id                  BIGSERIAL PRIMARY KEY,
    name                TEXT NOT NULL UNIQUE,
    rate_limit_count    INT  NOT NULL,
    rate_limit_window_s INT  NOT NULL
);

CREATE TABLE IF NOT EXISTS api_products (
    id           BIGSERIAL PRIMARY KEY,
    name         TEXT NOT NULL,
    slug         TEXT NOT NULL UNIQUE,
    category     TEXT NOT NULL,
    version      TEXT NOT NULL DEFAULT '1.0.0',
    context_path TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    tags         TEXT[] NOT NULL DEFAULT '{}',
    icon         TEXT NOT NULL DEFAULT '',
    rating       NUMERIC(2,1) NOT NULL DEFAULT 0,
    published    BOOLEAN NOT NULL DEFAULT true,
    upstream_url TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_api_products_category ON api_products(category);
```

- [ ] **Step 2: Write `internal/db/migrations/0002_seed.sql`** (the 9 sample APIs from the Atlas mockup + 3 plans)

```sql
INSERT INTO plans (name, rate_limit_count, rate_limit_window_s) VALUES
  ('Free',   60,   60),
  ('Silver', 300,  60),
  ('Gold',   1000, 60)
ON CONFLICT (name) DO NOTHING;

INSERT INTO api_products (name, slug, category, version, context_path, description, tags, icon, rating) VALUES
  ('SEOAPI','seoapi','Marketing','1.0.0','/seo','Audit on-page, suivi de positions et analyse de backlinks à la demande.','{seo,marketing,temps-réel}','seo',5.0),
  ('ReviewsAPI','reviewsapi','Marketing','1.0.0','/reviews','Collecte et agrégation d''avis clients depuis plusieurs sources.','{avis,marketing}','reviews',4.5),
  ('StockAnalysisAPI','stockanalysisapi','Finance','1.0.0','/stockAnalysis','Cours boursiers, indicateurs techniques et signaux en temps réel.','{finance,temps-réel}','stock',4.0),
  ('testAPI','testapi','Engineering','1.0','/test','Bac à sable pour valider vos intégrations avant la mise en production.','{sandbox,interne}','test',3.0),
  ('KeyWordResearchAPI','keywordresearchapi','Marketing','1.0.0','/keyword','Volume de recherche, difficulté et suggestions de mots-clés.','{seo,mots-clés}','keyword',4.5),
  ('PeopleAPI','peopleapi','Administration','1.0.0','/people','Annuaire, rôles et provisioning des utilisateurs de l''organisation.','{identité,admin}','people',4.0),
  ('CurrencyConverterAPI','currencyconverterapi','Finance','1.0.0','/currencyconv','Taux de change actualisés et conversion multidevise instantanée.','{finance,devises}','currency',5.0),
  ('PhoneVerification','phoneverification','Administration','1.0','/phoneverify','Vérification de numéros et envoi de codes OTP par SMS.','{otp,identité}','phone',4.0),
  ('PizzaShackAPI','pizzashackapi','Engineering','1.0.0','/pizzashack','Commande, suivi de livraison et menu — l''API de démonstration.','{pizza,démo}','pizza',4.5)
ON CONFLICT (slug) DO NOTHING;
```

- [ ] **Step 3: Write `internal/db/migrate.go`** (embeds and runs the SQL files in lexical order)

```go
package db

import (
	"context"
	"embed"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate runs every migrations/*.sql file once, in filename order,
// tracking applied files in a schema_migrations table.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx,
		`CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		var exists bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE name=$1)`, name).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		sqlBytes, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO schema_migrations(name) VALUES($1)`, name); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 4: Verify build**

Run: `go build ./...`
Expected: success (no output).

- [ ] **Step 5: Commit**

```bash
git add internal/db/migrate.go internal/db/migrations/
git commit -m "feat: embedded migrations with schema and seed data"
```

---

## Task 5: ApiProduct type + repository (TDD against Postgres)

**Files:**
- Create: `internal/catalog/product.go`, `internal/catalog/repo.go`, `internal/catalog/repo_test.go`

> Tests run against the dev Postgres from docker-compose. Ensure `make up` has run and `DATABASE_URL` points at it.

- [ ] **Step 1: Write `internal/catalog/product.go`**

```go
package catalog

// Product is a published API in the catalog.
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
	Rating      float64  `json:"rating"`
}

// Query holds catalog filter/search/sort parameters.
type Query struct {
	Search   string // matches name/description (case-insensitive)
	Category string // exact category, empty = all
	Tag      string // must be present in tags, empty = all
	Sort     string // "alpha" | "rating" (default)
}
```

- [ ] **Step 2: Write the failing test `internal/catalog/repo_test.go`**

```go
package catalog

import (
	"context"
	"os"
	"testing"

	"apisix-portal/internal/db"
)

func testPool(t *testing.T) (context.Context, *Repo) {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://portal:portal@localhost:5432/portal?sslmode=disable"
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, url)
	if err != nil {
		t.Skipf("no database available: %v", err)
	}
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(pool.Close)
	return ctx, NewRepo(pool)
}

func TestListReturnsSeededProducts(t *testing.T) {
	ctx, repo := testPool(t)
	all, err := repo.List(ctx, Query{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 9 {
		t.Fatalf("expected 9 seeded products, got %d", len(all))
	}
}

func TestListFiltersByCategory(t *testing.T) {
	ctx, repo := testPool(t)
	fin, err := repo.List(ctx, Query{Category: "Finance"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(fin) != 2 {
		t.Fatalf("expected 2 Finance products, got %d", len(fin))
	}
}

func TestGetBySlug(t *testing.T) {
	ctx, repo := testPool(t)
	p, err := repo.GetBySlug(ctx, "pizzashackapi")
	if err != nil {
		t.Fatalf("GetBySlug: %v", err)
	}
	if p.Name != "PizzaShackAPI" {
		t.Fatalf("expected PizzaShackAPI, got %q", p.Name)
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/catalog/ -run TestList -v`
Expected: compile error / FAIL — `Repo`, `NewRepo`, `List`, `GetBySlug` not defined.

- [ ] **Step 4: Write `internal/catalog/repo.go`**

```go
package catalog

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a product does not exist.
var ErrNotFound = errors.New("product not found")

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

const baseSelect = `SELECT id,name,slug,category,version,context_path,description,tags,icon,rating
	FROM api_products WHERE published = true`

func scan(rows pgx.Rows) ([]Product, error) {
	var out []Product
	for rows.Next() {
		var p Product
		if err := rows.Scan(&p.ID, &p.Name, &p.Slug, &p.Category, &p.Version,
			&p.ContextPath, &p.Description, &p.Tags, &p.Icon, &p.Rating); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// List returns published products matching the query.
func (r *Repo) List(ctx context.Context, q Query) ([]Product, error) {
	sql := baseSelect
	args := []any{}
	add := func(cond string, val any) {
		args = append(args, val)
		sql += " AND " + strings.Replace(cond, "?", "$"+itoa(len(args)), 1)
	}
	if q.Category != "" {
		add("category = ?", q.Category)
	}
	if q.Tag != "" {
		add("? = ANY(tags)", q.Tag)
	}
	if q.Search != "" {
		add("(name ILIKE ? OR description ILIKE ?)", "%"+q.Search+"%")
		// second placeholder reuses the same arg
		args = append(args, "%"+q.Search+"%")
		sql = strings.Replace(sql, "OR description ILIKE $"+itoa(len(args)-1),
			"OR description ILIKE $"+itoa(len(args)), 1)
	}
	if q.Sort == "alpha" {
		sql += " ORDER BY name ASC"
	} else {
		sql += " ORDER BY rating DESC, name ASC"
	}
	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scan(rows)
}

// GetBySlug returns a single published product or ErrNotFound.
func (r *Repo) GetBySlug(ctx context.Context, slug string) (Product, error) {
	rows, err := r.pool.Query(ctx, baseSelect+" AND slug = $1", slug)
	if err != nil {
		return Product{}, err
	}
	defer rows.Close()
	ps, err := scan(rows)
	if err != nil {
		return Product{}, err
	}
	if len(ps) == 0 {
		return Product{}, ErrNotFound
	}
	return ps[0], nil
}

func itoa(n int) string { return strings.TrimSpace(strings.Map(func(r rune) rune { return r }, intToStr(n))) }
func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}
```

> Note: the search ILIKE handling above is intentionally explicit. If the reviewer prefers, replace the two-placeholder dance with a `pgx` named approach — but keep behavior: search matches name OR description, case-insensitively.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `make up && go test ./internal/catalog/ -v`
Expected: `TestListReturnsSeededProducts`, `TestListFiltersByCategory`, `TestGetBySlug` all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/catalog/product.go internal/catalog/repo.go internal/catalog/repo_test.go
git commit -m "feat: catalog product repository with filter/search/sort (TDD)"
```

---

## Task 6: Catalog HTTP handler (TDD)

**Files:**
- Create: `internal/catalog/handler.go`, `internal/catalog/handler_test.go`

- [ ] **Step 1: Write the failing test `internal/catalog/handler_test.go`**

```go
package catalog

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeLister implements the Lister interface without a database.
type fakeLister struct{ items []Product }

func (f fakeLister) List(_ contextStub, q Query) ([]Product, error) {
	if q.Category == "Finance" {
		return f.items[:1], nil
	}
	return f.items, nil
}
func (f fakeLister) GetBySlug(_ contextStub, slug string) (Product, error) {
	for _, p := range f.items {
		if p.Slug == slug {
			return p, nil
		}
	}
	return Product{}, ErrNotFound
}

func TestProductsEndpointReturnsJSON(t *testing.T) {
	h := NewHandler(fakeLister{items: []Product{{Name: "A", Slug: "a"}, {Name: "B", Slug: "b"}}})
	req := httptest.NewRequest(http.MethodGet, "/api/products", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got []Product
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d products, want 2", len(got))
	}
}

func TestProductsEndpointFiltersByCategory(t *testing.T) {
	h := NewHandler(fakeLister{items: []Product{{Slug: "a"}, {Slug: "b"}}})
	req := httptest.NewRequest(http.MethodGet, "/api/products?category=Finance", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var got []Product
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got) != 1 {
		t.Fatalf("got %d, want 1 filtered", len(got))
	}
}
```

> The test uses `contextStub` so the interface does not import `context` noise into the test signature; define it in the handler file as an alias.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/catalog/ -run TestProducts -v`
Expected: compile error — `NewHandler`, `Lister`, `contextStub` undefined.

- [ ] **Step 3: Write `internal/catalog/handler.go`**

```go
package catalog

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"apisix-portal/internal/httpx"
)

type contextStub = context.Context

// Lister is the read surface the handler needs (satisfied by *Repo).
type Lister interface {
	List(ctx context.Context, q Query) ([]Product, error)
	GetBySlug(ctx context.Context, slug string) (Product, error)
}

type Handler struct {
	repo   Lister
	router chi.Router
}

func NewHandler(repo Lister) *Handler {
	h := &Handler{repo: repo, router: chi.NewRouter()}
	h.router.Get("/api/products", h.list)
	h.router.Get("/api/products/{slug}", h.getBySlug)
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.router.ServeHTTP(w, r) }

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	q := Query{
		Search:   r.URL.Query().Get("search"),
		Category: r.URL.Query().Get("category"),
		Tag:      r.URL.Query().Get("tag"),
		Sort:     r.URL.Query().Get("sort"),
	}
	items, err := h.repo.List(r.Context(), q)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to list products")
		return
	}
	if items == nil {
		items = []Product{}
	}
	httpx.JSON(w, http.StatusOK, items)
}

func (h *Handler) getBySlug(w http.ResponseWriter, r *http.Request) {
	p, err := h.repo.GetBySlug(r.Context(), chi.URLParam(r, "slug"))
	if err == ErrNotFound {
		httpx.Error(w, http.StatusNotFound, "product not found")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to load product")
		return
	}
	httpx.JSON(w, http.StatusOK, p)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/catalog/ -run TestProducts -v`
Expected: both PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/catalog/handler.go internal/catalog/handler_test.go
git commit -m "feat: catalog HTTP handler GET /api/products[/{slug}] (TDD)"
```

---

## Task 7: User type + password hashing (TDD)

**Files:**
- Create: `internal/auth/user.go`, `internal/auth/user_test.go`

- [ ] **Step 1: Add dependency**

Run: `go get golang.org/x/crypto/bcrypt@latest`

- [ ] **Step 2: Write the failing test `internal/auth/user_test.go`**

```go
package auth

import "testing"

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("s3cret!")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == "s3cret!" {
		t.Fatal("hash must not equal the plaintext")
	}
	if !CheckPassword(hash, "s3cret!") {
		t.Fatal("correct password should verify")
	}
	if CheckPassword(hash, "wrong") {
		t.Fatal("wrong password must not verify")
	}
}
```

- [ ] **Step 3: Run to verify it fails**

Run: `go test ./internal/auth/ -run TestHashAndVerify -v`
Expected: compile error — `HashPassword`/`CheckPassword` undefined.

- [ ] **Step 4: Write `internal/auth/user.go`**

```go
package auth

import "golang.org/x/crypto/bcrypt"

type User struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"`
}

// HashPassword returns a bcrypt hash of the plaintext password.
func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	return string(b), err
}

// CheckPassword reports whether plain matches the stored bcrypt hash.
func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
```

- [ ] **Step 5: Run to verify it passes**

Run: `go test ./internal/auth/ -run TestHashAndVerify -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/auth/user.go internal/auth/user_test.go
git commit -m "feat: user type and bcrypt password hashing (TDD)"
```

---

## Task 8: JWT issue/verify (TDD)

**Files:**
- Create: `internal/auth/token.go`, `internal/auth/token_test.go`

- [ ] **Step 1: Add dependency**

Run: `go get github.com/golang-jwt/jwt/v5@latest`

- [ ] **Step 2: Write the failing test `internal/auth/token_test.go`**

```go
package auth

import "testing"

func TestIssueAndParseToken(t *testing.T) {
	tk := NewTokenizer("test-secret")
	token, err := tk.Issue(42, "dev@example.com", "developer")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	claims, err := tk.Parse(token)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if claims.UserID != 42 || claims.Email != "dev@example.com" || claims.Role != "developer" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestParseRejectsBadSecret(t *testing.T) {
	good := NewTokenizer("secret-a")
	bad := NewTokenizer("secret-b")
	token, _ := good.Issue(1, "a@b.c", "developer")
	if _, err := bad.Parse(token); err == nil {
		t.Fatal("token signed with a different secret must not verify")
	}
}
```

- [ ] **Step 3: Run to verify it fails**

Run: `go test ./internal/auth/ -run TestIssueAndParse -v`
Expected: compile error — `NewTokenizer` undefined.

- [ ] **Step 4: Write `internal/auth/token.go`**

```go
package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID int64  `json:"uid"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

type Tokenizer struct{ secret []byte }

func NewTokenizer(secret string) *Tokenizer { return &Tokenizer{secret: []byte(secret)} }

// Issue creates a signed JWT valid for 24h.
func (t *Tokenizer) Issue(uid int64, email, role string) (string, error) {
	claims := Claims{
		UserID: uid, Email: email, Role: role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(t.secret)
}

// Parse verifies the token signature/expiry and returns its claims.
func (t *Tokenizer) Parse(tokenStr string) (*Claims, error) {
	tok, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return t.secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := tok.Claims.(*Claims)
	if !ok || !tok.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}
```

- [ ] **Step 5: Run to verify it passes**

Run: `go test ./internal/auth/ -run 'TestIssueAndParse|TestParseRejects' -v`
Expected: both PASS.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/auth/token.go internal/auth/token_test.go
git commit -m "feat: JWT issue/verify (TDD)"
```

---

## Task 9: User repository (TDD against Postgres)

**Files:**
- Create: `internal/auth/repo.go`

- [ ] **Step 1: Write the failing test — append to `internal/auth/user_test.go`**

```go
// --- appended to internal/auth/user_test.go ---
import_block_note := "ensure these imports are present at top of file"
_ = import_block_note
```

Replace the top of `internal/auth/user_test.go` so its imports read:

```go
package auth

import (
	"context"
	"os"
	"testing"

	"apisix-portal/internal/db"
)
```

Then add:

```go
func testUserRepo(t *testing.T) (context.Context, *Repo) {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://portal:portal@localhost:5432/portal?sslmode=disable"
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, url)
	if err != nil {
		t.Skipf("no database available: %v", err)
	}
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(pool.Close)
	return ctx, NewRepo(pool)
}

func TestCreateAndGetUser(t *testing.T) {
	ctx, repo := testUserRepo(t)
	email := "dev+" + randSuffix() + "@example.com"
	u, err := repo.Create(ctx, email, "hash", "Dev")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if u.ID == 0 || u.Role != "developer" {
		t.Fatalf("unexpected user: %+v", u)
	}
	got, hash, err := repo.GetByEmail(ctx, email)
	if err != nil {
		t.Fatalf("GetByEmail: %v", err)
	}
	if got.ID != u.ID || hash != "hash" {
		t.Fatalf("mismatch: %+v hash=%q", got, hash)
	}
}

func TestCreateDuplicateEmailFails(t *testing.T) {
	ctx, repo := testUserRepo(t)
	email := "dup+" + randSuffix() + "@example.com"
	if _, err := repo.Create(ctx, email, "h", "A"); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if _, err := repo.Create(ctx, email, "h", "B"); err == nil {
		t.Fatal("duplicate email should fail")
	}
}

func randSuffix() string {
	return time.Now().Format("150405.000000")
}
```

Add `"time"` to the test imports.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/auth/ -run TestCreate -v`
Expected: compile error — `NewRepo`, `Create`, `GetByEmail` undefined.

- [ ] **Step 3: Write `internal/auth/repo.go`**

```go
package auth

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

// Create inserts a developer user and returns it.
func (r *Repo) Create(ctx context.Context, email, passwordHash, name string) (User, error) {
	var u User
	err := r.pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, name, role)
		 VALUES ($1,$2,$3,'developer')
		 RETURNING id, email, name, role`,
		email, passwordHash, name,
	).Scan(&u.ID, &u.Email, &u.Name, &u.Role)
	return u, err
}

// GetByEmail returns the user and its password hash.
func (r *Repo) GetByEmail(ctx context.Context, email string) (User, string, error) {
	var u User
	var hash string
	err := r.pool.QueryRow(ctx,
		`SELECT id, email, name, role, password_hash FROM users WHERE email=$1`, email,
	).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &hash)
	return u, hash, err
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/auth/ -run TestCreate -v`
Expected: `TestCreateAndGetUser`, `TestCreateDuplicateEmailFails` PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/auth/repo.go internal/auth/user_test.go
git commit -m "feat: user repository create/get (TDD)"
```

---

## Task 10: Auth HTTP handler — register & login (TDD)

**Files:**
- Create: `internal/auth/handler.go`, `internal/auth/handler_test.go`

- [ ] **Step 1: Write the failing test `internal/auth/handler_test.go`**

```go
package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// memRepo is an in-memory UserStore for handler tests.
type memRepo struct {
	byEmail map[string]struct {
		u    User
		hash string
	}
	nextID int64
}

func newMemRepo() *memRepo {
	return &memRepo{byEmail: map[string]struct {
		u    User
		hash string
	}{}}
}

func (m *memRepo) Create(_ context.Context, email, hash, name string) (User, error) {
	if _, ok := m.byEmail[email]; ok {
		return User{}, errors.New("duplicate")
	}
	m.nextID++
	u := User{ID: m.nextID, Email: email, Name: name, Role: "developer"}
	m.byEmail[email] = struct {
		u    User
		hash string
	}{u, hash}
	return u, nil
}

func (m *memRepo) GetByEmail(_ context.Context, email string) (User, string, error) {
	v, ok := m.byEmail[email]
	if !ok {
		return User{}, "", errors.New("not found")
	}
	return v.u, v.hash, nil
}

func newTestHandler() *Handler {
	return NewHandler(newMemRepo(), NewTokenizer("test-secret"))
}

func TestRegisterThenLogin(t *testing.T) {
	h := newTestHandler()

	reg := httptest.NewRequest(http.MethodPost, "/api/auth/register",
		strings.NewReader(`{"email":"a@b.c","password":"pw123456","name":"A"}`))
	regRec := httptest.NewRecorder()
	h.ServeHTTP(regRec, reg)
	if regRec.Code != http.StatusCreated {
		t.Fatalf("register status = %d, want 201; body=%s", regRec.Code, regRec.Body)
	}

	login := httptest.NewRequest(http.MethodPost, "/api/auth/login",
		strings.NewReader(`{"email":"a@b.c","password":"pw123456"}`))
	loginRec := httptest.NewRecorder()
	h.ServeHTTP(loginRec, login)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login status = %d, want 200", loginRec.Code)
	}
	if !strings.Contains(loginRec.Body.String(), `"token"`) {
		t.Fatalf("login response missing token: %s", loginRec.Body)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	h := newTestHandler()
	reg := httptest.NewRequest(http.MethodPost, "/api/auth/register",
		strings.NewReader(`{"email":"a@b.c","password":"pw123456","name":"A"}`))
	h.ServeHTTP(httptest.NewRecorder(), reg)

	login := httptest.NewRequest(http.MethodPost, "/api/auth/login",
		strings.NewReader(`{"email":"a@b.c","password":"WRONG"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, login)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/auth/ -run 'TestRegister|TestLoginWrong' -v`
Expected: compile error — `Handler`, `NewHandler` undefined.

- [ ] **Step 3: Write `internal/auth/handler.go`**

```go
package auth

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"apisix-portal/internal/httpx"
)

// UserStore is the persistence surface the handler needs (satisfied by *Repo).
type UserStore interface {
	Create(ctx context.Context, email, passwordHash, name string) (User, error)
	GetByEmail(ctx context.Context, email string) (User, string, error)
}

type Handler struct {
	store  UserStore
	tk     *Tokenizer
	router chi.Router
}

func NewHandler(store UserStore, tk *Tokenizer) *Handler {
	h := &Handler{store: store, tk: tk, router: chi.NewRouter()}
	h.router.Post("/api/auth/register", h.register)
	h.router.Post("/api/auth/login", h.login)
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.router.ServeHTTP(w, r) }

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil || c.Email == "" || len(c.Password) < 8 {
		httpx.Error(w, http.StatusBadRequest, "email and password (min 8 chars) required")
		return
	}
	hash, err := HashPassword(c.Password)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not hash password")
		return
	}
	u, err := h.store.Create(r.Context(), c.Email, hash, c.Name)
	if err != nil {
		httpx.Error(w, http.StatusConflict, "email already registered")
		return
	}
	token, err := h.tk.Issue(u.ID, u.Email, u.Role)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not issue token")
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"user": u, "token": token})
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	u, hash, err := h.store.GetByEmail(r.Context(), c.Email)
	if err != nil || !CheckPassword(hash, c.Password) {
		httpx.Error(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	token, err := h.tk.Issue(u.ID, u.Email, u.Role)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not issue token")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"user": u, "token": token})
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/auth/ -run 'TestRegister|TestLoginWrong' -v`
Expected: both PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/auth/handler.go internal/auth/handler_test.go
git commit -m "feat: auth register/login HTTP handlers (TDD)"
```

---

## Task 11: Wire the entrypoint and smoke-test the running service

**Files:**
- Create: `cmd/portal/main.go`

- [ ] **Step 1: Write `cmd/portal/main.go`**

```go
package main

import (
	"context"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"apisix-portal/internal/auth"
	"apisix-portal/internal/catalog"
	"apisix-portal/internal/config"
	"apisix-portal/internal/db"
)

func main() {
	ctx := context.Background()
	cfg := config.Load()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	catalogH := catalog.NewHandler(catalog.NewRepo(pool))
	authH := auth.NewHandler(auth.NewRepo(pool), auth.NewTokenizer(cfg.JWTSecret))

	r := chi.NewRouter()
	r.Use(middleware.Logger, middleware.Recoverer)
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	r.Mount("/", catalogH)
	r.Mount("/auth", authH) // mounts /auth + the handler's own /api/auth/* -> see note

	log.Printf("portal listening on %s", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, r); err != nil {
		log.Fatal(err)
	}
}
```

> Routing note: both handlers register absolute paths (`/api/products`, `/api/auth/register`) on their own internal chi routers. Mount them at root so paths resolve as written:
> ```go
> r.Mount("/", catalogH)
> r.Mount("/", authH)
> ```
> Replace the two `r.Mount` lines above with these two root mounts. chi allows multiple mounts at `/` as long as the concrete routes do not collide (they don't: `/api/products*` vs `/api/auth/*`).

- [ ] **Step 2: Run the full test suite**

Run: `make up && go test ./...`
Expected: all packages PASS (catalog + auth, including DB-backed tests).

- [ ] **Step 3: Run the service and smoke-test every endpoint**

Run (in one shell):
```bash
go run ./cmd/portal
```
In another shell:
```bash
curl -s localhost:8080/healthz; echo
curl -s 'localhost:8080/api/products' | head -c 200; echo
curl -s 'localhost:8080/api/products?category=Finance' | python3 -c 'import sys,json;print(len(json.load(sys.stdin)),"finance apis")'
curl -s localhost:8080/api/products/pizzashackapi | python3 -c 'import sys,json;print(json.load(sys.stdin)["name"])'
curl -s -X POST localhost:8080/api/auth/register -d '{"email":"dev@example.com","password":"pw123456","name":"Dev"}' | python3 -c 'import sys,json;print("token" in json.load(sys.stdin))'
curl -s -X POST localhost:8080/api/auth/login -d '{"email":"dev@example.com","password":"pw123456"}' | python3 -c 'import sys,json;print("token" in json.load(sys.stdin))'
```
Expected: `ok`; a JSON array of products; `2 finance apis`; `PizzaShackAPI`; `True`; `True`.

- [ ] **Step 4: Commit**

```bash
git add cmd/portal/main.go
git commit -m "feat: wire entrypoint (migrate, catalog + auth routes, healthz)"
```

---

## Self-review notes (author)

- **Spec coverage:** local auth (Tasks 7–10) ✓; catalog browse/search/filter/sort read API (Tasks 5–6) ✓; Postgres source-of-truth + migrations + seed of the 9 sample APIs (Task 4) ✓; docker-compose with postgres/etcd/apisix (Task 2) ✓; `CredentialProvider`/`AuthProvider` interfaces and the subscribe loop are **deferred to Plan 3** by design; admin UI to **Plan 4**; React frontend to **Plan 2**.
- **No placeholders:** every code step contains complete, compilable code.
- **Type consistency:** `catalog.Lister`/`*Repo`, `auth.UserStore`/`*Repo`, `Tokenizer`, `Claims`, `Product`, `Query` names are used identically across tasks. The DB-backed tests `t.Skip` when no database is present so the suite still runs in CI without Postgres.
```
