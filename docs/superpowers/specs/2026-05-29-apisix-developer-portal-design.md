# APISIX Developer Portal — Design (V1)

> Status: draft for review · Date: 2026-05-29
> A WSO2-style developer portal that sits on top of Apache APISIX.

## 1. Purpose & scope

Build a self-service **developer portal** for APIs running on Apache APISIX, modeled on the WSO2 API Developer Portal. The defining capability of V1 is **discover → self-subscribe → get credentials**: an external developer browses a catalog of published APIs, subscribes an application to one, and automatically receives an API key that works against the live APISIX gateway.

The validated visual design is the **"Atlas"** catalog page (`/index.html` mockup): sticky top bar, dark category/tag sidebar, and a searchable/filterable grid of API cards with a Subscribe action.

### In scope (V1)
- Local-account auth (register / login).
- API catalog: browse, search, filter by category & tag, sort, grid/list views.
- API detail page.
- Applications: a developer creates apps that hold credentials.
- Subscriptions: subscribe an application to an API at a chosen plan (rate-limit tier).
- Credential issuance via APISIX **key-auth** (one API key per application).
- Automatic APISIX provisioning on subscribe (consumer + access enforcement + rate limit).
- Minimal admin: publish/unpublish API products and define plans.
- Light **and** dark theme.

### Out of scope (later phases)
OAuth2/JWT credentials · OIDC/SSO login · usage analytics dashboards · interactive OpenAPI docs / try-it-out console · ratings & comments · subscription approval workflows · multi-org tenancy · SDK generation.

## 2. Architecture

Faithful to WSO2's model: **the portal owns its own datastore as the source of truth** and provisions APISIX downstream via the Admin API. APISIX plays the combined role of gateway + key manager + traffic manager.

```
React SPA (Atlas UI)
      │  REST/JSON
      ▼
Go backend (portal API)  ──────►  PostgreSQL   (source of truth)
      │
      │  APISIX Admin API (:9180)
      ▼
Apache APISIX  ◄──►  etcd        (data plane: routes, consumers, plugins)
```

### Stack
- **Frontend:** React (SPA), ported from the Atlas mockup. Design tokens (oklch palette, Bricolage Grotesque + IBM Plex Sans + JetBrains Mono) extracted to CSS variables; light + dark themes.
- **Backend:** Go (HTTP API). Layered into focused packages, each with one clear responsibility (see §5).
- **Database:** PostgreSQL.
- **Gateway:** Apache APISIX + etcd.
- **Dev infra:** `docker-compose` brings up postgres + etcd + apisix (+ backend + frontend) for a one-command local environment. Greenfield APISIX assumed.

## 3. Domain model (PostgreSQL)

- **User** — `id, email, password_hash, name, role (developer|admin), created_at`.
- **ApiProduct** — catalog entry: `id, name, slug, category, version, context_path, description, tags[], icon, published, upstream_url, openapi_spec?, apisix_route_id, created_by, timestamps`.
- **Plan** — rate-limit tier: `id, name (Free|Silver|Gold), rate_limit_count, rate_limit_window_seconds, quota?`.
- **Application** — `id, owner_user_id, name, description, created_at`. Owns credentials.
- **Subscription** — `id, application_id, api_product_id, plan_id, status (active), created_at`.
- **Credential** — `id, application_id, api_key, apisix_consumer_username, status, created_at`. One key-auth credential per application.

**Decisions to confirm:** we include the WSO2 **Application** abstraction (an app groups subscriptions and holds one key), rather than issuing a separate key per API. Subscriptions are **auto-approved** in V1 (no approval workflow).

## 4. Key flows

### 4.1 Publish (admin)
Admin creates an ApiProduct (name, category, version, context path, upstream, plan eligibility, tags). On publish, the backend creates an **APISIX Route** for the context path → upstream, with the `key-auth` plugin enabled. The ApiProduct now appears in the catalog.

### 4.2 Discover & subscribe (developer)
1. Developer registers/logs in (local account).
2. Browses the catalog (Atlas), opens an API.
3. Picks/creates an **Application** and **Subscribes** at a Plan.
4. Backend provisions APISIX and returns the **API key**.

### 4.3 Provisioning on subscribe (APISIX, key-auth)
On a new subscription the backend, via the Admin API:
1. **Ensures a Consumer** for the Application exists, with a `key-auth` credential (the generated API key).
2. **Grants access** to the subscribed route — add the consumer to that route's `consumer-restriction` allow-list (this enforces "only subscribed APIs can be called," the WSO2 equivalent).
3. **Applies the plan's rate limit** — `limit-count` (count / window) scoped to the consumer on that route.

The developer then calls the gateway with `apikey: <their key>`. Unsubscribing reverses steps 2–3; deleting an application removes its consumer.

