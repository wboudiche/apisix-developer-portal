# Interactive API Docs + Try-it — Design

**Date:** 2026-06-26
**Status:** Approved, ready for planning
**Surface:** Catalog (new product detail page), admin product form, gateway proxy.

## Problem

The portal lists APIs but offers no documentation and no way to exercise an
endpoint. Peer portals (WSO2, Kong, Gravitee, Apigee) render interactive
OpenAPI docs with a "Try-it" console. The portal already *imports* OpenAPI specs
but discards them after pre-filling the create form. This feature stores those
specs and turns them into browsable docs plus a live, gateway-routed Try-it.

## Locked decisions (from brainstorming)

- **V1 scope:** rendered docs **and** full Try-it (live calls), not docs-only.
- **Renderer:** `@scalar/api-reference` (React), themed to the Atlas crimson tokens.
- **Try-it routing:** proxied **through the portal backend** → APISIX gateway
  (same-origin, no CORS config on APISIX, key injected server-side).
- **Access:** docs are **public**; Try-it requires login **+ an approved
  subscription** to the product, and uses that subscription's real app key.
  When the user is subscribed via several apps, they pick which app's key.
- **Out of scope (deferred):** ephemeral "test keys" for non-subscribers
  (WSO2-style GET TEST KEY); GraphQL/AsyncAPI/SOAP docs; SDK generation;
  per-operation rate display.

## Decomposition

One feature, three implementation plans, each independently shippable:

- **Plan A — Spec storage:** persist an OpenAPI spec on a product and serve it.
- **Plan B — Docs page:** new product detail page rendering Scalar (read-only).
- **Plan C — Try-it:** the gateway proxy + key/subscription resolution + Scalar wiring.

Build order A → B → C (C depends on B's page; B depends on A's spec endpoint).

---

## Plan A — Spec storage

- **Migration:** add `api_products.openapi_spec TEXT NOT NULL DEFAULT ''`. Holds
  the raw spec text (JSON or YAML — Scalar renders both; no normalization on store).
- **Import carries the spec:** today `POST /api/admin/products/import` returns a
  parsed draft and discards the raw bytes. Extend its response with the raw spec
  text (e.g. `{ draft: Product, spec: string }`, or add `spec` onto the draft).
  The frontend `ImportModal`/`ProductsPage` keeps the spec and includes it in the
  subsequent `POST /api/admin/products` create payload, so importing now keeps
  the spec instead of discarding it.
- **Manual attach:** the admin product Composer gains an optional **"Spécification
  OpenAPI"** field — paste a spec or upload a `.json/.yaml/.yml` file (read via
  `FileReader`). Persisted via the existing create/update product endpoints
  (`openapiSpec` field on the admin `Product`).
- **Public read endpoint:** `GET /api/products/{slug}/spec`:
  - `200` with the raw spec body (`Content-Type` reflecting JSON/YAML or
    `text/plain`) when the product is published and has a non-empty spec.
  - `404` when the product is unpublished, missing, or has no spec.
- **Validation:** on store, do a light parse check (reuse the import parser's
  `parseSpec`) so an obviously-broken spec is rejected at create/update time with
  a `400`. An empty spec is allowed (product simply has no docs).

## Plan B — Docs page

- **Route:** `/apis/:slug` (new). Catalog cards link to it; the catalog's
  "S'abonner" still works, and the detail page also offers it.
- **Page is the product's home** (none exists today): header with name, category
  (with its color/icon from `apiIcons`), version, rating stars, tags, description,
  and a **"S'abonner"** action reusing `SubscribeModal` (login-gated as today).
- **Docs:** render `@scalar/api-reference` fed from `GET /api/products/{slug}/spec`,
  themed to the Atlas light/dark tokens. Read-only docs require no login.
- **No-spec state:** product header + a "Documentation bientôt disponible"
  placeholder (no Scalar mount).
- Loading and fetch-error states surfaced inline (reuse existing patterns).

## Plan C — Try-it

- **Proxy endpoint:** `POST /api/try/{slug}` (behind `RequireAuth`). Request body:
  `{ method: string, path: string, query?: object, headers?: object, body?: string, appId?: number }`.
  Behaviour:
  1. Require login (JWT) — else `401`.
  2. Require an **approved** subscription by this user to `{slug}` — else `403`
     (UI shows "S'abonner pour essayer").
  3. Resolve the API key: the given `appId`'s credential (must belong to the user
     and be subscribed+approved to this product); if `appId` omitted and exactly
     one qualifying app exists, use it; if several and none given, `409` with the
     candidate apps so the UI prompts a pick.
  4. Forward to the gateway: `{APISIX_GATEWAY_URL}{context_path}{path}` with the
     method, query, passed-through headers, body, and the `apikey` header injected.
     Return `{ status, headers, body, durationMs }`.
- **Security:**
  - The proxy targets **only** the single configured gateway base — the path is
    derived from the product's `context_path`, never from client-supplied host —
    so there is no SSRF surface.
  - The key is injected **server-side** and never sent to the browser.
  - A request-body cap and a short timeout apply (mirroring the import fetcher).
  - Hop-by-hop and sensitive headers (Host, Authorization to the portal, Cookie)
    are stripped before forwarding; only safe request headers pass through.
- **Config:** new `APISIX_GATEWAY_URL` (default `http://localhost:9080`; the
  docker gateway data-plane). Distinct from the existing `APISIX_ADMIN_URL`.
- **Scalar wiring:** intercept Scalar's outgoing request (its request hook /
  custom client) so "Essayer" calls `POST /api/try/{slug}` with the
  method/path/query/headers/body and the selected `appId`, then render the
  returned response in Scalar's response panel. The app picker (a small select)
  shows only when the user has >1 qualifying subscribed app.

## Testing

- **Go**
  - Plan A: store + serve spec (published vs unpublished vs no-spec → 200/404);
    broken spec on create → 400; import response includes the raw spec.
  - Plan C: `POST /api/try/{slug}` — not-logged-in → 401; logged-in but not
    approved-subscribed → 403; multiple apps + no appId → 409 with candidates;
    happy path injects the key and forwards to a **fake gateway**, returning its
    status/body/timing; only the configured gateway host is ever targeted.
- **Frontend (vitest)**
  - Detail page: renders header from the product; mounts Scalar with the fetched
    spec; no-spec placeholder; "S'abonner" opens the modal; Try-it states
    (logged-out, not-subscribed, subscribed→calls `/api/try`, app picker on >1).
  - Import carries the spec into the create payload.
- **E2E (Playwright, one flow):** admin imports a spec → creates the product →
  open `/apis/:slug` → docs render → subscribe (approve) → Try-it an endpoint and
  get a response (against the `echo` upstream in the dev stack).

## Notes / risks

- Scalar bundle size: lazy-load the docs component so the catalog/admin bundles
  aren't affected.
- The dev stack must expose the gateway data-plane to the portal process
  (`APISIX_GATEWAY_URL`); `docker-compose` already publishes `9080`.
- Try-it consumes the subscriber's real quota (it is a real metered call) — this
  is intended and matches WSO2/Gravitee behaviour.
