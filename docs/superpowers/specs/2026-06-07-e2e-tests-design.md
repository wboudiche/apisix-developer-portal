# End-to-End Test Coverage — Full Lifecycle, Both Layers

**Date:** 2026-06-07
**Scope:** Add automated end-to-end coverage for the whole project: a Go test
that drives the portal's public HTTP API against the real backend + APISIX
gateway, and a Playwright suite that drives the React UI journeys. Closes the
gap the foundation spec named ("End-to-end (smoke): register → subscribe → call
the gateway … expect 200, then exceed the plan limit → expect 429").

## Decisions (user-confirmed)

1. **Both layers**: Go HTTP-API E2E (backend + gateway truth) AND Playwright UI
   E2E (frontend journeys).
2. **Full lifecycle**: publish → subscribe → approve → 200 → 429 → unsubscribe
   → 403, plus reject and delete-with-active-subs (409).
3. **Env-gated**: E2E layers need the docker stack + real DB; they are skipped in
   plain `go test ./...` / `vitest` (which stay fast and hermetic), matching the
   existing `RUN_APISIX_IT` pattern.

## Current state (verified)

- Unit/component: 147 Vitest tests (mocked API), Go package unit tests with
  faked `apisix.Gateway`.
- DB-backed repo tests skip when `DATABASE_URL` is unset.
- `internal/apisix/client_it_test.go` (`RUN_APISIX_IT=1`) drives the APISIX
  *client* directly (EnsureConsumer/EnsureRoute → 401/200/429). It does NOT go
  through the portal's HTTP API.
- **Gap:** no test exercises the portal's public HTTP endpoints (the ones the
  React app calls) end-to-end through to gateway enforcement.
- `cmd/portal/main.go` builds the router inline in `main()` — not reusable.

## Architecture

### Part A — Router factory (refactor)

New `internal/server/server.go`:

```
package server
func New(pool *pgxpool.Pool, cfg config.Config, gw apisix.Gateway) http.Handler
```

It contains the exact handler-wiring block currently inline in
`cmd/portal/main.go` (catalog/auth/plans/applications/subscriptions/admin
handlers, `requireAuth`/`requireAdmin`, the `mux`, `logRequests`), plus the
admin-role seeding call. `main.go` becomes: load cfg, connect+migrate db, build
gw, `h := server.New(pool, cfg, gw)`, serve `h`. The `statusRecorder`/
`logRequests` helpers move into `server` (or stay private there). No behavior
change; existing unit tests stay green and the E2E mounts `server.New(...)`.

### Part B — Go full-lifecycle E2E

`internal/e2e/lifecycle_test.go`, build/skip-gated:

```go
if os.Getenv("RUN_E2E") != "1" { t.Skip("set RUN_E2E=1 with the compose stack up") }
```

Setup (helpers in the same file):
- `DATABASE_URL` (default `postgres://portal:portal@localhost:5432/portal?sslmode=disable`),
  `APISIX_ADMIN_URL` (`http://localhost:19180`), `APISIX_ADMIN_KEY`
  (`edd1c9f034335f136f87ad84b625c8f1`), `APISIX_GATEWAY_URL`
  (`http://localhost:9080`). If the DB can't connect → `t.Skip`.
- `db.Connect` + `db.Migrate`; `apisix.NewClient`; `cfg` via `config.Load()`
  overridden with the test DB/APISIX values; `srv := httptest.NewServer(server.New(pool, cfg, gw))`.
- A tiny JSON HTTP helper (`do(method, path, token, body) (status, respBytes)`).
- Unique names per run derived from a caller-supplied timestamp/counter (NOT
  `time.Now()` inside the test body if it complicates determinism — a package
  var seeded once at test start is fine; uniqueness, not reproducibility, is
  the goal). Slug/ctx/email all carry the unique suffix.

Flow (one ordered test, sub-steps via `t.Run` for readable failures):
1. **admin login** → `POST /api/auth/login` admin@portal.local/adminpass → token.
2. **publish** → `POST /api/admin/products` {name, slug, category, contextPath
   `/e2e_<uniq>`, upstreamUrl `echo:8080`, version, published:true} → 201.
   `POST /api/admin/plans` {name `E2E_<uniq>`, rateLimit 2, windowSeconds 60} → 201.
