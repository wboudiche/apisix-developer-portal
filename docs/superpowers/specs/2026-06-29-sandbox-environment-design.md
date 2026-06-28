# Real Sandbox Environment — Design

**Date:** 2026-06-29
**Status:** Approved, ready for planning
**Surface:** new dedicated sandbox APISIX gateway; `internal/subscriptions` (provisioning), `internal/admin` (product sandbox upstream), `internal/tryit` (sandbox proxy), `internal/config`, `docker-compose.yml`; frontend Credentials tab + product detail Try-it + admin product Composer.

## Problem

The portal has one gateway and one upstream per product, gated behind admin
approval. Developers have no safe place to exercise an API against a
non-production backend before (or alongside) using production. WSO2's developer
portal solves this with per-application **Sandbox keys** that route to a
separate **sandbox endpoint**. This feature adds a real sandbox environment:
approved subscribers get a distinct sandbox key that reaches a product's
sandbox backend through a **dedicated sandbox gateway**, fully isolated from
production traffic.

## Locked decisions (from brainstorming)

- **Audience:** approved subscribers only. Sandbox is a non-production test
  backend, toggled against an existing (approved) subscription — not a
  pre-subscription evaluation surface.
- **Isolation:** a **dedicated sandbox APISIX gateway** (a second data-plane
  instance), not a second route on the production gateway. Sandbox traffic can
  never touch production.
- **Credentials:** a **separate sandbox key** per app (a true sandbox key,
  distinct from the production key) — matches WSO2's separate production/sandbox
  application keys.
- **Rate limit:** sandbox reuses the **subscription's plan limit** (same
  count/window) — no separate sandbox limit dimension.
- **Provisioning:** **opt-in, app-level** — the developer explicitly enables
  sandbox for an app, which generates the sandbox key on demand (matches WSO2,
  where sandbox keys are per-application and generated when wanted). One sandbox
  key per app works across every sandbox-enabled product the app is subscribed
  to.

## Architecture overview

A product may declare a **sandbox upstream** (a non-prod backend URL). A
subscriber app enables sandbox **once at the app level**: this generates a
distinct sandbox key, provisions an `app_<id>` consumer on the **sandbox
gateway**, and grants that consumer on the sandbox route of every
sandbox-enabled product the app actively subscribes to. The production gateway
and its provisioning are unchanged.

**Core invariant (the sandbox-route whitelist rule):** a product's sandbox
route on the sandbox gateway whitelists exactly the apps that are **active
subscribers of that product** AND **have a sandbox key**. Every lifecycle hook
below maintains this invariant.

## Infrastructure

`docker-compose.yml` gains an **`apisix-sandbox`** service (same
`apache/apisix:3.9.1-debian` image as production), **sharing the existing etcd**
with a distinct `deployment.etcd.prefix: /apisix-sandbox` so the two control
planes never collide (one etcd cluster, full config isolation between the two
APISIX instances).

- Sandbox host ports: data-plane **`:9081`**, admin **`:19280`** (production
  stays `:9080` / `:19180`).
- Dev convenience: a sandbox backend for products to point at in dev — reuse
  the existing `echo` service (a product's sandbox upstream can be `echo:8080`
  in dev), or add a second echo. No new required backend.

## Config — `internal/config`

New fields on `Config`:

- `APISIXSandboxAdminURL` — `APISIX_SANDBOX_ADMIN_URL`, default
  `http://localhost:19280`.
- `APISIXSandboxGatewayURL` — `APISIX_SANDBOX_GATEWAY_URL`, default
  `http://localhost:9081`.
- `APISIXSandboxAdminKey` — `APISIX_SANDBOX_ADMIN_KEY`, **defaults to
  `APISIXAdminKey`** (so dev needs no extra secret; overridable for prod).

**Sandbox is active only when both sandbox URLs are configured.** When unset,
the portal runs production-only: the sandbox gateway client is nil, all sandbox
provisioning hooks are no-ops, the sandbox endpoints return `409`/are inert, and
the frontend hides sandbox affordances. The existing dev-secrets guard
(`config.Validate`) is unchanged — the sandbox admin key inherits the production
key's dev-default, already covered by that guard.

## Data model — migration `0011_sandbox.sql`

- `api_products.sandbox_upstream_url TEXT NOT NULL DEFAULT ''` — the product's
  sandbox backend. Validated with the same scheme-aware `admin.ValidUpstream`
  used for `upstream_url`. A product is **sandbox-enabled** when this is
  non-empty.
- `credentials.sandbox_api_key TEXT NOT NULL DEFAULT ''` — the app's sandbox
  key, **encrypted at rest** via the existing `cipher` (exactly like `api_key`);
  `''` means sandbox is not enabled for the app. The sandbox consumer reuses the
  app's existing `consumer_username` (`app_<id>`) on the sandbox gateway.

