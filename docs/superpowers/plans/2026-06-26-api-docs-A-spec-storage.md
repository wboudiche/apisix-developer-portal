# Interactive API Docs — Plan A: Spec Storage Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist an OpenAPI spec on each API product, keep the spec when importing (instead of discarding it), let admins attach/replace one on the product form, and serve it publicly — so later plans can render docs and run Try-it.

**Architecture:** Add an `openapi_spec` TEXT column to `api_products`. The admin create/update path stores it; UPDATE preserves the existing spec when the incoming value is empty (so editing a product never wipes its docs). The OpenAPI import endpoint now returns the raw spec on the draft so the create payload carries it through. A public `GET /api/products/{slug}/spec` serves the raw spec for published products.

**Tech Stack:** Go 1.25 (chi, pgx), React 19 + TS (Vite, vitest).

## Global Constraints

- Go module `apisix-portal`; admin product code in `internal/admin`, catalog in `internal/catalog`.
- Spec stored as raw text (JSON or YAML) in `api_products.openapi_spec`, default `''` (empty = product has no docs).
- A non-empty spec must parse via the existing `parseSpec` (in `internal/admin/import.go`) or the write is rejected `400`.
- UPDATE semantics: an empty incoming `openapiSpec` means "leave the stored spec unchanged"; a non-empty one replaces it. (Clearing a spec is out of scope — re-import to replace.)
- The admin product LIST must NOT return the spec body (it can be large); only the dedicated public spec endpoint serves it.
- Error responses use `httpx.Error(w, status, msg)`; success uses `httpx.JSON`. Frontend uses pnpm.
- Imported/attached product `openapiSpec` field name is `openapiSpec` (JSON) end to end.

---

## Task A1: Store the spec on create/update (+ migration, model, validation)

**Files:**
- Create: `internal/db/migrations/0009_openapi_spec.sql`
- Modify: `internal/admin/product.go` (add `OpenAPISpec` field)
- Modify: `internal/admin/repo.go` (Create + Update SQL)
- Modify: `internal/admin/handler.go` (`decodeProduct` validation)
- Test: `internal/admin/handler_test.go`

**Interfaces:**
- Consumes: `parseSpec([]byte) (Product, error)` and `ErrBadSpec` from `internal/admin/import.go`.
- Produces: `admin.Product.OpenAPISpec string` (json `openapiSpec,omitempty`); create persists it; update persists it only when non-empty.

- [ ] **Step 1: Write the migration**

Create `internal/db/migrations/0009_openapi_spec.sql`:
```sql
-- Store the raw OpenAPI/Swagger spec (JSON or YAML) for a product so the
-- catalog can render interactive docs + Try-it. Empty = no docs.
ALTER TABLE api_products ADD COLUMN IF NOT EXISTS openapi_spec TEXT NOT NULL DEFAULT '';
```

- [ ] **Step 2: Write the failing handler tests**

Add to `internal/admin/handler_test.go`:
```go
func TestCreateProductStoresSpec(t *testing.T) {
	svc := &fakeService{products: map[int64]Product{}}
	h := NewHandler(svc, true)
	spec := `{"openapi":"3.0.0","info":{"title":"X","version":"1.0.0"}}`
	body, _ := json.Marshal(Product{
		Name: "X", Slug: "x", Category: "C", ContextPath: "/x", OpenAPISpec: spec,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/products", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var got Product
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.OpenAPISpec != spec {
		t.Errorf("spec not passed through: %q", got.OpenAPISpec)
	}
}

func TestCreateProductRejectsBrokenSpec(t *testing.T) {
	svc := &fakeService{products: map[int64]Product{}}
	h := NewHandler(svc, true)
	body, _ := json.Marshal(Product{
		Name: "X", Slug: "x", Category: "C", ContextPath: "/x", OpenAPISpec: "this is not a spec",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/products", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/admin/ -run 'TestCreateProductStoresSpec|TestCreateProductRejectsBrokenSpec' -v`
Expected: FAIL — `Product` has no field `OpenAPISpec` (compile error), then once the field exists, the broken-spec test fails with 201.

- [ ] **Step 4: Add the model field**

