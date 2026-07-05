# Full Test Stack (one-command docker-compose) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** One command (`make full`) brings up the whole portal — Go API + web + all infra + a LemonLDAP::NG OIDC provider — so every feature (including OAuth2) is testable in-browser.

**Architecture:** Two new Dockerfiles (Go API; Vite build behind nginx) + a `docker-compose.full.yml` override layered on the untouched `docker-compose.yml`, adding `portal`, `web`, and `lemonldap`. LemonLDAP::NG carries a baked config enabling a client-credentials OIDC relying party with JWT access tokens + JWKS.

**Tech Stack:** Docker Compose, Go 1.25, Node 22 + pnpm + nginx, LemonLDAP::NG, APISIX 3.9.1.

## Global Constraints

- **This is infra — verification is by bringing services up, not unit tests.** Each task ends by building/running its piece and observing a concrete result.
- Keep `docker-compose.yml` **unchanged**; all additions go in `docker-compose.full.yml` (an override run as `docker compose -f docker-compose.yml -f docker-compose.full.yml …`).
- Match existing values verbatim: APISIX admin key `edd1c9f034335f136f87ad84b625c8f1`; etcd `http://etcd:2379`; DB `postgres://portal:portal@postgres:5432/portal?sslmode=disable`; the portal listens on `:8080`; `pnpm build` = `tsc -b && vite build` → `web/dist`.
- Dev posture only: `PORTAL_ENV=dev`, built-in dev secrets, `UPSTREAM_ALLOW_PRIVATE=1`, loopback host ports. No TLS/prod hardening.
- The portal has **no DB connect-retry** → `depends_on: postgres {condition: service_healthy}`.

---

## Task 1: Portal image (`Dockerfile` + `.dockerignore`)

**Files:** Create `Dockerfile`, `.dockerignore`.

- [ ] **Step 1: Write `.dockerignore`**

```
.git
web/node_modules
web/dist
node_modules
.superpowers
*.md
docs
```

- [ ] **Step 2: Write `Dockerfile`** (multi-stage; migrations are `go:embed`ed, so only the binary ships)

```dockerfile
# syntax=docker/dockerfile:1
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /portal ./cmd/portal

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /portal /portal
EXPOSE 8080
ENTRYPOINT ["/portal"]
```

- [ ] **Step 3: Build it**

Run: `docker build -t apisix-portal-api:test .`
Expected: builds cleanly; `docker run --rm apisix-portal-api:test` exits with a config/DB error (no DB reachable) — that proves the binary starts and reads config (it will `log.Fatal` on the missing DB, which is expected standalone).

- [ ] **Step 4: Commit**

```bash
git add Dockerfile .dockerignore
git commit -m "build: portal Dockerfile (multi-stage, distroless) + dockerignore"
```

---

## Task 2: Web image (`web/Dockerfile` + `web/nginx.conf`)

**Files:** Create `web/Dockerfile`, `web/nginx.conf`.

**Interfaces:** Produces an nginx image serving the SPA on `:80`, proxying `/api/` → `http://portal:8080` (the service name from Task 4).

- [ ] **Step 1: Write `web/nginx.conf`**

```nginx
server {
  listen 80;
  server_name _;
  root /usr/share/nginx/html;
  index index.html;

  location /api/ {
    proxy_pass http://portal:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $remote_addr;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_read_timeout 60s;
  }

  location / {
    try_files $uri $uri/ /index.html;
  }
}
```

- [ ] **Step 2: Write `web/Dockerfile`** (node build via corepack/pnpm → nginx)

```dockerfile
# syntax=docker/dockerfile:1
FROM node:22-alpine AS build
WORKDIR /app
RUN corepack enable
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm build

FROM nginx:1.27-alpine
COPY web/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 80
```

(If `pnpm install --frozen-lockfile` fails on a lockfile mismatch, fall back to `pnpm install`; note it in the report.)

- [ ] **Step 3: Build it**

