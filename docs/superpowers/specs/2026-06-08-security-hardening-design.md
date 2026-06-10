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
repo layer when read. This adds one managed secret, `CREDENTIAL_ENC_KEY` — the
**base64 encoding of 32 random bytes** (`openssl rand -base64 32`),
**base64-decoded at startup** (boot fails if it doesn't decode to exactly 32
bytes); a raw ASCII passphrase would carry far less than 256 bits of entropy.
It is governed by the same fail-closed guard as the JWT secret (H1): a built-in
dev default (itself base64) that prod refuses to boot with.

### C1 — keep local tooling working while removing network exposure
The E2E (`internal/e2e`) and integration (`internal/apisix/client_it_test.go`)
suites reach the admin API at `http://localhost:19180`. So instead of removing
the port, **bind it to loopback** (`127.0.0.1:19180:9180`) and **restrict
`allow_admin`** to loopback + the RFC1918 ranges (`127.0.0.0/8`, `10.0.0.0/8`,
`172.16.0.0/12`, `192.168.0.0/16` — compose networks aren't guaranteed to land
in one bridge CIDR; the loopback port binding is the primary control, the
allow-list is defense-in-depth that removes the literal any-source rule). Local
tests still work; the admin API is no longer reachable from other hosts on the
network. Prod guidance (don't publish the port at all; inject a rotated key)
stays in the doc.

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
- **H3** new `internal/httpx` **rate-limit middleware**: in-memory per-IP token
  bucket (hand-rolled, no new dependency), applied to `/api/auth/` mounts in
  `internal/server/server.go`. Configurable burst/rate with safe defaults
  (e.g. 10 req/burst, refill ~1/2s); returns **429** with a `Retry-After`
  header on limit. The bucket map **evicts stale entries** (idle buckets swept
  when the map grows past a bound) so unique client IPs can't grow memory
  unboundedly — the limiter must not itself be a DoS vector. In addition to the
  per-IP middleware, the **login handler applies a per-account bucket** (keyed
  by lowercased email) so distributed stuffing of one account is also capped.
  Client IP from `RemoteAddr` (documented: behind a proxy, needs
  `X-Forwarded-For` handling — noted, not trusted blindly).
- **M2** new **security-headers middleware** wrapping the whole mux in
  `server.New`: `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`,
  `Referrer-Policy: no-referrer`, and a `Content-Security-Policy` suitable for the
  SPA (`default-src 'self'`; allow the Google Fonts origins the app uses, or
  self-host — see L4; `frame-ancestors 'none'`). The CSP keeps
  `style-src 'unsafe-inline'` for the React app's inline styles — a deliberate
  weakening, noted in the code. Also set **`Cache-Control: no-store` on
  `/api/` paths** so responses carrying secrets (the credentials endpoint
  returns the live API key over GET) never land in an HTTP cache. **HSTS is
  not emitted by the portal** — it belongs at the TLS-terminating proxy
  (documented with H2).

### Backend — auth
- **H5** `internal/auth/middleware.go` `RequireAdmin`: after verifying the token,
  **re-load the user's current role from the DB** (needs a `UserStore`/lookup
  injected into the middleware) and authorize on the DB role, not the claim.
  Closes the demoted-admin window. A **lookup error is a 500** (could not
  verify), not a 403 — a transient DB outage must not masquerade as "admin
  only"; only `role != "admin"` yields 403. `RequireAuth` unchanged (userID
  claim is fine; deletion is a rarer concern, documented).
- **M3** `internal/auth/handler.go`: **login** always runs a bcrypt comparison
  against a fixed dummy hash when the user is absent, to equalize timing (the
  serious half of the oracle). **Register keeps its distinct 409** as a
  deliberate UX trade-off (users must learn an email is taken) — documented in
  a code comment; the H3 rate limiter blunts bulk enumeration through it.
- **M6** `internal/auth/handler.go` + `user.go`: password policy — reject > 72
  bytes (bcrypt truncation) and keep the 8-char min; optionally a basic
  complexity note. `user.go` `HashPassword` rejects > 72 bytes explicitly.
- **L3** `internal/auth/user.go`: bcrypt cost `12` (constant).

