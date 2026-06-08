# Security Hardening — Remediation of the 2026-06-08 Review

**Date:** 2026-06-08
**Source:** `docs/security-review.md` (findings + severities).
**Scope:** Fix every code-addressable finding; document the deployment-only ones.

## Decisions (user-confirmed)

1. **Fix everything codeable** — C1, H1, H3, H4, H5, M1, M2, M3, M4, M6, L1, L2,
   L3. **H2 (TLS) and M5 (prod DB)** remain documented deployment items (already
   in `security-review.md`). **L4** (web lockfile/fonts) is build hygiene — done
   opportunistically if cheap, else left in the doc.
2. **Token transport:** keep the JWT in `localStorage`; add a security-headers
   middleware (CSP, nosniff, frame-deny) + client-side `401 → logout`. No cookie
   migration in this pass (documented as a future option).

## Two resolved design tensions

### L2 — keys must stay retrievable → encrypt at rest, not hash
The portal needs the plaintext API key for two live features: **displaying** it
(CredentialsTab reveal/copy, Overview quickstart) and **re-provisioning** APISIX
consumers (`ReprovisionPlan` → `EnsureConsumer(cred.APIKey)` on plan edits). A
one-way hash / show-once model would break both. Therefore L2 is implemented as
**encryption at rest**: keys are stored AES-256-GCM-encrypted, decrypted in the
repo layer when read. This adds one managed secret, `CREDENTIAL_ENC_KEY`
(32 bytes, base64), governed by the same fail-closed guard as the JWT secret
(H1): a built-in dev default that prod refuses to boot with.

### C1 — keep local tooling working while removing network exposure
The E2E (`internal/e2e`) and integration (`internal/apisix/client_it_test.go`)
suites reach the admin API at `http://localhost:19180`. So instead of removing
the port, **bind it to loopback** (`127.0.0.1:19180:9180`) and **restrict
`allow_admin`** to loopback + the Docker bridge range. Local tests still work;
the admin API is no longer reachable from other hosts on the network. Prod
guidance (don't publish the port at all; inject a rotated key) stays in the doc.

## Changes by finding

### Backend — config & secrets
- **H1** `internal/config/config.go`: invert the env default so an **unset/empty
  `PORTAL_ENV` is treated as production** (only `dev`/`development`/`test` are
  dev-like). `Validate()` then rejects built-in dev secrets unless explicitly in
  a dev env. Add a **min-length check**: reject `JWT_SECRET` < 32 bytes in
  non-dev. Add `CredentialEncKey` to `Config` with a dev default
  `DevCredentialEncKey` and include it in `UsesDevSecrets()`/`Validate()`.
- **C1** `docker-compose.yml`: `"127.0.0.1:19180:9180"` and
  `"127.0.0.1:5432:5432"` (M5 dev hardening). `deploy/apisix/config.yaml`:
  `allow_admin` → `127.0.0.0/8` + the compose bridge CIDR (e.g. `172.16.0.0/12`)
  instead of `0.0.0.0/0`.

### Backend — middleware
- **H3** new `internal/httpx` (or `internal/auth`) **rate-limit middleware**:
  in-memory per-IP token bucket (`golang.org/x/time/rate`), applied to
  `/api/auth/` mounts in `internal/server/server.go`. Configurable burst/rate
  with safe defaults (e.g. 5 req/burst, refill ~1/2s); returns **429** on limit.
  Client IP from `RemoteAddr` (documented: behind a proxy, needs
  `X-Forwarded-For` handling — noted, not trusted blindly).
- **M2** new **security-headers middleware** wrapping the whole mux in
  `server.New`: `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`,
  `Referrer-Policy: no-referrer`, and a `Content-Security-Policy` suitable for the
  SPA (`default-src 'self'`; allow the Google Fonts origins the app uses, or
  self-host — see L4; `frame-ancestors 'none'`). HSTS added only behind TLS
  (documented; emitted when `PORTAL_ENV` is prod to avoid breaking local HTTP).

### Backend — auth
- **H5** `internal/auth/middleware.go` `RequireAdmin`: after verifying the token,
  **re-load the user's current role from the DB** (needs a `UserStore`/lookup
  injected into the middleware) and authorize on the DB role, not the claim.
  Closes the demoted-admin window. `RequireAuth` unchanged (userID claim is fine;
  deletion is a rarer concern, documented).
