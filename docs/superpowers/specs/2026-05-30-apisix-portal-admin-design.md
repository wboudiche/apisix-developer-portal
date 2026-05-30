# APISIX Developer Portal — Admin (Plan 4) Design

**Date:** 2026-05-30
**Status:** Approved (design)
**Builds on:** Plans 1–3 (foundation, frontend, subscribe loop + subscribe UI) on `master`.

## Goal

Give portal administrators the tools to manage the catalog and govern access:
manage API products (including real upstreams, replacing the `echo:8080` demo
seed), manage rate-limit plans, and approve/reject subscriptions before they are
provisioned into APISIX. Admin functionality is exposed both as an API
(`/api/admin/*`) and as a role-gated React UI (`/admin`).

This closes the **known gap** from Plan 3: seeded products have empty
`upstream_url`, so HTTP subscribe only works against the echo demo. Product CRUD
with a per-product `upstream_url` makes real products subscribable end-to-end.

## Scope

In scope (decomposed into four sequenced sub-plans):

- **4a — Admin auth + Product CRUD.** `RequireAdmin`, role in request context,
  seeded admin, `/api/admin/products` CRUD + publish toggle + per-product
  upstream; re-provision route on upstream change.
- **4b — Plan management.** `/api/admin/plans` CRUD; re-provision affected
  consumers on rate-limit edit.
- **4c — Subscription approval.** `subscriptions.status`; subscribe → `pending`
  (no provisioning); admin approve → provision + `active`; reject → `rejected`.
- **4d — Admin UI.** Role-gated `/admin` React pages: product editor, plan
  editor, approvals queue.

Out of scope (future plans): per-product auto-approve flag; API key rotation;
subscription soft-delete/audit log; OIDC/SSO; CORS config; per-product route ids
(routes remain derived as `prod_<id>`).

## Decisions (locked)

- **Admin auth: role in JWT + `RequireAdmin`.** The JWT already carries `role`
  (`internal/auth/token.go`); login already issues it. `RequireAuth` currently
  discards it. Consequence: a role change takes effect on the admin's next login
  (24h token TTL). Acceptable for V1; documented.
- **Delete product with active subscriptions → `409`.** Unpublish is the soft
  path: `published=false` hides it from the public catalog but leaves the APISIX
  route and consumers untouched, so existing consumers keep working.
- **Plan rate-limit edit → re-provision affected consumers now.** Immediately
  `EnsureConsumer` with the new limits for every application currently subscribed
  on that plan (N admin-API calls; admin actions are low-volume).
- **Universal approval-required for V1.** Every new subscription waits for admin
  approval. There is no per-product auto-approve flag in this scope.

## Architecture

New package `internal/admin` (handlers + repo), mounted at `/api/admin/` behind
`RequireAdmin`. It owns **writes** to `api_products`, `plans`, and the approval
transitions of `subscriptions`; the existing `catalog`, `plans`, and
`subscriptions` packages keep their **reads** and the developer-facing subscribe
flow.

Re-provisioning (rebuild a product's route whitelist, rebuild a consumer for a
plan) is needed by both the admin path and the subscribe path. The reusable
provisioning helpers are extracted so both share one implementation against the
`apisix.Gateway` interface (`internal/apisix`), rather than duplicating logic.

```
                 writes                         reads
 admin pkg ────────────────► api_products ◄──────────── catalog pkg
 (/api/admin, RequireAdmin)  plans         ◄──────────── plans pkg
                             subscriptions ◄───────────► subscriptions pkg
        │                                                      │
        └──────── shared provisioning helpers ────────────────┘
                  (EnsureRoute / EnsureConsumer via apisix.Gateway)
```

## Components

### 1. Admin auth & authorization (4a)

- `RequireAuth` additionally stores `claims.Role` in request context via new
  `WithRole`/`Role` accessors (mirroring `WithUserID`/`UserID`).
- `RequireAdmin(tk)` middleware: validates the Bearer JWT, then `403` unless
  `claims.Role == "admin"`. Standalone (does not chain `RequireAuth`).
- **Admin seeding:** config field `ADMIN_EMAIL` (default `admin@portal.local`).
  Migration `0006` promotes that email to `admin` if present; on startup an
  idempotent `UPDATE users SET role='admin' WHERE email=$ADMIN_EMAIL` ensures the
  role. First run: register that email, then it is promoted. No bootstrap
  endpoint.

### 2. Product management (4a)

Endpoints behind `RequireAdmin`:

- `GET /api/admin/products` — all products incl. unpublished.
- `POST /api/admin/products` — create.
- `GET /api/admin/products/{id}` — single, incl. unpublished.
- `PUT /api/admin/products/{id}` — update.
- `DELETE /api/admin/products/{id}` — delete.

