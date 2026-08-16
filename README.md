# APISIX Developer Portal

**The self-service front door to your APIs.** Apache APISIX routes and secures
traffic, but it has no place for the people who *consume* your APIs. This portal
is that place: a catalog developers can browse, a sign-up flow that issues them
credentials, and an approval queue that keeps you in control of who gets access.

Go backend (`cmd/portal`), React + Vite frontend (`web/`), PostgreSQL as the
source of truth, and APISIX as the data-plane gateway it configures downstream.

---

## What problem it solves

Without a portal, onboarding an API consumer is a manual chore: someone files a
ticket, an operator hand-writes an APISIX route, generates a key, mails it over,
and remembers to revoke it later. That doesn't scale past a handful of
consumers, and nobody can see who is calling what.

The portal turns that into a self-service loop with an approval gate:

```
developer discovers an API  →  creates an app  →  subscribes to a plan
                                                         ↓
                                                  (pending approval)
                                                         ↓
   calls the gateway with a key  ←  APISIX route provisioned  ←  admin approves
```

Nothing reaches the gateway until an admin says yes. Subscribing issues the
credential immediately, but it grants no access until approval — so the catalog
can stay open while access stays governed.

### Who it's for

| Role | What they do in the portal |
|------|----------------------------|
| **Developers** (API consumers) | Browse the catalog, read docs, try endpoints in the browser, create applications, subscribe to plans, collect and rotate API keys, watch their quota. |
| **Admins** (API providers) | Publish API products (or import an OpenAPI spec), set plans and pricing, approve or reject subscriptions, issue invoices, and tune every runtime setting from the UI. |

### Core concepts

| Term | Meaning |
|------|---------|
| **API product** | A published API in the catalog — context path, upstream, auth method, plans, docs, lifecycle status. |
| **Application** | A developer's app. Holds the credentials (API key, optionally an OAuth2 client id) and the subscriptions. |
| **Subscription** | Links one application to one product on a chosen plan. Starts `pending`, becomes `active` once an admin approves. |
| **Plan** | The tier a subscription runs on: a rate limit (requests per window) and an optional recurring price. |
| **Gateway** | Apache APISIX. Approved subscriptions are provisioned here; consumers call their API through it. |
| **Sandbox** | A separate, isolated gateway for safe experimentation, with its own key. |

---

## Run it

One command brings up **everything in containers** — the portal API, the built
web app, PostgreSQL, APISIX (production + sandbox), Prometheus, a Mailpit inbox,
and a **LemonLDAP::NG** OIDC provider — so every feature, including the OAuth2
flow, works from a single URL with no host-side Go or Node processes.

```bash
make full          # docker compose -f docker-compose.yml -f docker-compose.full.yml up -d --build
```

Then open **<http://localhost:8088>**.

No prerequisites. In-network issuer resolution (APISIX → LemonLDAP) is handled
by compose **network aliases** (`auth.example.com` / `manager.example.com`), and
the host-side OAuth2 commands below reach LemonLDAP at `localhost:8081` with an
explicit `Host:` header — so no `/etc/hosts` change is required.

> Optional: to browse the LemonLDAP portal/manager by name you could add
> `127.0.0.1 auth.example.com manager.example.com` to `/etc/hosts`, but that maps
> to port **80** while LemonLDAP is published on **8081** — the `Host:`-header
> commands below are the supported path.

Tear down (removes volumes):

```bash
make full-down     # ... down -v
```

### Become an admin

The admin role is granted at portal startup to whoever owns `ADMIN_EMAIL`
(default `admin@portal.local`):

1. Register `admin@portal.local` at <http://localhost:8088/register>.
2. `docker compose -f docker-compose.yml -f docker-compose.full.yml restart portal`
3. Log back in — the **Admin** menu now appears.

Changing `ADMIN_EMAIL` itself (e.g. via **Admin → Paramètres**) applies live —
no restart needed for that. It's only the *first* registration against the
still-unchanged default that needs one: promotion is a plain `UPDATE ... WHERE
email=$1`, a no-op until that row exists, and nothing re-runs it at
registration time.

---

## Use it