- **M3** `internal/auth/handler.go`: **register** returns a neutral response that
  does not distinguish existing emails (still create-or-conflict internally, but
  the response/timing is uniform); **login** always runs a bcrypt comparison
  against a fixed dummy hash when the user is absent, to equalize timing.
- **M6** `internal/auth/handler.go` + `user.go`: password policy — reject > 72
  bytes (bcrypt truncation) and keep the 8-char min; optionally a basic
  complexity note. `user.go` `HashPassword` rejects > 72 bytes explicitly.
- **L3** `internal/auth/user.go`: bcrypt cost `12` (constant).

### Backend — gateway/provisioning
- **H4** `internal/admin/product.go` `validUpstream`: parse the host and
  **reject loopback, link-local (169.254/16), RFC1918, and ULA** unless an
  explicit allow-list env (`UPSTREAM_ALLOW_PRIVATE=1`, for the dev stack which
  uses `echo:8080` / docker-internal hosts) is set. Dev stack sets the flag so
  `echo:8080` keeps working; prod blocks private targets by default.
  NOTE: the dev stack's `echo` resolves to a bridge IP (private), so the dev
  flag is required for existing tests — set it in docker-compose for the portal
  service and in the E2E/IT env.
- **M1** `internal/admin/product.go`: validate `contextPath` to
  `^/[A-Za-z0-9/_-]+$` (must start `/`, no `*`/spaces), normalize trailing
  slashes; add a **uniqueness/overlap check** against existing products
  (normalized prefix) returning 409 on collision.

### Backend — crypto hygiene
- **L1** `internal/subscriptions/repo.go` `GenerateKey`: check the `crypto/rand`
  error and propagate (signature returns `(string, error)`; callers updated) or
  `panic` on entropy failure — choose propagate; update `GetOrCreateCredential`
  and `genKey` plumbing (`subscriptions.GenerateKey` currently `func() string` —
  becomes `func() (string, error)` or wraps with a fatal on failure; pick the
  minimal-churn option: keep `func() string` but panic on rand failure, with a
  comment, since callers can't meaningfully recover and it's an entropy outage).
- **L2** encrypt-at-rest for `api_key` (see tension above): new
  `internal/crypto` AES-256-GCM `Encrypt/Decrypt` using `CredentialEncKey`;
  `subscriptions/repo.go` encrypts on write, decrypts on read; a **migration**
  is NOT needed for fresh dev DBs but existing rows would be plaintext — provide
  a note/one-shot re-encrypt is out of scope (dev DBs are disposable; documented).

### Frontend
- **M2/M4** `web/src/api/client.ts`: on `401`, clear stored auth and redirect to
  `/login` (a small hook into `AuthProvider`'s logout, or a window redirect +
  localStorage clear). `web/index.html` / serving layer: the CSP is set
  server-side (M2 backend), so no FE change needed beyond the 401 handling.
- **L4** (if cheap) commit the web lockfile and self-host fonts; else leave in
  the doc.

## Testing

- Go: unit tests for each new pure unit — config `Validate` (dev vs prod, short
  secret), `validUpstream` (private/loopback blocked, allow-flag), `contextPath`
  validation + overlap, the rate limiter (allows burst, 429s over limit),
  security-headers middleware (asserts headers present), `RequireAdmin` DB
  re-check (token says admin but DB says developer → 403), login timing/enum
  (absent user still runs compare; register neutral), AES-GCM round-trip.
- E2E (`internal/e2e`, `RUN_E2E=1`): must still pass — the SSRF allow-flag is set
  for the dev stack so `echo:8080` works; admin-port loopback binding keeps the
  harness reaching `localhost:19180`; the rate limiter must not break the E2E's
  auth calls (limits set high enough, or the limiter keyed so the test's
  sequential calls pass). Re-run the full lifecycle + authz suites.
- Frontend: a Vitest test that a `401` from a mocked api call triggers logout.
- Whole-repo gates: `go test ./internal/... ./cmd/...`, `go vet`, web `vitest` +
  `tsc -b` + `build`, and `RUN_E2E=1 go test ./internal/e2e/...` green.
- Update `docs/security-review.md` checklist statuses as items land.

## Out of scope (documented, not implemented)
- **H2** TLS termination, **M5** prod DB credential/TLS enforcement — deployment
  topology, not portal code.
- Full token revocation / refresh-token system (H5 long-term) — only the admin
  DB-role re-check is implemented now.
- HttpOnly-cookie token migration — documented future option.
- Re-encrypting pre-existing plaintext key rows in long-lived DBs.