3. **developer** → `POST /api/auth/register` fresh email → token; `POST
   /api/applications` {name} → appId; `POST /api/applications/<appId>/subscriptions`
   {productId, planId} → 200/201, capture apiKey, assert subscription `pending`
   via `GET /api/applications/<appId>` (detail) or the admin queue.
4. **pre-approval gateway** → `GET <gw>/e2e_<uniq>/x` with apikey →
   **403** (consumer not whitelisted yet) — accept 403 or 404 (route may not
   exist pre-approval); assert NOT 200. Document which in the assertion.
5. **approve** → admin `GET /api/admin/subscriptions?status=pending` → find id →
   `POST /api/admin/subscriptions/<id>/approve` → 204. Poll the detail until
   status `active` (short bounded retry; APISIX propagation also needs a brief
   sleep before gateway calls).
6. **gateway enforcement** (small retry/sleep for APISIX route propagation):
   - no key → **401**
   - with key → **200**
   - exceed limit (call `>2` times in the window) → **429**
7. **unsubscribe** → `DELETE /api/applications/<appId>/subscriptions/<productId>`
   → then gateway with key → **403**.
8. **reject path** → second app subscribes → admin `reject` → detail never
   `active`; gateway stays 403.
9. **delete-409** → `DELETE /api/admin/products/<productId>` while a *different*
   still-active subscription exists → **409**. (Re-create a fresh active sub if
   step 7 cleared the first; or assert 409 before step 7's unsubscribe — order
   the steps so an active sub exists at delete time, then clean up.)
- `t.Cleanup`: delete the product (best-effort), delete created APISIX
  consumers/routes (`prod_<id>`, `app_<appId>`), so reruns are clean.

### Part C — Playwright UI E2E

`web/e2e/` with `@playwright/test` (dev dependency) + `web/playwright.config.ts`
(testDir `e2e`, baseURL `http://localhost:5174`, single chromium project, no
auto-webServer — the stack/vite are started manually, documented). A `lifecycle.spec.ts`:

Journeys (UI-observable; mocked nothing — real running app):
- **admin publishes**: login admin → `/admin/products` → composer → create a
  product with a unique name → it appears in the rows and in the catalog `/`.
- **developer subscribes**: register/login a fresh dev → open the product in the
  catalog → subscribe (choose app + plan) → the application's subscription shows
  **En attente**.
- **admin approves**: login admin → `/admin/approvals` → Approuver the row → it
  leaves the queue.
- **developer sees active**: dev → app detail → status **Active**, the
  Identifiants tab shows a real key, the Aperçu quickstart shows the real path.

Gateway 200/429 is NOT asserted here (browser can't cleanly test APISIX
rate-limiting) — that lives in Part B. `package.json` script `test:e2e` →
`playwright test`. Browser binaries via `npx playwright install chromium`
(documented, not run in CI by default).

### Part D — Wiring & docs

- `Makefile`: keep `test` (unit, fast). Add:
  - `test-e2e` → `RUN_E2E=1 go test ./internal/e2e/... -count=1 -v` (assumes
    `make up` done). Also a convenience `e2e` that does `up` + wait + `test-e2e`.
  - Optionally `test-it` → `RUN_APISIX_IT=1 go test ./internal/apisix/... -run Integration`.
- `docs/testing.md`: the three layers (unit / integration+E2E backend / UI E2E),
  what each covers, exact commands, the env-gating rationale, and the
  prerequisites (docker stack up, vite+portal running for Playwright).

## Testing (of the tests)

- Part A refactor: `go build ./... && go test ./internal/... ./cmd/...` green
  (unchanged behavior); `go vet ./...` clean.
- Part B: with the stack up, `RUN_E2E=1 go test ./internal/e2e/...` passes; with
  the stack down (or `RUN_E2E` unset) it **skips** (not fails). Verify both.
- Part C: with vite+portal+stack up, `npm run test:e2e` passes; verify it
  exercises the real lifecycle (a unique product created via UI ends Active).
- Whole repo gates still green: `go test ./internal/... ./cmd/...`, `cd web &&
  npx vitest run` (147), `tsc -b`, `npm run build`.

## Out of scope

- CI pipeline configuration (the gating makes these runnable in CI later; wiring
  a CI file is a separate task).
- Load/perf testing; multi-user concurrency beyond the lifecycle.
- Backend feature gaps (metrics, key rotation, app delete) — unchanged.