### 4.4 Error handling
- Provisioning is treated as a transaction: if any Admin API call fails, the backend rolls back partial APISIX state and the subscription is not marked active; the user sees a clear error. No silent failures.
- Admin API calls are retried with backoff on transient errors; persistent failures surface to the user/admin.

## 5. Backend structure (Go) — units & boundaries

Each package has one purpose, a clear interface, and is testable in isolation:

- `auth` — registration, login, sessions/JWT, password hashing. **`AuthProvider` interface** (V1: `LocalAuth`; later: `OIDCAuth`).
- `catalog` — ApiProduct CRUD + query (search/filter/sort).
- `plans` — plan definitions.
- `applications` — application CRUD per user.
- `subscriptions` — subscribe/unsubscribe orchestration; calls the provisioner.
- `credentials` — **`CredentialProvider` interface** (V1: `KeyAuthProvider`; later: `OAuth2Provider`). Generates keys, maps to APISIX consumers.
- `apisix` — typed Admin API client (routes, consumers, plugins) behind a `Gateway` interface so it can be faked in tests.
- `admin` — publish/unpublish, plan management (admin-role only).
- `httpapi` — REST handlers, wiring, middleware (authz by role).

The two interfaces (`CredentialProvider`, `AuthProvider`) are the seams that let OAuth2/JWT and OIDC drop in later without a rewrite.

## 6. Frontend structure (React)

Port the Atlas design, decomposed into components: `TopBar`, `CategoryRail`, `ApiCard`, `CatalogGrid`, `FilterChips`, `ApiDetail`, `SubscribeModal` (choose app + plan → shows issued key), `ApplicationList`/`ApplicationDetail` (keys, rotate), `Login`/`Register`, `AdminProducts`. Catalog data comes from the backend (`GET /api/products`), replacing the hardcoded `APIS` array. Theme tokens drive light/dark via a `data-theme` attribute.

## 7. Testing approach
- **Backend:** unit tests per package; the `apisix.Gateway` interface is faked to test subscription/provisioning logic without a live gateway. An integration test runs provisioning against a real APISIX from docker-compose.
- **Frontend:** component tests for catalog filtering/subscribe flow; the API layer is mocked.
- **End-to-end (smoke):** register → subscribe → call the gateway with the issued key → expect 200, then exceed the plan limit → expect 429.

## 8. Milestones (suggested build order)
1. Repo scaffold + docker-compose (postgres, etcd, apisix) + health checks.
2. Backend: schema/migrations, auth (local), catalog read API; seed the 9 sample APIs.
3. Frontend: port Atlas catalog to React against the real catalog API; light/dark.
4. Applications + subscriptions + `KeyAuthProvider` + APISIX provisioning (the core loop).
5. API detail + SubscribeModal + Application/keys UI.
6. Minimal admin (publish products, manage plans).
7. E2E smoke test; polish.

## 9. Open questions for reviewer
- Confirm the **Application** abstraction and **auto-approve** subscriptions for V1.
- Confirm **enforcement via `consumer-restriction`** (vs. allowing any valid key to hit any key-auth route).
- Is a **minimal admin UI** in V1 enough, or should products be seeded via config/migration only for now?

## 10. Addendum — decisions superseded or clarified since (2026-06-07)

Recorded after implementation review; the sections above are kept as written.

- **§4.1 route-at-publish → superseded by lazy provisioning.** Publishing only
  writes the product row (`published=true` = catalog visibility). The APISIX
  route `prod_<id>` is created on the **first subscription approval** and
  rebuilt on whitelist/upstream changes. Consequence: a published product with
  no subscribers has no gateway route (its path 404s rather than 401s).
- **§4.2/§4.3 auto-approve → superseded by the approval workflow**
  (2026-05-30 admin spec, locked): subscribe stores `pending` and provisions
  nothing; only admin approval provisions the consumer + route.
- **§4.4 transactionality → implemented as an ordering guarantee** (commit
  d3cf092): `Approve` marks the subscription `active` only after both gateway
  calls (consumer + route incl. the new consumer) succeed, so
  *active in DB ⇒ provisioned in gateway*. A failure leaves the row `pending`
  and surfaces the error; re-approve is idempotent and converges.
  Retries with backoff remain **future work**.
- **§4.3 "deleting an application removes its consumer" → future work.** No
  application-delete endpoint exists yet (the portal UI shows a demo toast);
  APISIX consumers are never deleted today.
- **Data model:** `apisix_route_id` and `openapi_spec` were dropped; route ids
  are derived (`prod_<id>`, locked in the admin spec). Key rotation and a
  metrics pipeline remain future work (the application page shows demo data
  for those seams, see 2026-06-05 app-detail spec).