### Backend — gateway/provisioning
- **H4** `internal/admin/product.go` `validUpstream`: parse with
  `net.SplitHostPort` (not string-cutting — also handles IPv6 literals), then
  **reject loopback, link-local (169.254/16), RFC1918, ULA, and unspecified**
  addresses unless an explicit allow-list env (`UPSTREAM_ALLOW_PRIVATE=1`, for
  the dev stack which uses `echo:8080` / docker-internal hosts) is set.
  For **IP literals**, check the parsed IP. For **hostnames**, resolve at
  validation time (via an overridable `lookupIP` hook for tests) and reject if
  the name does not resolve or *any* resolved address is private — this closes
  shorthand-IP bypasses (`127.1`, `0x7f.0.0.1` parse as hostnames but resolve
  to loopback) and attacker domains pointing at internal ranges. Dot-less
  hostnames (docker names) are rejected without the flag. Dev stack sets the
  flag so `echo:8080` keeps working; prod blocks private targets by default.
  **Residual risk (documented):** validation-time resolution is TOCTOU — a DNS
  record can change between validation and proxying (rebinding). The long-term
  fix is an operator-controlled upstream allow-list instead of free-form
  `host:port`; out of scope here.
  NOTE: the dev stack's `echo` resolves to a bridge IP (private), so the dev
  flag is required for existing tests — set it in docker-compose for the portal
  service and in the E2E/IT env.
- **M1** `internal/admin/product.go`: validate `contextPath` to
  `^/[A-Za-z0-9/_-]+$` (must start `/`, no `*`/spaces), normalize trailing
  slashes; add a **uniqueness/overlap check** against existing products
  returning 409 on collision. Overlap is **prefix-overlap on `/` boundaries**,
  not just equality: reject if the new path is a path-prefix of an existing one
  or vice versa (`/v1` vs `/v1/orders` collide as APISIX routes — `/v1/*`
  shadows `/v1/orders/*`). Exact uniqueness is additionally enforced by a **DB
  unique index on `context_path`** (new migration), mirroring the existing
  slug 23505→409 handling, so concurrent creates can't race past the check;
  the prefix-overlap query remains check-then-insert (low residual race,
  admin-only surface — documented).

### Backend — crypto hygiene
- **L1** `internal/subscriptions/repo.go` `GenerateKey`: check the `crypto/rand`
  error and propagate (signature returns `(string, error)`; callers updated) or
  `panic` on entropy failure — choose propagate; update `GetOrCreateCredential`
  and `genKey` plumbing (`subscriptions.GenerateKey` currently `func() string` —
  becomes `func() (string, error)` or wraps with a fatal on failure; pick the
  minimal-churn option: keep `func() string` but panic on rand failure, with a
  comment, since callers can't meaningfully recover and it's an entropy outage).
- **L2** encrypt-at-rest for `api_key` (see tension above): new
  `internal/crypto` AES-256-GCM `Encrypt/Decrypt` using `CredentialEncKey`
  (base64-decoded, see tension above); `subscriptions/repo.go` encrypts on
  write, decrypts on read. Ciphertext is stored with a **`v1:` version prefix**
  so a future key rotation (decrypt-with-old, re-encrypt-with-new) is possible
  without another wipe-the-DB event; rotation itself stays out of scope. A
  **migration** is NOT needed for fresh dev DBs but existing rows would be
  plaintext — provide a note/one-shot re-encrypt is out of scope (dev DBs are
  disposable; documented).

### Frontend
- **M2/M4** `web/src/api/client.ts`: on `401`, clear stored auth and redirect to
  `/login` (a small hook into `AuthProvider`'s logout, or a window redirect +
  localStorage clear). `web/index.html` / serving layer: the CSP is set
  server-side (M2 backend), so no FE change needed beyond the 401 handling.
- **L4** (if cheap) commit the web lockfile and self-host fonts; else leave in
  the doc.

## Testing

- Go: unit tests for each new pure unit — config `Validate` (dev vs prod, short
  secret), `validUpstream` (private/loopback blocked; shorthand/hostname
  resolution rejected via a stubbed `lookupIP`; allow-flag), `contextPath`
  validation + prefix-overlap (`/v1` vs `/v1/orders` → 409), the rate limiter
  (allows burst, 429 + `Retry-After` over limit, per-IP isolation, stale-bucket
  eviction), security-headers middleware (headers present; `no-store` on
  `/api/`), `RequireAdmin` DB re-check (token says admin but DB says developer
  → 403; lookup error → 500), login timing (absent user still runs compare;
  per-email bucket 429s repeated attempts), AES-GCM round-trip (and `v1:`
  prefix present; bad/missing prefix rejected).
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