No new per-subscription column. "App has sandbox enabled" ⇔
`credentials.sandbox_api_key != ''`. A product's sandbox-route whitelist is
derived (active subscribers ∩ apps with a sandbox key).

## Backend — provisioning

### Second gateway wiring

`cmd/portal/main.go` builds a second admin client
`apisix.NewClient(cfg.APISIXSandboxAdminURL, cfg.APISIXSandboxAdminKey)` and
passes it into `subscriptions.NewService` as `sandboxGW apisix.Gateway` (nil
when sandbox isn't configured). The existing `apisix.Gateway` interface is
reused unchanged — sandbox is the same operations against a different gateway.
On the sandbox gateway, naming mirrors production: consumer `app_<id>`, route
`prod_<id>` (the separate gateway/etcd-prefix means no collision with the
production objects). The sandbox route exposes `context_path/*` → the product's
**sandbox** upstream.

### New repo query

`SandboxConsumersForProduct(ctx, productID) ([]Credential, error)` — active
subscribers of the product whose credential has `sandbox_api_key != ''`
(returns the sandbox consumer username + sandbox key). Mirrors the existing
`ConsumersForProduct`; used to (re)build a sandbox route's whitelist.

### Lifecycle hooks

All hooks guard on `sandboxGW != nil` and (where product-scoped) the product
having a non-empty `sandbox_upstream_url`. They are best-effort in the same
spirit as the existing reprovision paths (gateway errors are returned;
gateway-before-DB ordering is preserved where a key changes).

- **EnableSandbox (app-level)** — `Service.EnableSandbox(ctx, appID) (key, err)`:
  1. Require the app has ≥1 **active** subscription to a **sandbox-enabled**
     product, else `ErrNoSandboxEligibleSubscription` (→ 409). Require sandbox
     configured, else `ErrSandboxNotConfigured` (→ 409).
  2. If the app already has a sandbox key, return it idempotently (no
     re-generation); otherwise generate a new key.
  3. `EnsureConsumer` on the sandbox gateway (`app_<id>`, sandbox key, the
     **plan limit** of the app's most-recent active subscription — reuse
     `ActivePlanForApp`).
  4. Store the encrypted sandbox key (`UpdateSandboxKey`).
  5. For every sandbox-enabled product the app actively subscribes to,
     `EnsureRoute` on the sandbox gateway (`prod_<id>`, context path, sandbox
     upstream, whitelist = `SandboxConsumersForProduct`).
  6. Return the sandbox key (reveal-once to the caller).
- **Approve** (extends existing `Service.Approve`): after the production path,
  if the product is sandbox-enabled AND the app already has a sandbox key,
  rebuild that product's sandbox route (adds the app). Apps without a sandbox
  key are simply absent — no-op.
- **Reject / Unsubscribe** (extend existing): after the production reprovision,
  rebuild the product's sandbox route from the rule (drops the app). A consumer
  left with no sandbox routes is harmless and is left in place.
- **ReprovisionPlan** (extend existing): also `EnsureConsumer` on the sandbox
  gateway for affected apps that have a sandbox key, with the new limit — the
  sandbox limit always tracks the plan.
- **Product sandbox-upstream change** (admin update): rebuild that product's
  sandbox route against the new upstream (mirrors how production reprovisions on
  an `upstream_url` change). If the field is cleared, delete the product's
  sandbox route.
- **RotateSandboxKey** — `Service.RotateSandboxKey(ctx, appID) (key, err)`:
  gateway-before-DB like the production `RotateKey` — generate a new key →
  `EnsureConsumer` on the sandbox gateway (old sandbox key 401s instantly) →
  store the encrypted key → log a `sandbox_key_rotated` event. `409` if the app
  has no sandbox key (`ErrNoSandboxKey`).

New `events` kind: `sandbox_key_rotated` (and optionally `sandbox_enabled`).

## Backend — HTTP endpoints

Owner-scoped (the existing applications `authorize` helper), behind `requireAuth`:

- `POST /api/applications/{id}/sandbox/enable` → `200 {sandboxApiKey}`
  (reveal-once); `409` (`ErrNoSandboxEligibleSubscription` /
  `ErrSandboxNotConfigured`).
- `POST /api/applications/{id}/sandbox/rotate` → `200 {sandboxApiKey}`; `409`
  (`ErrNoSandboxKey`).
- `GET /api/applications/{id}` (existing detail view) gains:
  `sandboxEnabled bool` (app has a sandbox key), `sandboxGatewayUrl string` (the
  configured sandbox gateway origin — where the dev sends sandbox calls from
  their own client; `""` when sandbox is not configured), and per-subscription
  `sandboxAvailable bool` (the product has a sandbox upstream). The full sandbox
  base for a given product is `sandboxGatewayUrl + context_path`, surfaced on
  that product's detail page rather than as an app-level field.

Admin (behind `requireAdmin`): product create/update payload gains
`sandboxUpstreamUrl` (validated via `ValidUpstream`; invalid → `400`/`422`).
Changing it reprovisions the product's sandbox route.

### Try-it (sandbox proxy)

`internal/tryit` gains a sandbox variant. `tryit.NewHandler` receives the
sandbox gateway URL and access to the app's sandbox key.

- The context endpoint (`GET /api/try/{slug}/context`) reports
  `sandboxAvailable` (product has a sandbox upstream) and `sandboxEnabled` (the
  app has a sandbox key).
- `ANY /api/try/{slug}/{appId}/sandbox/*` proxies to the **sandbox gateway URL**
  and injects the **sandbox key** (production stays `…/{appId}/*` → production
  gateway + production key). Same SSRF-safety as production: the host is always
  the configured sandbox gateway; the path is `context_path` + chi wildcard,
  never a client-supplied host. Same header stripping (apikey never returned),
  timeout, and body caps as the production proxy.

## Frontend

- **CredentialsTab** (`web/src/pages/application/CredentialsTab.tsx`): a
  **Sandbox card** beside the Production card. No sandbox key yet → an "Activer
  le sandbox" button, shown only when the app subscribes to ≥1 sandbox-enabled
  product (otherwise hidden with a hint). Enabled → reveal-once sandbox key +
  "Régénérer" (rotate, reuses the production card's reveal/rotate pattern) + the
  sandbox gateway URL (where to send sandbox calls; each product's full sandbox
  path is shown on its detail page).
- **ProductDetailPage Try-it**: a **Production / Sandbox toggle**. The Sandbox
  option appears only when the product has a sandbox upstream AND the app has a
  sandbox key; selecting it switches Scalar's `server` between the production and
  sandbox proxy paths (`/api/try/{slug}/{appId}` vs
  `/api/try/{slug}/{appId}/sandbox`).
- **Admin Composer** (`web/src/pages/admin/ProductsPage.tsx`): a "Sandbox
  upstream" field next to the existing upstream field; flows through create and
  update.
- Client fns: `enableSandbox(token, appId)`, `rotateSandboxKey(token, appId)`;
  `AdminProduct.sandboxUpstreamUrl`, app-detail sandbox fields added to types.
  French copy; reuse Atlas tokens.

## Testing

### Backend (Go)

- **Provisioning (Fake gateways):** the Service test setup builds a second
  `apisix.Fake` for the sandbox gateway. Cover:
  - `EnableSandbox`: generates a key, provisions the sandbox consumer (plan
    limit) + sandbox routes for the app's sandbox-enabled active subs; idempotent
    second call returns the same key; `409` when no eligible subscription /
    sandbox not configured.
  - `SandboxConsumersForProduct`: returns active subscribers with a sandbox key
    only (excludes pending/rejected and keyless apps).
  - Approve / Reject / Unsubscribe rebuild the product's sandbox route per the
    rule.
  - `ReprovisionPlan` updates the sandbox consumer's limit.
  - Product sandbox-upstream change rebuilds / clears the sandbox route.
  - `RotateSandboxKey`: gateway-before-DB, old→new key, event logged; `409`
    when no sandbox key.
- **Validation:** `sandbox_upstream_url` accepted/rejected by `ValidUpstream`
  (reuse the upstream-validation cases); admin create/update with a sandbox
  upstream.
- **Try-it:** the sandbox proxy injects the sandbox key, targets the sandbox
  gateway, and is SSRF-safe (host fixed); access gates (owner + active sub +
  has sandbox key) → 403 otherwise.

### Frontend (vitest)

- CredentialsTab sandbox card states: no-key → "Activer le sandbox"; after
  enable → reveal + rotate; hidden when no sandbox-eligible subscription.
- Try-it toggle: Sandbox option visibility (product sandbox upstream + app key)
  and server-URL switch.
- `enableSandbox` / `rotateSandboxKey` client fns hit the right URLs with auth.

### Live (controller)

Bring up the sandbox gateway (`docker compose up -d apisix-sandbox`); as admin,
set a sandbox upstream on a product; as an approved subscriber, enable sandbox →
call the sandbox gateway with the sandbox key → 200 to the sandbox backend; the
production key on the sandbox gateway → 401; rotate the sandbox key (old→401,
new→200); the Try-it Production/Sandbox toggle round-trips in the browser.

## Out of scope (deferred)

- Explicit **"disable sandbox"** (delete the sandbox consumer + clear the key +
  drop from all sandbox routes). V1 is enable + rotate.
- **Sandbox usage on the quota meter** — the meter reads the production
  Prometheus only; sandbox metering (a sandbox Prometheus scrape) is a later
  add.
- **OpenAPI-import auto-mapping** of a second (sandbox) server URL — sandbox
  upstream is set manually by the admin in V1.
- Pre-subscription / anonymous sandbox evaluation (this V1 is subscriber-only).
