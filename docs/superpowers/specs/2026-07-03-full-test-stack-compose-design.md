# Full Test Stack (one-command docker-compose) — Design

**Date:** 2026-07-03
**Status:** Approved, ready for planning
**Surface:** new root `Dockerfile` + `.dockerignore`; `web/Dockerfile` + `web/nginx.conf`; `docker-compose.full.yml` (override); `deploy/lemonldap/` (baked LemonLDAP::NG config); `Makefile` (`full`/`full-down`); a README/runbook note. No application code changes.

## Problem

The repo's `docker-compose.yml` brings up **infrastructure only** (postgres, etcd,
apisix, apisix-sandbox, echo, mailpit, prometheus); the Go API and the Vite web
app run on the host (`make run` / `pnpm dev`). Two gaps block "test every feature
from one command": (1) there is **no bundled OIDC provider**, so the OAuth2-for-
consumers feature (Suite 7 of the QA runbook) can't be exercised end-to-end; and
(2) starting the portal + web by hand is friction. This adds a **single-command,
fully-containerized test stack** — every component, including a LemonLDAP::NG
identity provider — so `make full` → open one URL → test everything.

## Locked decisions (from brainstorming)

- **Bundle LemonLDAP::NG** as the OIDC provider (the org's WebSSO/OIDC suite). The
  portal's OAuth2 is the **client-credentials** grant, validated at the APISIX
  gateway via **JWKS** + a client-id claim — LL::NG supports this.
- **Fully containerize**: add `portal` + `web` containers so one `docker compose up`
  runs the whole system; the browser opens a single URL.
- **Keep the existing `docker-compose.yml` untouched** (the host-dev loop stays as
  is). The full stack is an **override file** layered on it.

## Architecture — compose layering

- Base `docker-compose.yml` (unchanged): etcd, apisix, apisix-sandbox, echo,
  mailpit, prometheus, postgres.
- New **`docker-compose.full.yml`** (override), run as
  `docker compose -f docker-compose.yml -f docker-compose.full.yml up` (wrapped by
  `make full`). It ADDS three services and reuses everything else:
  - **`portal`** — the Go API (new root `Dockerfile`), wired by service name to
    postgres/apisix/sandbox/prometheus/mailpit/lemonldap; the SPA reaches it via
    the web container's `/api` proxy (no host port needed, but one is mapped for
    direct API poking during tests).
  - **`web`** — the built SPA served by nginx (new `web/Dockerfile` +
    `web/nginx.conf`), proxying `/api` → `portal:8080`. Host port **`8088:80`** —
    the single URL to open.
  - **`lemonldap`** — LemonLDAP::NG (OIDC provider), with a baked config and a
    **network alias** matching its issuer hostname.

## The portal image — root `Dockerfile`

- Multi-stage: `golang:1.25` builds a static binary (`CGO_ENABLED=0 go build
  ./cmd/portal`); the DB migrations are `go:embed`-ed into the binary, so the
  runtime stage only needs the binary. Runtime = a slim base (`gcr.io/distroless/
  static` or `alpine`), non-root, `EXPOSE 8080`, `ENTRYPOINT ["/portal"]`.
