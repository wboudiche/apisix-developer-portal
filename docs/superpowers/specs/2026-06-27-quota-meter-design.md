# Per-App Rate-Limit Meter — Design

**Date:** 2026-06-27
**Status:** Approved, ready for planning
**Surface:** Application detail → Usage tab (`web/src/pages/application/UsageTab.tsx`), `internal/subscriptions` + `internal/metrics`.

## Problem

The portal shows usage stats but never tells a developer how close they are to
their rate limit. A live meter ("≈ used / limit over the last window") is a
cheap, high-value addition — and one WSO2 4.x's portal lacks. We build it from
the metrics the portal already collects.

## Reality / locked decisions (from brainstorming)

- The APISIX `limit-count` is **per-consumer (per-app)**, keyed by
  `consumer_name` — NOT per-product. So the meter is **per-application**: one
  shared limit across all the app's subscribed APIs.
- The plan limit is a **rate** (`count` per `window_seconds`, e.g. 1000/60s),
  **not** a monthly quota. The meter shows rate-limit headroom over the plan's
  window, not a calendar quota. (A real monthly quota was considered and
  deferred — it would need a new plan field + usage accounting.)
- "Used" is **approximate**, derived from Prometheus
  (`apisix_http_status{consumer="app_N"}`), not the gateway's exact internal
  limit-count counter. The UI labels it `≈` / "approx.".
- Placement: atop the **Usage tab**, above the existing stat cards.

## Backend

### Endpoint: `GET /api/applications/{id}/quota`

Added to the subscriptions `Handler` next to `/usage` (already mounted under
`/api/applications/` behind `requireAuth`, owner-scoped via the handler's
`authorize`). Composes data the handler can already reach:

- **Limit + window:** `reader.ActivePlanForApp(appID)` (the helper added for key
  rotation) → `PlanInfo{Count, WindowSeconds}`.
- **Consumer:** `reader.GetCredential(appID)` → `ConsumerUsername` (`app_<id>`).
- **Used:** new metrics method `RequestsInWindow(consumer, windowSeconds)`.

Flow:
1. `authorize` (non-owner → 403/404).
2. `GetCredential` → `ErrNotFound` ⇒ `200 {"hasQuota": false}` (no consumer yet).
3. `ActivePlanForApp` → `ErrNoActiveSubscription` ⇒ `200 {"hasQuota": false}`.
4. If `h.usage == nil` (Prometheus unconfigured) ⇒
   `200 {"hasQuota": true, "limit": N, "windowSeconds": W, "available": false}`.
5. `used, err := h.usage.RequestsInWindow(ctx, consumer, W)`; on error ⇒ same
   `available:false` shape (log server-side); else
   `200 {"hasQuota": true, "used": U, "limit": N, "windowSeconds": W, "available": true}`.

**Response type** (`subscriptions.Quota`):
```
{ hasQuota bool; used int64; limit int; windowSeconds int; available bool }
```
`Cache-Control: no-store` (per-tenant data, same posture as `/usage`).

### Metrics: `RequestsInWindow`

New method on `metrics.Service` (and the `UsageReader` interface the handler
holds): `RequestsInWindow(ctx, consumer string, windowSeconds int) (int64, error)`.
PromQL: `sum(increase(apisix_http_status{consumer="<consumer>"}[<windowSeconds>s]))`
(the same metric `/usage` uses; reuse the existing `consumerRe` guard and the
`Scalar` querier). Returns the rounded scalar. Caching is optional and may be
skipped (the value is meant to be live).

### Interface additions

- `subscriptions.Reader` += `ActivePlanForApp(ctx, appID) (PlanInfo, error)`
  (already implemented by `*subscriptions.Repo`).
- `subscriptions.UsageReader` += `RequestsInWindow(ctx, consumer string, windowSeconds int) (int64, error)`
  (implemented by `*metrics.Service`; a `metrics.Querier`-based unit test covers it).

## Frontend

### `getQuota` client fn

`getQuota(token, appId): Promise<Quota>` → `GET /api/applications/{id}/quota`.
`type Quota = { hasQuota: boolean; used?: number; limit?: number; windowSeconds?: number; available?: boolean }`.

### `QuotaMeter` component (atop `UsageTab`)

- Fetches quota on mount (and when `appId` changes), with an alive-guard.
- `hasQuota === false` → render nothing (the app has no active subscription).
- `available === false` → show the limit + "métriques indisponibles" (no fake bar).
- `available === true` → a progress bar `used/limit` + the label
  `≈ {used} / {limit} requêtes sur les dernières {windowSeconds}s (approx.)`.
  The bar uses a warning color at ≥80% and a danger color at ≥100% (clamped).
- Rendered above the existing stat cards in `UsageTab`; reuses Atlas tokens.

## Testing

- **Go**
  - `metrics.RequestsInWindow`: builds the exact PromQL for a consumer+window
    and returns the rounded scalar (fake `Querier`); rejects a malformed
    consumer via `consumerRe`.
  - Quota handler: `hasQuota:false` for no-credential and for no-active-sub;
    `available:false` when `usage` is nil; happy path returns
    used/limit/windowSeconds with `available:true`; owner-scoped (non-owner →
    403/404). Use the existing handler-test fakes (`newMemStore`, fake usage
    reader) + a fake `UsageReader.RequestsInWindow`.
- **Frontend (vitest)**
  - `getQuota` client fn POSTs/GETs the right URL with auth and parses the body.
  - `QuotaMeter`: renders the bar + label when available; "métriques
    indisponibles" when `available:false`; renders nothing when
    `hasQuota:false`; the warning/danger color thresholds.
- **Live (controller)**
  - Hit the try-it/echo product several times, open the Usage tab, confirm the
    meter shows a non-zero `used` against the plan limit and the window matches
    the plan.

## Out of scope (deferred)

- A real monthly quota (new plan field + cumulative accounting + reset).
- Exact gateway counter (APISIX limit-count remaining is not queryable here).
- Per-subscription/per-product meters (the limit is per-consumer, so a
  per-product meter would misrepresent the shared limit).