In `internal/admin/product.go`, add to the `Product` struct (after `Published`):
```go
	// OpenAPISpec is the raw OpenAPI/Swagger document (JSON or YAML) backing the
	// product's docs + Try-it. Empty = no docs. omitempty so list/update
	// responses (which don't re-select it) don't echo an empty string.
	OpenAPISpec string `json:"openapiSpec,omitempty"`
```

- [ ] **Step 5: Persist it in Create and Update**

In `internal/admin/repo.go`, change `Create`'s SQL + args:
```go
		`INSERT INTO api_products(name, slug, category, version, context_path, description, tags, icon, upstream_url, published, openapi_spec)
		 VALUES($1,$2,$3,COALESCE(NULLIF($4,''),'1.0.0'),$5,$6,$7,$8,$9,$10,$11)
		 RETURNING `+productCols,
		p.Name, p.Slug, p.Category, p.Version, p.ContextPath, p.Description, p.Tags, p.Icon, p.UpstreamURL, p.Published, p.OpenAPISpec))
```
Change `Update`'s SQL + args (preserve on empty):
```go
		`UPDATE api_products SET name=$2, slug=$3, category=$4, version=COALESCE(NULLIF($5,''),'1.0.0'),
		   context_path=$6, description=$7, tags=$8, icon=$9, upstream_url=$10, published=$11,
		   openapi_spec=COALESCE(NULLIF($12,''), openapi_spec)
		 WHERE id=$1
		 RETURNING `+productCols,
		p.ID, p.Name, p.Slug, p.Category, p.Version, p.ContextPath, p.Description, p.Tags, p.Icon, p.UpstreamURL, p.Published, p.OpenAPISpec))
```
(`productCols` is unchanged — the spec is intentionally not in the RETURNING set, so list/create/update responses stay lean and `scanProduct` is untouched.)

- [ ] **Step 6: Validate the spec in decodeProduct**

In `internal/admin/handler.go`, in `decodeProduct`, after the existing `p.validate(h.allowPrivate)` block and before `return p, true`:
```go
	if p.OpenAPISpec != "" {
		if _, err := parseSpec([]byte(p.OpenAPISpec)); err != nil {
			httpx.Error(w, http.StatusBadRequest, "openapiSpec is not a valid OpenAPI 3.x / Swagger 2.0 document")
			return Product{}, false
		}
	}
```

- [ ] **Step 7: Run the tests to verify they pass**

Run: `go test ./internal/admin/ -run 'TestCreateProductStoresSpec|TestCreateProductRejectsBrokenSpec' -v && go vet ./internal/admin/ && go test ./internal/admin/`
Expected: PASS (both new tests + full package).

- [ ] **Step 8: Commit**

```bash
git add internal/db/migrations/0009_openapi_spec.sql internal/admin/product.go internal/admin/repo.go internal/admin/handler.go internal/admin/handler_test.go
git commit -m "feat(admin): store openapi_spec on products (create/update + validation)"
```

---

## Task A2: Public spec endpoint `GET /api/products/{slug}/spec`

**Files:**
- Modify: `internal/catalog/product.go` (errors/helpers as needed)
- Modify: `internal/catalog/repo.go` (`GetSpecBySlug`)
- Modify: `internal/catalog/handler.go` (route + handler + content-type helper)
- Test: `internal/catalog/handler_test.go`, `internal/catalog/repo_test.go`

**Interfaces:**
- Consumes: existing `catalog.ErrNotFound`.
- Produces: `Lister.GetSpecBySlug(ctx, slug string) (string, error)` (returns the raw spec for a published product; `ErrNotFound` when missing/unpublished/empty). Route `GET /api/products/{slug}/spec`.

- [ ] **Step 1: Write the failing handler test**