Run: `docker build -t apisix-portal-web:test -f web/Dockerfile .`
Expected: `pnpm build` runs `tsc -b && vite build`, `dist/` is produced, the nginx image builds. (Proxy behavior is verified live in Task 4.)

- [ ] **Step 4: Commit**

```bash
git add web/Dockerfile web/nginx.conf
git commit -m "build: web Dockerfile (vite build behind nginx, /api proxy)"
```

---

## Task 3: LemonLDAP::NG OIDC service + baked config

**Files:** Create `deploy/lemonldap/lmConf-1.json` (baked config), `deploy/lemonldap/README.md` (how it was produced + the client creds).

**Interfaces:** Produces an OIDC issuer at `http://auth.example.com` with a relying party `client_id=apisix-portal-app`, `client_secret=apisix-portal-secret`, **client-credentials grant + JWT access tokens + JWKS**. Consumed by the portal (Task 4) via `OIDC_ISSUER`.

- [ ] **Step 1: Boot LemonLDAP::NG once to generate a base config**

Run a throwaway container to get a valid default `lmConf-1.json` + RSA keys, then export it:
```bash
docker run -d --name lmtmp -p 19876:80 coudot/lemonldap-ng:2.19
sleep 8
docker cp lmtmp:/var/lib/lemonldap-ng/conf/lmConf-1.json deploy/lemonldap/lmConf-1.json
```
(Exact image tag: use the current `coudot/lemonldap-ng` release; record it in the README.)

- [ ] **Step 2: Enable OIDC + add the relying party in the config**

Edit `deploy/lemonldap/lmConf-1.json` to set the OIDC service on and register the RP. The required keys (LemonLDAP::NG 2.x names):
```jsonc
{
  "issuerDBOpenIDConnectActivation": 1,
  "oidcServiceMetaDataIssuer": "http://auth.example.com",
  // an RSA sig key must exist: oidcServicePrivateKeySig / oidcServicePublicKeySig / oidcServiceKeyIdSig
  "oidcRPMetaDataOptions": {
    "apisix-portal-app": {
      "oidcRPMetaDataOptionsClientID": "apisix-portal-app",
      "oidcRPMetaDataOptionsClientSecret": "apisix-portal-secret",
      "oidcRPMetaDataOptionsClientCredentials": 1,
      "oidcRPMetaDataOptionsAccessTokenJWT": 1,
      "oidcRPMetaDataOptionsBypassConsent": 1,
      "oidcRPMetaDataOptionsIDTokenSignAlg": "RS256",
      "oidcRPMetaDataOptionsAccessTokenSignAlg": "RS256"
    }
  },
  "oidcRPMetaDataExportedVars": { "apisix-portal-app": {} },
  "oidcRPMetaDataScopes": { "apisix-portal-app": {} }
}
```
Keep every other key from the exported default (LL::NG rejects a partial config). If hand-editing proves fragile, an accepted alternative: configure the RP via the Manager UI at `http://manager.example.com` on the throwaway container, then re-export `lmConf-1.json` — document whichever path in the README.

- [ ] **Step 3: Verify OIDC discovery, JWKS, and a client-credentials token**

Add `127.0.0.1 auth.example.com manager.example.com` to `/etc/hosts`, then against the throwaway container (published on `:19876`, but discovery uses the issuer host):
```bash
docker rm -f lmtmp
docker run -d --name lmtmp -p 80:80 \
  -v "$PWD/deploy/lemonldap/lmConf-1.json:/var/lib/lemonldap-ng/conf/lmConf-1.json:ro" \
  coudot/lemonldap-ng:2.19
sleep 8
curl -s http://auth.example.com/.well-known/openid-configuration | head
curl -s http://auth.example.com/oauth2/jwks | head
curl -s -u apisix-portal-app:apisix-portal-secret \
  -d grant_type=client_credentials http://auth.example.com/oauth2/token
```
Expected: discovery JSON with `issuer=http://auth.example.com` + a `jwks_uri`; JWKS returns keys; the token call returns an `access_token`. **Decode the access token** (`cut -d. -f2 | base64 -d`) and record which claim carries `apisix-portal-app` (likely `sub` or `client_id` or `aud`) — this becomes `OIDC_CLIENT_ID_CLAIM` in Task 4. `docker rm -f lmtmp`.