A five-minute tour of the loop. The full task-by-task walkthrough of every
feature is in **[`docs/user-guide.md`](docs/user-guide.md)**.

### As an admin — publish an API

1. **Admin → Produits → New**. Give it a name, a **context path** (the prefix
   consumers will call, e.g. `/pizzashack`), and an **upstream** (where the real
   backend lives). Or use **Import OpenAPI** to fill it from a spec.
2. Pick an **auth method** — `key-auth` (API key) or `oauth2` (bearer JWT).
3. Attach **plans** under **Admin → Plans** — each sets a rate limit and an
   optional price.
4. Publish. It's now in the catalog.

### As a developer — consume an API

1. **Browse** the catalog on the home page; search with `/`, filter by category
   or tag.
2. **Open an API** to read its docs and **try endpoints live** in the browser —
   against **Production** or the isolated **Sandbox**, your choice.
3. **Create an application** under **Applications** — this is what holds your
   credentials.
4. **Subscribe** the app to the API on a plan. Status: `pending`.
5. **An admin approves** (**Admin → Abonnements**) — you get an email, and the
   APISIX route is provisioned for you.
6. **Call the gateway** with your key:

   ```bash
   curl -H 'apikey: <your-key>' http://localhost:9080/<contextPath>/...
   ```

7. **Watch your usage** and quota on the application page; **rotate** the key
   whenever you need to.

### Runtime settings — no restart needed

Every parameter is visible, and almost every one editable, live at
**Admin → Paramètres**. Env vars seed the defaults; values saved in the UI live
in PostgreSQL and win over env (each row shows its source and a reset-to-env
action). Secrets are write-only, and APISIX/SMTP changes are health-probed
before they apply.

Only the boot-critical parameters stay env-only: `DATABASE_URL`, `PORTAL_ADDR`,
`PORTAL_ENV`, `JWT_SECRET`, `CREDENTIAL_ENC_KEY`.

---

## How it works

The portal **owns its data** — products, applications, subscriptions, plans and
invoices live in PostgreSQL — and **provisions APISIX downstream** through its
Admin API. APISIX is never the source of truth; it's the enforcement point.

- **key-auth products** — each application becomes an APISIX **consumer** with
  an API key; each product becomes a **route** whose consumer allow-list is
  exactly its set of active subscribers. Rate limiting is per-consumer, per plan.
- **oauth2 products** — the route validates the bearer JWT against the identity
  provider's **JWKS**, and a serverless function enforces the subscriber
  client-id allow-list (200 subscribed / 403 not subscribed / 401 no token).
- **Usage and quota** come from **Prometheus** scraping the gateway.
- **Emails** (the approval loop) go out over SMTP; in the dev stack they land in
  **Mailpit**.

### Components

| Component | URL | Purpose |
|-----------|-----|---------|
| **Web app** | <http://localhost:8088> | The SPA — the single URL to open. nginx serves the build and proxies `/api` → portal. |
| **Portal API** | <http://localhost:8090/api> | Go backend, for direct API poking (`curl :8090/api/plans`). |
| **Mailpit** | <http://localhost:8025> | Internal mail — the portal sends approval-loop emails here; read them in the web inbox. SMTP sink on `:1025`. |
| **LemonLDAP::NG** | <http://localhost:8081> | OIDC provider (issuer `http://auth.example.com`). Manager UI + `/oauth2/token` + JWKS. |
| **APISIX gateway** | <http://localhost:9080> | Data plane. Approved routes are served here (a bare `/` returns 404 — expected). |
| **APISIX sandbox** | <http://localhost:9081> | Isolated try-it gateway. |
| **Prometheus** | <http://localhost:9099> | Scrapes APISIX; backs the usage/quota views. |

---

## Email verification

Setting `REQUIRE_EMAIL_VERIFICATION=1` (with SMTP configured, otherwise the
portal refuses to start) forces new registrations to confirm their address via
a link before they can log in; in the dev stack that email lands in Mailpit.

> Ops note for internet-facing deploys: verification/resend mail is rate-limited
> per email address and per client IP, but one IP can still trigger mail to many
> *distinct* addresses within the shared IP budget — monitor outbound-mail volume
> at the relay if abuse matters to you.