Fields: `name, slug, category, version, context_path, description, tags, icon,
upstream_url, published`.

Behavior:

- Admin reads bypass the `published=true` filter.
- **On `upstream_url` change** while the product has active subscriptions →
  re-provision the route (rebuild whitelist from active subscribers, new
  upstream). Takes effect immediately.
- **DELETE** → `409` if active subscriptions exist; otherwise delete row +
  best-effort `DELETE /routes/prod_<id>`.
- **Unpublish** (`published=false`) → catalog visibility only; route + consumers
  untouched.

### 3. Plan management (4b)

Endpoints behind `RequireAdmin`:

- `GET /api/admin/plans` — all plans.
- `POST /api/admin/plans` — create.
- `PUT /api/admin/plans/{id}` — update.
- `DELETE /api/admin/plans/{id}` — delete.

Fields: `name, rate_limit_count, rate_limit_window_s`.

Behavior:

- **On rate-limit edit** → for every application currently subscribed on the
  plan, `EnsureConsumer` with the new limits.
- **DELETE** → `409` if any subscription references the plan.

### 4. Subscription approval (4c)

- **Schema (migration `0007`):** add `status TEXT NOT NULL DEFAULT 'active'` to
  `subscriptions`. Existing rows default to `active` (no break). New
  subscriptions are created `pending`.
- **Subscribe flow change:** `Subscribe` persists the subscription as `pending`
  and performs **no** APISIX provisioning. The credential/key is still created
  (the developer has an app identity), but the route whitelist does not include
  the app until approval, so the key will not pass the gateway yet.
- **Admin endpoints behind `RequireAdmin`:**
  - `GET /api/admin/subscriptions?status=pending` — approval queue (app, product,
    plan, developer email).
  - `POST /api/admin/subscriptions/{id}/approve` → set `active`, then provision
    (ensure consumer with plan limits + rebuild route whitelist). Idempotent.
  - `POST /api/admin/subscriptions/{id}/reject` → set `rejected`, no provisioning.
- **Whitelist correctness:** `ConsumersForProduct` returns only `active`
  subscribers, so pending/rejected apps never leak into a route whitelist.
- **Developer-facing:** the Applications view shows each subscription's status
  (Pending / Active / Rejected) so the developer understands why a key is not yet
  working.

### 5. Admin UI (4d)

React, gated behind the admin role.

- `AuthProvider` exposes `role` (from the decoded token) so the app can gate.
- **Route group `/admin`** wrapped in an admin guard that redirects non-admins to
  the catalog. TopBar shows an "Admin" link only when `role === 'admin'`.
- **Products page:** table of all products (published + unpublished); create/edit
  form (all fields incl. `upstream_url`, `published` toggle); delete with confirm
  and graceful `409` message.
- **Plans page:** table + create/edit form (name, count, window); delete with
  `409` handling.
- **Approvals queue:** list of `pending` subscriptions with Approve / Reject;
  reflects status after action.
- Reuses existing tokens/styling (Atlas design, light/dark).

## Data flow

Admin writes `api_products` / `plans` / `subscriptions`; catalog/plans/
subscriptions read. Shared provisioning helpers (route-rebuild-for-product,
consumer-rebuild-for-plan) are the single path to the `apisix.Gateway`, used by
both the admin path and the subscribe/approve path.

## Error handling

- `403` — authenticated non-admin hitting `/api/admin/*`.
- `401` — missing/invalid token.
- `409` — destructive action with dependencies (delete product with active subs;
  delete plan in use).
- `404` — unknown id.
- `400` — validation (bad slug, malformed `upstream_url` host:port, empty/invalid
  rate limits).
- Internal errors are logged server-side and return a generic message (matches
  the Plan-3 polish-2 pattern; no internal-error leakage).

## Testing

- Repos and services tested hermetically with `apisix.Fake`.
- The live integration test (`RUN_APISIX_IT=1`) is extended to cover
  approve → provision → `200` and reject → `401`.
- Frontend: vitest per admin page (guard redirect, product/plan CRUD forms,
  approvals queue actions), following the existing TDD pattern.

## Migrations

- `0006_seed_admin.sql` — promote `ADMIN_EMAIL` to `admin` (idempotent).
- `0007_subscription_status.sql` — add `subscriptions.status` (default `active`).

## Build order

4a (auth + product CRUD) → 4b (plans) → 4c (approval) → 4d (UI). Each sub-plan
ships working, tested software and is approved before the next. 4a alone closes
the upstream gap; 4c is the only change to existing subscribe behavior and is
deliberately isolated.