- [ ] **Step 4: Commit**

```bash
git add deploy/lemonldap/
git commit -m "test-stack: LemonLDAP::NG baked config (client-credentials OIDC + JWKS)"
```

---

## Task 4: `docker-compose.full.yml` + env wiring + make targets

**Files:** Create `docker-compose.full.yml`; modify `Makefile`.

**Interfaces:** Consumes the images (Tasks 1–2) + LL::NG config (Task 3) + the confirmed `OIDC_CLIENT_ID_CLAIM`.

- [ ] **Step 1: Write `docker-compose.full.yml`**

```yaml
name: apisix-portal
services:
  lemonldap:
    image: coudot/lemonldap-ng:2.19
    volumes:
      - ./deploy/lemonldap/lmConf-1.json:/var/lib/lemonldap-ng/conf/lmConf-1.json:ro
    networks:
      default:
        aliases: [auth.example.com, manager.example.com]
    ports:
      - "127.0.0.1:8081:80"   # manager UI + token endpoint from the host

  portal:
    build: { context: ., dockerfile: Dockerfile }
    depends_on:
      postgres: { condition: service_healthy }
      apisix: { condition: service_started }
      lemonldap: { condition: service_started }
    environment:
      PORTAL_ENV: dev
      UPSTREAM_ALLOW_PRIVATE: "1"
      PORTAL_ADDR: ":8080"
      DATABASE_URL: postgres://portal:portal@postgres:5432/portal?sslmode=disable
      APISIX_ADMIN_URL: http://apisix:9180
      APISIX_GATEWAY_URL: http://apisix:9080
      APISIX_SANDBOX_ADMIN_URL: http://apisix-sandbox:9180
      APISIX_SANDBOX_GATEWAY_URL: http://apisix-sandbox:9080
      APISIX_ADMIN_KEY: edd1c9f034335f136f87ad84b625c8f1
      PROMETHEUS_URL: http://prometheus:9090
      SMTP_HOST: mailpit
      SMTP_PORT: "1025"
      SMTP_FROM: portal@local.test
      OIDC_ISSUER: http://auth.example.com
      OIDC_CLIENT_ID_CLAIM: CONFIRMED_IN_TASK_3   # sub | client_id | azp — from the decoded token
      ADMIN_EMAIL: admin@portal.local
      PORTAL_BASE_URL: http://localhost:8088
    ports:
      - "127.0.0.1:8090:8080"  # direct API access for curl checks

  web:
    build: { context: ., dockerfile: web/Dockerfile }
    depends_on: [portal]
    ports:
      - "8088:80"
```

(The base file already defines postgres/etcd/apisix/apisix-sandbox/echo/mailpit/prometheus; the override merges by `name: apisix-portal`. `lemonldap` needs the network alias so APISIX resolves `auth.example.com` in-network.)

- [ ] **Step 2: Set `OIDC_CLIENT_ID_CLAIM`** to the claim confirmed in Task 3 Step 3 (replace the placeholder).

- [ ] **Step 3: Add Make targets** to `Makefile`:

```make
full:        ; docker compose -f docker-compose.yml -f docker-compose.full.yml up -d --build
full-down:   ; docker compose -f docker-compose.yml -f docker-compose.full.yml down -v
```
Add `full full-down` to the `.PHONY` line.

- [ ] **Step 4: Bring the whole stack up**

Run: `make full` then `docker compose -f docker-compose.yml -f docker-compose.full.yml ps`
Expected: all services up; `curl -s http://localhost:8088/ | grep -i apisix` shows the SPA; `curl -s http://localhost:8088/api/plans` returns JSON (proves the web→portal `/api` proxy + portal→postgres). Portal logs show migrations applied.