---

## OAuth2 end to end

Two relying parties are pre-seeded in LemonLDAP::NG, so you can test one API per
client and watch a token for one client get rejected on another:

| client_id | client_secret |
|-----------|---------------|
| `apisix-portal-app` | `apisix-portal-secret` |
| `apisix-portal-app2` | `apisix-portal-secret2` |

| Field | Value |
|-------|-------|
| issuer | `http://auth.example.com` |
| grant | `client_credentials` (RS256 JWT access token) |
| client-id claim | `client_id` (portal `OIDC_CLIENT_ID_CLAIM`) |

As admin, set a product's auth method to **OAuth2**, register client id
`apisix-portal-app` on an app, and approve a subscription. The portal then
configures an APISIX route that validates the bearer JWT against LemonLDAP's
JWKS (`openid-connect`, bearer-only) and 403s any `client_id` not on the
subscriber allow-list. Verify it directly:

```bash
# Mint a client-credentials JWT (Host header selects LL::NG's issuer vhost)
JWT=$(curl -s -H 'Host: auth.example.com' \
  -u apisix-portal-app:apisix-portal-secret \
  -d 'grant_type=client_credentials&scope=openid' \
  http://localhost:8081/oauth2/token | python3 -c 'import sys,json;print(json.load(sys.stdin)["access_token"])')

# Call the gateway on the subscribed product's context path:
curl -H "Authorization: Bearer $JWT" http://localhost:9080/<contextPath>/...   # 200  subscribed client
curl                                 http://localhost:9080/<contextPath>/...   # 401  no token
#                                      (a valid token whose client_id isn't subscribed → 403)
```

> The baked config in `deploy/lemonldap/` ships a **test-only** RSA signing key.
> This stack mirrors the dev posture (built-in dev secrets, loopback ports) and
> is **not** production-hardened. See `deploy/lemonldap/README.md`.

---

## Develop on it

For day-to-day work, run the infrastructure in containers and the two app
processes on the host, so both hot-reload:

```bash
make up                  # infra only: Postgres, APISIX ×2, Prometheus, Mailpit
make run                 # Go API on :8080
cd web && pnpm dev       # Vite on :5173, proxying /api → :8080
```

Open <http://localhost:5173>. Override the proxy target with `PORTAL_PROXY` if
the API runs elsewhere (e.g. `PORTAL_PROXY=http://localhost:8090`).

`make run` sets `PORTAL_ENV=dev` (unset is treated as production, which refuses
the built-in dev secrets) and `UPSTREAM_ALLOW_PRIVATE=1` (lets products target
docker-internal upstreams like `echo:8080`, blocked by the SSRF guard
otherwise).

### Tests

For the full test layering — unit, component, and E2E — see
**[`docs/testing.md`](docs/testing.md)**.

```bash
make test           # Go unit tests
make test-e2e       # E2E against real DB + real APISIX
make test-e2e-web   # Playwright suite (isolated Postgres + API + Vite)
```

With `make full` up, the `internal/e2e` suite runs the real portal handler
against the live containers:

```bash
PORTAL_ENV=dev UPSTREAM_ALLOW_PRIVATE=1 RUN_E2E=1 \
  OIDC_ISSUER=http://auth.example.com OIDC_CLIENT_ID_CLAIM=client_id \
  go test ./internal/e2e/...
```

`TestOAuth2TwoClients` drives the OAuth2 path end to end: it publishes two
`authType=oauth2` APIs (one per pre-seeded client), then asserts each client's
token gets **200** on its own API, **403** on the other (valid signature, but
its `client_id` isn't on that route's allow-list), and **401** with no token.
The `OIDC_*` vars are needed only for that test — omit them and it skips; the
key-auth suites still run.

---

## Documentation

| Document | What's in it |
|----------|--------------|
| [`docs/user-guide.md`](docs/user-guide.md) | Task-by-task guide to every feature, developer and admin. |
| [`docs/testing.md`](docs/testing.md) | The test layers and how to run each. |
| [`docs/security-review.md`](docs/security-review.md) | Security posture and review notes. |
