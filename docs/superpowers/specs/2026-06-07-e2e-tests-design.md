# End-to-End Test Coverage — Full Lifecycle + CI

**Date:** 2026-06-07
**Scope:** Add automated end-to-end coverage for the whole project: a Go test
that drives the portal's public HTTP API against the real backend + APISIX
gateway (lifecycle + authorization negatives), and a CI job that brings the
stack up and runs every layer on each push. Closes the gap the foundation spec
named ("End-to-end (smoke): register → subscribe → call the gateway … expect
200, then exceed the plan limit → expect 429").

## Decisions (user-confirmed)

1. **Go HTTP-API E2E** (backend + gateway truth), **plus authorization
   negatives** (cross-tenant + non-admin) — the highest-value, lowest-flakiness
   layer. Playwright UI E2E is **dropped** in favor of a CI job (below): for a
   project this size the Go E2E + the 147 component tests already cover the
   journeys, and a browser layer is the flakiest/highest-maintenance option.
2. **Full lifecycle**: publish → subscribe → approve → 200 → 429 → unsubscribe
   → 403, plus reject and delete-with-active-subs (409), plus authz negatives.
3. **Env-gated locally, run in CI**: the E2E needs the docker stack + real DB;
   it is skipped in plain `go test ./...` / `vitest` (which stay fast and
   hermetic, matching `RUN_APISIX_IT`) but **CI runs it for real** so the gated
   suite cannot silently rot.

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

**Authorization negatives** (same file, separate `t.Run` blocks — cheap,
non-flaky, DB-only, no gateway timing). Register two developers A and B; A
creates an application:
- **cross-tenant read**: B `GET /api/applications/<A's appId>` → **404** (the
  `owns` check treats another tenant's app as not-found); assert NOT 200 and
  A's apiKey is not leaked in the body.
- **cross-tenant mutate**: B `POST /api/applications/<A's appId>/subscriptions`
  and B `DELETE /api/applications/<A's appId>/subscriptions/<productId>` →
  **404/403** (never 200/204).
- **non-admin blocked**: developer A's token on `POST /api/admin/products`,
  `GET /api/admin/subscriptions`, `POST /api/admin/subscriptions/<id>/approve`
  → **403** each.
- **no token**: those admin endpoints with no Authorization header → **401**.
Kept in the E2E file under the same `RUN_E2E` gate for one cohesive suite.

### Part C — CI job

`.github/workflows/ci.yml` — runs every layer on push / pull_request so the
gated E2E cannot silently rot:
- **job `unit`** (no services): `go test ./internal/... ./cmd/...` (the faked-
  gateway unit tests that don't need a DB) + `cd web && npm ci && npx vitest run`
  + `npx tsc -b` + `npm run build`.
- **job `e2e`**: `docker compose up -d` (postgres, etcd, apisix, echo), wait for
  health (poll `/healthz` on the portal is N/A here — poll APISIX admin + a
  `pg_isready`-style DB check, bounded), then `RUN_E2E=1 go test ./internal/e2e/...
  -count=1`. Uses the repo's dev secrets (already the compose defaults). Tears
  the stack down in `if: always()`.
The job runs the real DB + APISIX so the lifecycle + gateway assertions execute
for real in CI — addressing the "gated tests rot" risk directly.

### Part D — Wiring & docs

- `Makefile`: keep `test` (unit, fast, hermetic). Add:
  - `test-e2e` → `RUN_E2E=1 go test ./internal/e2e/... -count=1 -v` (assumes
    `make up` done).
  - `e2e` → `up` + bounded wait + `test-e2e` (convenience).
  - `test-it` → `RUN_APISIX_IT=1 go test ./internal/apisix/... -run Integration`.
- `docs/testing.md`: the layers (unit / backend E2E+IT / CI), what each covers,
  exact commands, the env-gating rationale, prerequisites (docker stack up), and
  how CI runs them.

## Testing (of the tests)

- Part A refactor: `go build ./... && go test ./internal/... ./cmd/...` green
  (unchanged behavior); `go vet ./...` clean.
- Part B: with the stack up, `RUN_E2E=1 go test ./internal/e2e/...` passes
  (lifecycle + authz negatives); with the stack down (or `RUN_E2E` unset) it
  **skips** (not fails). Verify both. Re-run twice to confirm no cross-run
  collision (unique names + cleanup).
- Part C: validate the workflow YAML (`actionlint` if available, else careful
  review); confirm step ordering brings the stack up before the gated test and
  tears down on failure. (Actual GitHub execution happens on push.)
- Whole repo gates still green: `go test ./internal/... ./cmd/...`, `cd web &&
  npx vitest run` (147), `tsc -b`, `npm run build`.

## Out of scope

- Playwright/browser UI E2E (dropped — see decision 1; the component tests +
  Go E2E cover the journeys at far lower flakiness/maintenance).
- Load/perf testing; multi-user concurrency beyond the lifecycle.
- Backend feature gaps (metrics, key rotation, app delete) — unchanged.
