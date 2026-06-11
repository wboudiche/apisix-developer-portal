# Testing

Three layers, fastest first.

## 1. Unit / component (default, hermetic)

- **Backend:** `make test` (`go test ./internal/... ./cmd/...`). Package tests
  with a faked `apisix.Gateway`. DB-backed repo tests **skip** when
  `DATABASE_URL` is unset; with a database up they run.
- **Frontend:** `cd web && npx vitest run` — component tests with the API
  mocked, plus `npx tsc -b` and `npm run build`.

These need no docker stack and run in seconds. They do **not** cross the
frontend↔backend or backend↔APISIX boundaries (both are mocked) — that is the
job of the E2E layer.

## 2. End-to-end (real DB + real APISIX gateway)

`internal/e2e` mounts the real portal handler in-process and drives its public
HTTP API, then calls the live APISIX gateway. Gated by `RUN_E2E=1` so it never
runs in the hermetic suite.

```sh
make up            # postgres, etcd, apisix, echo
make test-e2e      # PORTAL_ENV=dev UPSTREAM_ALLOW_PRIVATE=1 RUN_E2E=1 go test ./internal/e2e/...
# or one shot:
make e2e           # up + wait + test-e2e
```

Since the 2026-06 security hardening, local E2E (and `make run`) need two env
vars, which the Makefile targets set for you:

- `PORTAL_ENV=dev` — an unset env is now treated as **production**, which
  refuses the built-in dev secrets at boot.
- `UPSTREAM_ALLOW_PRIVATE=1` — the SSRF guard otherwise rejects products whose
  upstream is a docker-internal/private host like `echo:8080`.

API keys are now stored AES-GCM-encrypted. A database created **before** that
change holds plaintext rows that fail decryption — recreate the dev DB once
with `docker compose down -v && make up`.

When the portal runs behind a reverse proxy in production, set
`TRUSTED_PROXIES` to the proxy's CIDR(s) (comma-separated, e.g.
`TRUSTED_PROXIES=10.0.0.0/8`). The per-IP auth rate limiter then reads the real
client IP from `X-Forwarded-For` instead of bucketing every request under the
proxy's address. Leave it unset for a directly-exposed server (local dev) so
`X-Forwarded-For` is ignored and can't be spoofed.

Covers: publish a product + plan → developer subscribes → admin approves →
gateway 401 (no key) / 200 (key) / 429 (over the plan limit) → unsubscribe 403
→ delete-with-active-subscription 409, plus authorization negatives
(cross-tenant read/mutate, non-admin blocked, no-token 401).

It **skips** (not fails) when `RUN_E2E` is unset or the DB/stack is down.

## 3. APISIX client integration

`internal/apisix/client_it_test.go` drives the APISIX admin client directly:

```sh
make up
make test-it       # RUN_APISIX_IT=1 go test ./internal/apisix/...
```

## CI

`.github/workflows/ci.yml` runs layer 1 in the `unit` job and layer 2 in the
`e2e` job (it brings the docker stack up, waits for Postgres + APISIX, runs the
gated E2E for real, and tears the stack down). This keeps the gated suite from
silently rotting — every push exercises the real lifecycle.
