# Import API products from an OpenAPI / Swagger spec — Design

**Date:** 2026-06-25
**Status:** Implemented (2026-06-25, branch `feat/openapi-import`)
**Surface:** Admin → Products (`internal/admin`, `web/src/pages/admin/ProductsPage.tsx`)

## Problem

Admins can only create API products by filling the Composer form field-by-field. For
APIs that already have an OpenAPI 3.x or Swagger 2.0 description, that is redundant
data entry. We want admins to import a spec and have the product fields pre-filled.

## Scope (locked decisions)

- **Source:** OpenAPI 3.x **and** Swagger 2.0 specs only (no APISIX-route import, no
  bulk CSV, no Postman — explicitly out of scope for this iteration).
- **Input:** two methods — **file upload** (`.json/.yaml/.yml`) and **URL fetch**.
- **Flow:** parse → **pre-fill the existing Composer form** → admin reviews/edits →
  admin clicks Create (existing `POST /api/admin/products`). No one-shot create, no
  separate preview screen.
- **Persistence:** the spec is **discarded after pre-fill**. Nothing about the raw
  spec is stored. Re-importing later starts fresh. (Storing the spec to render
  catalog API docs is a possible *future* feature, deliberately not built now.)

## Flow

1. Admin opens **Products** and clicks a new **"Importer une API"** action (next to
   "Nouveau produit").
2. An import dialog offers two tabs: **Fichier** (file picker) and **URL** (text input).
3. On submit, the frontend calls `POST /api/admin/products/import`:
   - File tab → frontend reads the file text via `FileReader`, sends `{ "spec": "<text>" }`.
   - URL tab → sends `{ "url": "https://…" }`; the **backend** fetches it.
4. Backend parses the spec, extracts metadata into a **draft `Product`** (not persisted),
   returns it as JSON.
5. Frontend opens the existing **Composer**, pre-filled from the draft, with a hint
   "Importé — vérifiez les champs". `published` starts **false** (draft).
6. Admin reviews/edits and clicks **Create**. This is the existing create path, so all
   current validation applies (host:port + SSRF upstream check, context-path rules,
   slug/name requirements).

## Backend

### New endpoint: `POST /api/admin/products/import`

- Added to `internal/admin/Handler`'s chi router, mounted behind the existing
  `requireAdmin` middleware (no new wiring in `internal/server/server.go` beyond the
  route already being under `/api/admin/products/`).
- Request body is exactly one of:
  - `{ "url": "<string>" }` — backend fetches the spec.
  - `{ "spec": "<string>" }` — raw JSON or YAML text (the uploaded file's contents).
- Responses:
  - `200 OK` with a draft `Product` JSON (same shape the form/`AdminProduct` consumes).
  - `422 Unprocessable Entity` with a clear message when the spec cannot be parsed or
    has no `info.title`.
  - `400` for a malformed request body (neither/both fields).
- **Writes nothing to the database.**

### URL fetch — SSRF guard

The URL path reuses the existing SSRF protection pattern from `internal/admin/product.go`
(`isPrivateIP`, DNS resolution + private-range rejection):

- Only `http`/`https` schemes accepted.
- Host resolved; rejected if **any** resolved address is loopback / link-local /
  private / unspecified — **unless** `allowPrivate` is set (the dev stack flag already
  threaded into `admin.NewHandler`), so docker-internal hosts work in dev.
- 5s timeout, response body capped (~2 MB), redirects not blindly followed to private
  ranges (re-validate on redirect or disable redirects).

### Parser — new file `internal/admin/import.go`

- A minimal struct covering both spec versions (decode the fields we map, ignore the rest):
  - OpenAPI 3.x: `openapi`, `info{title,version,description}`, `servers[]{url}`, `tags[]{name}`.
  - Swagger 2.0: `swagger`, `info{...}`, `host`, `basePath`, `schemes[]`, `tags[]{name}`.
- YAML-or-JSON input handled uniformly via `sigs.k8s.io/yaml` (new dependency; converts
  YAML→JSON then decodes with the same JSON-tagged struct). JSON is valid YAML's subset,
  so one path handles both.
- A pure mapping function `draftFromSpec(parsed) (Product, error)` so it is unit-testable
  without HTTP.

### Field mapping (spec → draft Product)

| Product field | Source | Fallback |
|---|---|---|
| `name` | `info.title` | **required** — `422` if empty |
| `version` | `info.version` | `1.0.0` |
| `description` | `info.description` | `""` |
| `slug` | `slugify(info.title)` | — |
| `contextPath` | path component of `servers[0].url` (3.x) or `basePath` (2.0) | `/<slug>` |
| `upstreamUrl` | `host:port` parsed from `servers[0].url`, or `host` + scheme→port (2.0) | `""` |
| `category` | first top-level `tags[].name` | `""` |
| `tags` | top-level `tags[].name` (all) | `[]` |
| `published` | always `false` | — |

Notes:
- `upstreamUrl` is a **seed only** — the spec's advertised server may differ from the
  real backend. The admin confirms it; create-time host:port + SSRF validation is the
  real gate. If the server URL has no explicit port, derive from scheme (https→443,
  http→80) or leave blank.
- `contextPath` is normalised to start with `/`; if empty, falls back to `/<slug>`.

## Frontend

- **`ProductsPage.tsx`**: add an **"Importer une API"** button next to "Nouveau produit".
- New **`ImportModal`** component: two tabs (Fichier / URL).
  - Fichier: `<input type="file" accept=".json,.yaml,.yml">`, read text via `FileReader`.
  - URL: text input.
  - Submit → `adminImportProduct(token, {spec}|{url})`.
- New API client fn `adminImportProduct` in `web/src/api/client.ts` returning a draft
  `AdminProduct`.
- On success: open the existing Composer pre-filled from the draft (reuse the
  `openEdit`-style state path, but treat it as a create — no `editing.id`), show a hint
  "Importé — vérifiez les champs".
- On error (422 / SSRF reject / parse fail): surface the backend message via the
  existing `Toast`.

## Testing

- **Go**
  - Table tests for `draftFromSpec` / parser: OpenAPI 3.x, Swagger 2.0, YAML input,
    JSON input, missing-title → error, server-URL → upstream extraction (with and
    without explicit port), `basePath` → contextPath.
  - SSRF: URL fetch rejects private IPs and non-http(s) schemes; honours `allowPrivate`.
  - Handler test for `POST /api/admin/products/import`: 200 happy path (spec body),
    422 bad spec, 400 bad request body. Confirm nothing is persisted.
- **Frontend (vitest)**
  - `ImportModal`: file submit and URL submit call the client; error message displayed.
  - Successful import opens the pre-filled Composer with mapped values.

## Out of scope (deliberately deferred)

- Storing the raw spec / rendering interactive API docs (Swagger UI) in the catalog.
- Importing from existing APISIX routes, bulk CSV/JSON, or Postman collections.
- Importing path-level operations as anything beyond metadata (we map info/servers/tags
  only; individual operations are not turned into routes).
