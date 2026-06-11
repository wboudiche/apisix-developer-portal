# Security Review — APISIX Developer Portal

**Date:** 2026-06-08
**Scope:** Full application — Go backend (`internal/`, `cmd/`), React frontend
(`web/`), and the docker-compose dev stack (`docker-compose.yml`,
`deploy/apisix/config.yaml`). Read-only review across four threat areas:
authentication/authorization/multi-tenancy, injection/gateway-provisioning/SSRF,
secrets/transport/configuration, and frontend.

## Executive summary

**Application-level security is strong; deployment/secrets/transport posture is
dev-grade.** The authorization model, multi-tenant isolation, and injection
handling are well built and (for authz) E2E-verified. The serious risk is
concentrated in how the stack is *configured and exposed* — an open APISIX admin
port, fail-open secret defaults, and no TLS — which would be dangerous if shipped
as-is but is mostly config to fix.

Severity counts: **1 Critical, 5 High, 6 Medium, 4 Low.** Verified-clean areas
are listed at the end.

---

## Critical

### C1 — APISIX Admin API exposed to the host with `allow_admin: 0.0.0.0/0`
**Where:** `docker-compose.yml:30` (`"19180:9180"`), `deploy/apisix/config.yaml:16-17`
(`allow_admin: 0.0.0.0/0`), `deploy/apisix/config.yaml:14` (admin key committed).
**Scenario:** The APISIX Admin API is published on host port 19180, accepts any
source IP, and is protected only by an admin key committed in the repo. Anyone
who can reach the host can `PUT /apisix/admin/routes|consumers` directly —
create/replace routes, strip key-auth and rate limits, or repoint upstreams —
**bypassing the portal's authorization model entirely** (the portal is meant to
be the only writer of routes/consumers).
**Fix:** Remove the `19180` host port mapping (the portal reaches APISIX over the
internal Docker network); restrict `allow_admin` to the compose subnet; move the
admin key out of source into a secret/env and rotate it.

---

## High

### H1 — Dev-secret guard fails open (default `PORTAL_ENV=dev`)
**Where:** `internal/config/config.go:40` (`Env` defaults to `"dev"`),
`config.go:45-52` (`isDevLike` treats `""`/`dev`/`development`/`test` as dev),
`config.go:63-66` (`Validate` short-circuits for dev-like), `cmd/portal/main.go:22-27`.
**Scenario:** `Validate()` only rejects the built-in dev secrets when `PORTAL_ENV`
is explicitly non-dev. The default is `dev`, so a production deploy that forgets
to set `PORTAL_ENV` boots with the **public** JWT secret `dev-secret-change-me`
and the committed APISIX admin key — and only logs a warning. With the public
JWT secret, anyone forges an HS256 token with `role:"admin"` and any `uid`
(`internal/auth/token.go:22-31`) → full admin + tenant impersonation.
**Fix:** Fail closed — require explicit `PORTAL_ENV=dev` to allow dev secrets
(treat unset/empty as production), or refuse to start on the built-in defaults
unless `ALLOW_DEV_SECRETS=1`. Also reject `JWT_SECRET` shorter than 32 bytes.

### H2 — No TLS anywhere (plaintext credentials, tokens, keys)
**Where:** `cmd/portal/main.go` (plain `ListenAndServe`), `internal/config/config.go:34,37`
(`sslmode=disable`, `http://…:19180`), `internal/apisix/client.go` (admin key over HTTP).
**Scenario:** Login credentials, `Authorization: Bearer` JWTs, generated API
keys, and the APISIX admin key all traverse plaintext HTTP. On any non-loopback
network these are sniffable / MITM-able.
**Fix (deployment, not portal code):** Terminate TLS at a reverse proxy in front
of the portal and gateway; use HTTPS to the admin API in prod; set Postgres
`sslmode=require`/`verify-full` for non-local DB. Documented here as the
recommended production topology rather than implemented in the Go server.