In `internal/catalog/handler_test.go`, first extend the fake Lister used by existing tests to add:
```go
func (f *fakeLister) GetSpecBySlug(_ context.Context, slug string) (string, error) {
	s, ok := f.specs[slug]
	if !ok {
		return "", ErrNotFound
	}
	return s, nil
}
```
(Add a `specs map[string]string` field to the fake's struct; initialise it in the existing helper that builds the fake. If the fake is a struct literal per-test, add `specs: map[string]string{...}` there.)

Then add the test:
```go
func TestGetSpecBySlug(t *testing.T) {
	f := &fakeLister{specs: map[string]string{"orders": `{"openapi":"3.0.0"}`}}
	h := NewHandler(f)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/products/orders/spec", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != `{"openapi":"3.0.0"}` {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type=%q", ct)
	}

	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/api/products/missing/spec", nil))
	if rec2.Code != http.StatusNotFound {
		t.Fatalf("missing slug: status=%d", rec2.Code)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/catalog/ -run TestGetSpecBySlug -v`
Expected: FAIL — `GetSpecBySlug` not in the interface / route 404 (chi has no `/spec` route yet).

- [ ] **Step 3: Add the repo query**

In `internal/catalog/repo.go`:
```go
// GetSpecBySlug returns the raw OpenAPI spec for a published product, or
// ErrNotFound when the product is missing, unpublished, or has no spec.
func (r *Repo) GetSpecBySlug(ctx context.Context, slug string) (string, error) {
	var spec string
	err := r.pool.QueryRow(ctx,
		`SELECT openapi_spec FROM api_products WHERE slug=$1 AND published=true`, slug).Scan(&spec)
	if errors.Is(err, pgx.ErrNoRows) || spec == "" {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return spec, nil
}
```
Ensure `errors` and `github.com/jackc/pgx/v5` are imported in repo.go (add if missing).

- [ ] **Step 4: Add the interface method, route, handler, content-type helper**

In `internal/catalog/handler.go`, add to the `Lister` interface:
```go
	GetSpecBySlug(ctx context.Context, slug string) (string, error)
```
Register the route in `NewHandler` (after the `/{slug}` route):
```go
	h.router.Get("/api/products/{slug}/spec", h.getSpec)
```
Add the handler + helper:
```go
func (h *Handler) getSpec(w http.ResponseWriter, r *http.Request) {
	spec, err := h.repo.GetSpecBySlug(r.Context(), chi.URLParam(r, "slug"))
	if err == ErrNotFound {
		httpx.Error(w, http.StatusNotFound, "spec not found")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to load spec")
		return
	}
	w.Header().Set("Content-Type", specContentType(spec))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(spec))
}

// specContentType guesses JSON vs YAML from the first non-space byte.
func specContentType(s string) string {
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			continue
		}
		if r == '{' || r == '[' {
			return "application/json"
		}
		break
	}
	return "application/yaml"
}
```

- [ ] **Step 5: Run the handler test to verify it passes**

Run: `go test ./internal/catalog/ -run TestGetSpecBySlug -v`
Expected: PASS.

- [ ] **Step 6: Add a DB-backed repo test**

In `internal/catalog/repo_test.go` (follow the file's existing setup helper that returns a `*Repo` + seeds a product; mirror an existing test's seeding style). Add:
```go
func TestGetSpecBySlugPublishedOnly(t *testing.T) {
	ctx, repo, pool := testCatalogRepo(t) // use the file's existing setup helper name
	_, _ = pool.Exec(ctx,
		`INSERT INTO api_products(name,slug,category,context_path,published,openapi_spec)
		 VALUES('SpecPub','spec-pub','C','/sp',true,'{"openapi":"3.0.0"}'),
		        ('SpecPriv','spec-priv','C','/spr',false,'{"openapi":"3.0.0"}')`)

	if s, err := repo.GetSpecBySlug(ctx, "spec-pub"); err != nil || s == "" {
		t.Fatalf("published: %v %q", err, s)
	}
	if _, err := repo.GetSpecBySlug(ctx, "spec-priv"); err != ErrNotFound {
		t.Fatalf("unpublished: want ErrNotFound, got %v", err)
	}
	if _, err := repo.GetSpecBySlug(ctx, "nope"); err != ErrNotFound {
		t.Fatalf("missing: want ErrNotFound, got %v", err)
	}
}
```
NOTE: match the actual setup-helper name and signature already used in `repo_test.go` (it may return only `(ctx, repo)` and expose the pool differently — adapt the seeding to that file's convention; if the helper doesn't expose a pool, seed via a fresh `db.Connect` like `internal/applications/repo_test.go` does).

- [ ] **Step 7: Run catalog tests**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/catalog/ && go vet ./internal/catalog/`
Expected: PASS (repo test runs against the dev DB; skips if absent).

- [ ] **Step 8: Commit**

```bash
git add internal/catalog/repo.go internal/catalog/handler.go internal/catalog/handler_test.go internal/catalog/repo_test.go
git commit -m "feat(catalog): public GET /api/products/{slug}/spec"
```

---

## Task A3: Import carries the spec

**Files:**
- Modify: `internal/admin/handler.go` (`importSpec`)
- Test: `internal/admin/import_handler_test.go`

**Interfaces:**
- Consumes: `parseSpec`, the `Product.OpenAPISpec` field (Task A1).
- Produces: the import response Product now has `openapiSpec` set to the raw spec text.

- [ ] **Step 1: Write the failing test**

Add to `internal/admin/import_handler_test.go`:
```go
func TestImport_ReturnsRawSpec(t *testing.T) {
	h := newImportHandler(false)
	spec := `{"openapi":"3.0.0","info":{"title":"Imported API","version":"3.0.0"}}`
	body := `{"spec": ` + strconvQuote(spec) + `}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/products/import", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var p Product
	_ = json.Unmarshal(rec.Body.Bytes(), &p)
	if p.OpenAPISpec != spec {
		t.Errorf("openapiSpec = %q, want raw spec", p.OpenAPISpec)
	}
}

// strconvQuote JSON-quotes a string for embedding in the request body.
func strconvQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/admin/ -run TestImport_ReturnsRawSpec -v`
Expected: FAIL — `openapiSpec` empty.

- [ ] **Step 3: Set the raw spec on the draft**

In `internal/admin/handler.go`, in `importSpec`, after `draft, err := parseSpec(data)` succeeds and before `httpx.JSON(w, http.StatusOK, draft)`:
```go
	draft.OpenAPISpec = string(data)
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/admin/ -run TestImport_ReturnsRawSpec -v && go test ./internal/admin/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/admin/handler.go internal/admin/import_handler_test.go
git commit -m "feat(admin): import returns the raw spec so create can persist it"
```

---

## Task A4: Frontend — persist imported spec + manual attach on the product form

**Files:**
- Modify: `web/src/api/types.ts` (`AdminProduct.openapiSpec`)
- Modify: `web/src/pages/admin/ProductsPage.tsx` (FormState, onImported, submit, Composer field)
- Test: `web/src/pages/admin/ProductsPage.test.tsx`

**Interfaces:**
- Consumes: import returns `openapiSpec` (A3); create/update accept `openapiSpec` (A1).
- Produces: imported specs are persisted on create; admins can paste/upload a spec; empty field on edit leaves the stored spec unchanged.

- [ ] **Step 1: Write the failing test**

Add to `web/src/pages/admin/ProductsPage.test.tsx` (inside the `describe('ProductsPage', …)` block):
```tsx
  it('persists the imported spec in the create payload', async () => {
    const create = vi.spyOn(api, 'adminCreateProduct').mockResolvedValue({} as AdminProduct)
    vi.spyOn(api, 'adminImportProduct').mockResolvedValue({
      name: 'Imported API', slug: 'imported', category: 'Finance', version: '2.5.0',
      contextPath: '/v2', description: '', tags: [], icon: '', upstreamUrl: '', published: false,
      openapiSpec: '{"openapi":"3.0.0","info":{"title":"Imported API","version":"2.5.0"}}',
    })
    renderPage()
    await screen.findByText('CurrencyConverterAPI')
    await userEvent.click(screen.getByRole('button', { name: /Importer une API/i }))
    await userEvent.click(screen.getByRole('tab', { name: /URL/i }))
    await userEvent.type(screen.getByPlaceholderText(/https/i), 'https://x/openapi.json')
    await userEvent.click(screen.getByRole('button', { name: /^Importer$/i }))
    await screen.findByText('Créer un produit')
    await userEvent.click(screen.getByRole('button', { name: /Créer le produit/i }))
    await waitFor(() => expect(create).toHaveBeenCalled())
    const payload = create.mock.calls[0][1]
    expect(payload.openapiSpec).toContain('"openapi":"3.0.0"')
  })
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd web && pnpm exec vitest run src/pages/admin/ProductsPage.test.tsx`
Expected: FAIL — `payload.openapiSpec` is undefined.

- [ ] **Step 3: Add the type field**

In `web/src/api/types.ts`, add to `AdminProduct` (after `published`):
```ts
  openapiSpec?: string
```

- [ ] **Step 4: Thread it through FormState, onImported, submit, and the Composer**

In `web/src/pages/admin/ProductsPage.tsx`:

Add `openapiSpec` to `FormState` and `EMPTY`:
```tsx
interface FormState {
  name: string; slug: string; category: string; contextPath: string
  upstreamUrl: string; version: string; published: boolean; openapiSpec: string
}
const EMPTY: FormState = { name: '', slug: '', category: '', contextPath: '', upstreamUrl: '', version: '1.0.0', published: true, openapiSpec: '' }
```
In `openCreate`/`openEdit`, they already set the form from `EMPTY`/the product — add `openapiSpec`:
- `openEdit` sets the form fields; add `openapiSpec: ''` (empty = keep stored spec on update).
In `onImported`, include the spec:
```tsx
      version: draft.version, published: false, openapiSpec: draft.openapiSpec ?? '',
```
In `submit`, add to `payload`:
```tsx
      openapiSpec: form.openapiSpec,
```
Add the field inside the Composer's `<div className="grid2">` (a full-width row after Version):
```tsx
          <div className="field" style={{ gridColumn: '1 / -1' }}>
            <label htmlFor="f-spec">Spécification OpenAPI <span className="opt">optionnel</span></label>
            <input id="f-spec-file" type="file" accept=".json,.yaml,.yml"
              onChange={async e => { const f = e.target.files?.[0]; if (f) set('openapiSpec', await f.text()) }} />
            <textarea id="f-spec" className="ipt mono" rows={4} placeholder="Collez une spec OpenAPI 3.x / Swagger 2.0…"
              value={form.openapiSpec} onChange={e => set('openapiSpec', e.target.value)} />
            <div className="help">{editing ? 'Laissez vide pour conserver la spécification existante.' : 'Alimente la documentation et le « Essayer » du produit.'}</div>
          </div>
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `cd web && pnpm exec vitest run src/pages/admin/ProductsPage.test.tsx`
Expected: PASS.

- [ ] **Step 6: Full frontend gate**

Run: `cd web && pnpm exec vitest run --exclude 'e2e/**' && pnpm exec tsc --noEmit && pnpm build`
Expected: all green.

- [ ] **Step 7: Commit**

```bash
git add web/src/api/types.ts web/src/pages/admin/ProductsPage.tsx web/src/pages/admin/ProductsPage.test.tsx
git commit -m "feat(web): persist imported spec + attach a spec on the product form"
```

---

## Task A5: Live verification

- [ ] **Step 1: Apply the migration + restart the portal**

Run (dev stack up): restart the portal so migration 0009 runs.
```bash
PORTAL_ENV=dev UPSTREAM_ALLOW_PRIVATE=1 PORTAL_ADDR=:8090 go run ./cmd/portal
```
Expected: starts clean (migrations apply on boot).

- [ ] **Step 2: Import a spec → create → fetch the public spec**

As admin: import a public spec (URL tab), create the product, then:
```bash
curl -s http://localhost:8090/api/products/<new-slug>/spec | head -c 200
```
Expected: the raw spec bytes (200). For a product without a spec or unpublished: `404`.

- [ ] **Step 3: Confirm edit-preserves-spec**

Edit the product (leave the spec field empty), save, re-fetch `/spec`.
Expected: the spec is still present (not wiped).

---

## Self-Review notes

- **Spec coverage (Plan A scope):** migration + column ✅ (A1); store on create/update with empty-preserves-on-update ✅ (A1); non-empty spec validated via `parseSpec` → 400 ✅ (A1); public `GET /api/products/{slug}/spec` 200/404 + JSON/YAML content-type ✅ (A2); import carries the raw spec ✅ (A3); frontend persists imported spec + manual paste/upload, edit leaves spec unchanged ✅ (A4); list does not return the spec (RETURNING uses `productCols`, list select unchanged) ✅.
- **Type consistency:** `OpenAPISpec`/`openapiSpec` used consistently across model, repo, import, types, form, and the spec endpoint.
- **Note for implementer:** `internal/admin` has no DB-backed repo test (it uses `fakeService`), so A1's persistence is asserted at the handler/wiring level + the live check in A5; the catalog repo (A2) does have DB tests (`repo_test.go`) — match its existing setup helper exactly.