- [ ] **Step 5: Commit**

```bash
git add docker-compose.full.yml Makefile
git commit -m "test-stack: docker-compose.full.yml (portal+web+lemonldap) + make full"
```

---

## Task 5: End-to-end live verification + docs

**Files:** Modify `README.md` (add a "Full test stack" section). Verification only — no code beyond docs.

- [ ] **Step 1: Admin bootstrap**

Register `admin@portal.local` at `http://localhost:8088/register`, then
`docker compose -f docker-compose.yml -f docker-compose.full.yml restart portal`. Log in → the Admin link appears.

- [ ] **Step 2: Smoke every subsystem in-container**

- Subscribe→approve→gateway: create an app, subscribe, approve in `/admin/approvals`, then `curl -H "apikey: <key>" http://localhost:9080/<contextPath>/…` → 200 (needs a product with a real upstream, e.g. `echo:8080`).
- Emails: trigger the approval loop, open Mailpit at `http://localhost:8025`.
- Usage: generate gateway traffic, check the app’s Usage tab.

- [ ] **Step 3: OAuth2 end-to-end (the new capability)**

- As admin, set a product’s auth method to **oauth2**; on the app, register `client_id=apisix-portal-app`; approve a subscription.
- Mint a token: `curl -u apisix-portal-app:apisix-portal-secret -d grant_type=client_credentials http://auth.example.com/oauth2/token`.
- Call the gateway: `curl -H "Authorization: Bearer <jwt>" http://localhost:9080/<contextPath>/…`.
Expected: **200** for the subscribed client; **403** for a token whose client-id claim isn’t subscribed; **401** with no token. If the client-id claim differs from what was set in Task 4, correct `OIDC_CLIENT_ID_CLAIM` and `make full` again.

- [ ] **Step 4: Write the docs**

Add a "Full test stack" section to `README.md`: the `/etc/hosts` line (`127.0.0.1 auth.example.com manager.example.com`), `make full`, open `http://localhost:8088`, the admin-restart step, `make full-down`, and the LL::NG client creds. Include a **component/URL table** so each piece is obvious — web `:8088`, portal API `:8090`, **Mailpit (internal mail — the portal sends approval-loop emails here; read them at `:8025`)**, LemonLDAP::NG `:8081`, APISIX gateway `:9080` / sandbox `:9081`, Prometheus `:9099`. Cross-reference the QA runbook.

- [ ] **Step 5: Commit**

```bash
git add README.md
git commit -m "docs: one-command full test stack (make full) + OAuth2 setup"
```

---

## Self-Review notes

- **Spec coverage:** compose override (T4) ✅; portal Dockerfile (T1) ✅; web Dockerfile+nginx `/api` proxy (T2) ✅; LemonLDAP::NG baked client-credentials OIDC + JWKS + issuer alias + `/etc/hosts` (T3/T4) ✅; the client-id-claim caveat resolved by decoding a real token (T3→T4) ✅; env wiring (T4) ✅; `make full`/`full-down` + admin restart + docs (T4/T5) ✅; live acceptance incl OAuth2 (T5) ✅.
- **Type/name consistency:** service names `portal`/`web`/`lemonldap`; issuer `http://auth.example.com`; client `apisix-portal-app`/`apisix-portal-secret`; the web proxy targets `http://portal:8080`; ports 8088 (web), 8090 (portal API), 8081 (LL::NG) — used consistently across tasks.
- **Notes for the implementer:** LemonLDAP::NG is the real risk — Task 3 is exploratory (boot → configure → export → decode a token). If its client-credentials JWT can’t carry a stable client-id claim after reasonable effort, STOP and escalate (the spec’s fallback is a lighter OIDC provider); do not fake it. The base `docker-compose.yml` stays untouched — only add the override + Makefile lines + Dockerfiles + `deploy/lemonldap/`.
```
