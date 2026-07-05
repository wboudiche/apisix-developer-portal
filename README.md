# APISIX Developer Portal

A WSO2-style developer portal for Apache APISIX: publish API products, let
developers register apps, subscribe to plans, get approved, and call the
gateway with an API key or an OAuth2 bearer token. Go backend (`cmd/portal`),
React + Vite frontend (`web/`), APISIX as the data-plane gateway.

For day-to-day host development use `make up` (infra only) + `make run` +
`cd web && pnpm dev`. For the test layers (unit / component / E2E) see
[`docs/testing.md`](docs/testing.md).

## Full test stack (one command)

`make full` brings up **everything in containers** — the portal API, the built
web app, all infrastructure, and a **LemonLDAP::NG** OIDC provider — so every
feature (including the OAuth2-for-consumers flow) is exercisable from one URL,
with no host-side Go or Node processes.

### Prerequisites

The OAuth2 issuer must resolve to the same hostname from both the host and
inside the Docker network. Add this line to your host `/etc/hosts` **once**:

```
127.0.0.1   auth.example.com manager.example.com
```

### Run it

```bash
make full                       # docker compose -f docker-compose.yml -f docker-compose.full.yml up -d --build
# open http://localhost:8088
```

First-run **admin bootstrap** (the admin role is granted at portal startup for
`ADMIN_EMAIL=admin@portal.local`):

1. Register `admin@portal.local` at <http://localhost:8088/register>.
2. `docker compose -f docker-compose.yml -f docker-compose.full.yml restart portal`
3. Log back in — the **Admin** menu now appears.

Tear down (removes volumes):

```bash
make full-down                  # ... down -v
```

### Components

| Component | URL | Purpose |
|-----------|-----|---------|
| **Web app** | <http://localhost:8088> | The SPA — the single URL to open. nginx serves the build and proxies `/api` → portal. |
| **Portal API** | <http://localhost:8090/api> | Go backend, for direct API poking (`curl :8090/api/plans`). |
| **Mailpit** | <http://localhost:8025> | **Internal mail** — the portal sends approval-loop emails here; read them in the web inbox. SMTP sink on `:1025`. |
| **LemonLDAP::NG** | <http://localhost:8081> | OIDC provider (issuer `http://auth.example.com`). Manager UI + `/oauth2/token` + JWKS. |
| **APISIX gateway** | <http://localhost:9080> | Data plane. Approved routes are served here (a bare `/` returns 404 — expected). |
| **APISIX sandbox** | <http://localhost:9081> | Isolated try-it gateway. |
| **Prometheus** | <http://localhost:9099> | Scrapes APISIX; backs the usage/quota views. |

### LemonLDAP::NG client-credentials app (pre-seeded)

| Field | Value |
|-------|-------|
| client_id | `apisix-portal-app` |
| client_secret | `apisix-portal-secret` |
| issuer | `http://auth.example.com` |
| grant | `client_credentials` (RS256 JWT access token) |
| client-id claim | `client_id` (portal `OIDC_CLIENT_ID_CLAIM`) |

> The baked config in `deploy/lemonldap/` ships a **test-only** RSA signing key.
> This stack mirrors the dev posture (built-in dev secrets, loopback ports) and
> is **not** production-hardened. See `deploy/lemonldap/README.md`.

### OAuth2 end-to-end (the new capability)

As admin, set a product's auth method to **OAuth2**, register `client_id`
`apisix-portal-app` on an app, and approve a subscription — the portal then
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
curl                                   http://localhost:9080/<contextPath>/...   # 401  no token
#                                        (a valid token whose client_id isn't subscribed → 403)
```

For the full manual walkthrough of every feature (subscribe → approve →
gateway, emails, usage/quota, sandbox isolation, ratings), see
[`docs/testing.md`](docs/testing.md).