- `.dockerignore` excludes `web/node_modules`, `.git`, build artifacts, `.superpowers`.
- The portal has **no DB connect-retry**, so in compose it `depends_on: postgres:
  {condition: service_healthy}` (postgres already has a healthcheck) and
  `depends_on: apisix` (started) — enabling gateway metrics at boot is best-effort
  (logs, doesn't crash, per existing behavior).

## The web image — `web/Dockerfile` + `web/nginx.conf`

- Multi-stage: a `node` stage runs the package manager install + `pnpm build`
  (`tsc -b && vite build` → `dist/`); an `nginx:alpine` stage serves `dist/`.
- `web/nginx.conf`: `try_files $uri /index.html;` (SPA fallback) +
  `location /api/ { proxy_pass http://portal:8080; proxy_set_header Host $host;
  proxy_set_header X-Forwarded-For $remote_addr; }`. The frontend already uses
  **same-origin relative `/api` paths** (the `langHeaders`/`fetch` calls), so no
  frontend change and no CORS.
- Host port `8088:80`.

## LemonLDAP::NG — client-credentials OIDC

- **Image:** the LemonLDAP::NG all-in-one (Apache + Portal + Manager + Handler),
  reading its configuration from a mounted conf volume.
- **Baked config `deploy/lemonldap/`** pre-defines, so no manual UI setup:
  - the **OIDC issuer/service enabled**, with an RSA signing key (so tokens are
    RS256-signed and a **JWKS** endpoint is served);
  - one **relying party** (the portal's app client) with a fixed **`client_id`** +
    `client_secret`, **client-credentials grant enabled**, and **JWT access tokens
    enabled** (so the access token APISIX receives is a verifiable JWT, not opaque);
  - consent bypassed; a scope the RP may request.
- **Issuer URL reachability (must resolve identically in two worlds):**
  - APISIX (in-network) fetches `<issuer>/.well-known/openid-configuration` + JWKS.
  - The developer/test script (host) mints a token from `<issuer>/oauth2/token`.
  Both must use the **same** issuer string (the token's `iss` must match what APISIX
  discovers). Solution: give `lemonldap` a compose **network alias** equal to its
  issuer hostname (e.g. `auth.example.com`, LL::NG's default portal vhost) AND
  document a one-line host `/etc/hosts` entry
  (`127.0.0.1 auth.example.com manager.example.com`). Then
  `OIDC_ISSUER=http://auth.example.com` resolves from both the apisix container and
  the host. (Host port for lemonldap mapped for the manager UI + token endpoint.)
- **The client-id claim (finalized at live-verify):** the portal matches the JWT's
  client-id claim (`OIDC_CLIENT_ID_CLAIM`, default `azp`) against the subscription
  whitelist. LL::NG's client-credentials JWT may carry the id as `sub`/`client_id`
  rather than `azp`. During implementation we mint one token, inspect the claims,
  and set `OIDC_CLIENT_ID_CLAIM` to whatever LL::NG emits. The portal already
  supports a configurable claim (`OIDC_CLIENT_ID_CLAIM` env), so this is a config
  value — no code change. If LL::NG cannot emit a stable client-id claim in the
  client-credentials JWT, the fallback is documented (use `sub`).

## Env wiring (`docker-compose.full.yml` → `portal`)

`PORTAL_ENV=dev`, `UPSTREAM_ALLOW_PRIVATE=1`, `PORTAL_ADDR=:8080`;
`DATABASE_URL=postgres://portal:portal@postgres:5432/portal?sslmode=disable`;
`APISIX_ADMIN_URL=http://apisix:9180`, `APISIX_GATEWAY_URL=http://apisix:9080`
(the gateway is also reached by the host at `:9080` for curl checks);
`APISIX_SANDBOX_ADMIN_URL=http://apisix-sandbox:9180`,
`APISIX_SANDBOX_GATEWAY_URL=http://apisix-sandbox:9080`;
`APISIX_ADMIN_KEY=edd1c9f034335f136f87ad84b625c8f1` (matches the apisix config);
`PROMETHEUS_URL=http://prometheus:9090`;
`SMTP_HOST=mailpit SMTP_PORT=1025 SMTP_FROM=portal@local.test`;
`OIDC_ISSUER=http://auth.example.com` `OIDC_CLIENT_ID_CLAIM=<confirmed>`;
`ADMIN_EMAIL=admin@portal.local`; `PORTAL_BASE_URL=http://localhost:8088` (so email
links point at the web URL).

## Operations

- **`make full`** = `docker compose -f docker-compose.yml -f docker-compose.full.yml
  up -d --build`; **`make full-down`** = the same with `down -v`.
- **Admin bootstrap** (unchanged behavior): register `admin@portal.local` in the
  browser, then `docker compose … restart portal` — the admin role is applied at
  startup (`EnsureAdminRole`). Documented in the runbook's setup suite.
- The **QA runbook** setup section is updated to a one-command path (`make full` +
  the `/etc/hosts` line + the admin restart), and **Suite 7 (OAuth2) becomes
  runnable** (it was flagged "requires OIDC_ISSUER" before).

## Testing / acceptance

Not unit-testable (infra). Acceptance = **live**: `make full` brings all containers
healthy; the web URL loads and every QA-runbook suite is exercisable in-container:
- portal reaches postgres (migrations run), apisix (subscribe→approve→gateway 200),
  sandbox (isolation), prometheus (usage/quota), mailpit (approval emails visible
  at `:8025`);
- **OAuth2 end-to-end**: set a product to oauth2 + register the app's `client_id`;
  mint a client-credentials token from LL::NG; call the gateway with the Bearer JWT
  → **200** for the subscribed client, **403** for a non-subscribed `azp`/id,
  **401** without a token.
The controller runs this end-to-end and **looks at** a minted token's claims + the
gateway responses.

## Out of scope

- Production hardening (TLS, real secrets, resource limits, non-dev `PORTAL_ENV`) —
  this stack is for **local testing**, mirroring the dev posture (built-in dev
  secrets, loopback ports).
- A user-login (authorization-code) OAuth2 flow — the portal's consumer OAuth2 is
  client-credentials only; LL::NG's WebSSO login UI is present but not required for
  the tests.
- Replacing the host-dev loop — `docker-compose.yml` + `make run`/`pnpm dev` stay
  for fast iteration; the full stack is additive.
- CI wiring / publishing images to a registry.