### H3 — No rate limiting on authentication endpoints
**Where:** `internal/auth/handler.go:41-86`, mounted at `internal/server/server.go`
with no limiter.
**Scenario:** `/api/auth/login` and `/api/auth/register` accept unlimited
attempts → online brute force, credential stuffing, registration flooding.
bcrypt slows each guess but nothing caps the rate.
**Fix:** Per-IP (and ideally per-account) rate limiting with backoff/lockout on
the auth routes.

### H4 — SSRF via unrestricted `upstreamUrl`
**Where:** `internal/admin/product.go` `validUpstream` (only checks `host:port`
shape) → `internal/subscriptions/service.go` → `internal/apisix/client.go`
`EnsureRoute` (sets the upstream node). Re-applied on update via
`internal/admin/service.go`.
**Scenario:** `validUpstream` does not block loopback, link-local, or private
ranges. An admin (the bar is "can publish an API product") can point APISIX at
`169.254.169.254` (cloud metadata → IAM creds), `127.0.0.1:19180` (the admin API
itself), or any internal-only service reachable from the gateway. Reachable
through the gateway by any approved consumer.
**Fix:** In `validUpstream`, reject loopback / link-local (`169.254.0.0/16`) /
RFC1918 / ULA hosts unless explicitly allow-listed via config; prefer an
operator-controlled upstream allow-list over free-form `host:port`.

### H5 — Role trusted from JWT for 24h; no revocation
**Where:** `internal/auth/middleware.go:33-34,78-84`, `internal/auth/token.go:22-31`.
**Scenario:** `RequireAuth`/`RequireAdmin` read `userID`/`role` solely from the
token and never reconcile with the DB. Tokens last a fixed 24h with no refresh,
no `jti`, no denylist, no logout. A demoted admin keeps admin for up to 24h; a
deleted user's token keeps working; a leaked token is usable until expiry.
**Fix:** Re-load the user's current role from the DB on admin routes (closes the
demoted-admin window cheaply); longer term add short-lived access tokens +
refresh + a revocation mechanism (`jti` denylist or `token_version`/
`password_changed_at` checked per request).

---

## Medium

### M1 — `contextPath` unvalidated → route overlap / shadowing
**Where:** `internal/admin/product.go` (only a non-empty check) →
`internal/subscriptions/service.go` `EnsureRoute(..., prod.ContextPath+"/*", …)`.
**Scenario:** No charset/shape/uniqueness validation. Two products can be given
the same or overlapping `contextPath` (e.g. both `/` or `/v1`), producing
colliding APISIX route URIs that shadow each other's traffic — a cross-product
boundary break (combine with H4 to point a broad path at an attacker upstream).
Arbitrary characters flow straight into the APISIX `uri`. (The route *id*
`prod_<id>` is safe — derived from a DB int.)
**Fix:** Validate to a strict pattern (`^/[a-zA-Z0-9/_-]+$`, must start with `/`,
no `*`), normalize, and enforce non-overlap/uniqueness across products.

### M2 — JWT in `localStorage` + no CSP / security headers
**Where:** `web/src/auth/AuthProvider.tsx` (token in `localStorage`),
`internal/server/server.go` (only `logRequests` middleware),
`internal/httpx/respond.go` (sets only `Content-Type`).
**Scenario:** A 24h JWT in `localStorage` is readable by any JS on the origin →
any XSS = silent token theft. No `Strict-Transport-Security`,
`X-Content-Type-Options`, `X-Frame-Options`/CSP → clickjacking, MIME sniffing,
and nothing blunts an XSS. (CORS is absent, which is correct for a same-origin
SPA — no permissive `*` problem.)
**Fix:** Add a security-headers middleware (`nosniff`, `X-Frame-Options: DENY`/
`frame-ancestors 'none'`, a CSP for the SPA, HSTS once TLS is in place). Longer
term, prefer an `HttpOnly; Secure; SameSite=Strict` cookie for the token.

### M3 — Account enumeration + login timing oracle
**Where:** `internal/auth/handler.go:53-55` (register `409 "email already
registered"`), `handler.go:75-78` (login skips bcrypt when the user is absent).
**Scenario:** Register's distinct 409 is a direct email-existence oracle. Login
runs bcrypt only for existing users, so response timing distinguishes
"user exists" from "user absent" even though the error string is uniform.
**Fix:** Neutral register response; always run a bcrypt compare against a dummy
hash on login when the user is absent to equalize timing. Combine with H3.

### M4 — No client-side `401 → logout`
**Where:** `web/src/api/client.ts`, `web/src/auth/AuthProvider.tsx`.
**Scenario:** No central handler clears the token / forces re-login on a 401, and
there's no client-side expiry check. Expired/revoked tokens linger in
`localStorage` and the user sees generic load errors instead of being logged out.
**Fix:** On `res.status === 401` in `parse`/`sendAuthed`, clear auth and redirect
to `/login`.

### M5 — Committed DB password + 5432 exposed; `sslmode=disable`
**Where:** `docker-compose.yml:6-9`, `.env.example`, `internal/config/config.go:34`.
**Scenario:** Trivial `portal/portal` credentials, Postgres published to the host
(`5432:5432`), no TLS. Fine for local dev, but there's no prod guard forcing a
real password (unlike H1's secret guard, which itself fails open).
**Fix:** Don't publish 5432 in prod; require a real `DATABASE_URL`; enable DB TLS
for non-local use.

### M6 — Weak password policy
**Where:** `internal/auth/handler.go:43` (only `len < 8`).
**Scenario:** No complexity / breach check; bcrypt silently truncates at 72 bytes
with no rejection, so long passphrases lose entropy past byte 72.
**Fix:** Stronger policy or breached-password check; reject/normalize > 72 bytes
(or pre-hash with SHA-256 before bcrypt).

---

## Low

### L1 — `crypto/rand` error ignored in API key generation
`internal/subscriptions/repo.go` `GenerateKey` discards the `rand.Read` error;
if the entropy source ever fails, the key becomes a predictable all-zeros
constant. Check and propagate the error. (Entropy and length are otherwise good —
128-bit from `crypto/rand`.)

### L2 — API keys stored in plaintext
Keys live in the `credentials` table in plaintext and are returned by the
owner-gated detail endpoint. No cross-tenant exposure, but a DB leak yields live
gateway keys. Consider storing a hash and showing the key once at creation.

### L3 — bcrypt at `DefaultCost`
Cost ~10 is acceptable; consider 12 for production.

### L4 — Build reproducibility / external font
Web uses floating semver ranges; commit a lockfile and run `npm audit` in CI.
`web/src/styles/base.css` imports Google Fonts (the only non-`/api` external
call; carries no token/key) — self-host to remove the dependency.

---

## Verified clean

- **SQL injection** — every query across all repos uses pgx placeholders; the
  catalog sort is whitelisted (no `ORDER BY` injection); the only dynamic SQL
  builds `$N` placeholders, not concatenated input.
- **Multi-tenancy / IDOR** — every app-scoped query is filtered by `owner_id`;
  detail/subscribe/unsubscribe gate on the `owns` check; cross-tenant access is
  blocked (confirmed by `internal/e2e/authz_test.go`).
- **Admin enforcement** — every `/api/admin/*` route is mounted behind
  `RequireAdmin`; no admin action is reachable without it.
- **JWT algorithm confusion** — `alg=none` and RS↔HS confusion are blocked
  (HMAC pinned in the keyfunc; `Valid` checked).
- **Privilege escalation at register** — role is hardcoded `developer` in the
  INSERT and cannot be set via the request body; admin is granted only via
  `EnsureAdminRole` for the configured `ADMIN_EMAIL`.
- **Mass assignment** — create/update decode into fixed structs; `id`/`owner`/
  `role`/`published`(non-admin) are not client-settable.
- **Path params** — `{id}`/`{appID}`/`{productID}` parsed via `ParseInt` with 400
  on error; no unparsed strings reach SQL or APISIX.
- **APISIX resource ids** — consumer `app_<id>` and route `prod_<id>` derived
  from DB ints; the plugin set is hardcoded (no plugin/id injection).
- **Frontend XSS** — no exploitable sinks (the one `dangerouslySetInnerHTML` is a
  static icon-table lookup); backend strings render as escaped JSX; API calls are
  same-origin `/api` only; no client-side secrets; `npm audit` reports 0 vulns.
- **Password hashing** — bcrypt with constant-time compare; no plaintext logging.
- **Error handling & logging** — fixed `{"error": msg}` responses (no stack
  traces / SQL errors leaked); the request logger logs `method path -> status`
  only (no headers, bodies, query strings, or secrets).

---

## Remediation checklist

Priority order. Status is updated as fixes land (2026-06-11: all code/config
items remediated on the `security-hardening` branch; see notes below the table).

| # | Severity | Item | Type | Status |
|---|----------|------|------|--------|
| C1 | Critical | Bind admin port to loopback; restrict `allow_admin` | config | ☑ |
| H1 | High | Fail-closed dev-secret guard + min JWT secret length | code | ☑ |
| H3 | High | Rate limiting on auth endpoints | code | ☑ |
| H4 | High | Block loopback/private ranges in `validUpstream` | code | ☑ |
| H5 | High | Re-check admin role from DB in `RequireAdmin` | code | ☑ |
| H2 | High | TLS termination (portal/gateway/admin/DB) | deployment | ☐ (documented) |
| M1 | Medium | Validate + de-overlap `contextPath` | code | ☑ |
| M2 | Medium | Security-headers middleware (+ CSP) | code | ☑ |
| M3 | Medium | Neutralize login timing oracle | code | ☑ |
| M4 | Medium | Client-side `401 → logout` | code | ☑ |
| M5 | Medium | Dev: 5432 bound to loopback ☑ / prod DB password + TLS | config | ☐ (documented) |
| M6 | Medium | Password policy + 72-byte handling | code | ☑ |
| L1 | Low | Propagate `crypto/rand` error in `GenerateKey` | code | ☑ |
| L2 | Low | Encrypt stored API keys at rest (AES-256-GCM) | code | ☑ |
| L3 | Low | bcrypt cost → 12 | code | ☑ |
| L4 | Low | Commit web lockfile; self-host fonts | build | ☐ |

Implementation notes (deviations from the original "Fix" wording, agreed in the
2026-06-08 hardening spec):

- **C1** — the admin port stays published but bound to `127.0.0.1` (the E2E and
  integration suites reach it at `localhost:19180`); `allow_admin` is loopback
  + the RFC1918 ranges instead of any-source. Prod guidance unchanged: don't
  publish the port at all, inject a rotated key.
- **H4** — IP literals and resolved hostname addresses are both checked
  (stubbable `lookupIP`, fail closed); `UPSTREAM_ALLOW_PRIVATE=1` is the dev
  escape hatch for docker-internal upstreams. Residual: validation-time DNS is
  TOCTOU (rebinding) — long-term fix is an operator upstream allow-list.
- **H5** — only the admin-role DB re-check was implemented; full token
  revocation/refresh remains future work.
- **M3** — the login timing oracle is closed (dummy bcrypt compare on absent
  users + per-account rate limit). Register deliberately keeps its distinct
  409 as a UX trade-off; the rate limiter blunts bulk enumeration through it.
- **L2** — implemented as **AES-256-GCM encryption at rest** (`v1:`-prefixed
  ciphertext, `CREDENTIAL_ENC_KEY` = base64 of 32 random bytes), *not* the
  originally suggested hash-and-show-once: the portal must re-display the key
  (CredentialsTab, quickstart) and re-provision APISIX consumers with it, so
  it has to stay recoverable. Pre-encryption plaintext rows fail decryption
  with a clear error — recreate dev DBs (`docker compose down -v`).
