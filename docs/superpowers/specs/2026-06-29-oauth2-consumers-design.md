# OAuth2 for Consumers — Design

**Date:** 2026-06-29
**Status:** Approved, ready for planning
**Surface:** `internal/config`, `internal/apisix` (new OAuth2 route composition), `internal/admin` (product `auth_type`), `internal/applications`/`internal/subscriptions` (app `oidc_client_id` + provisioning branch), `internal/catalog` (expose `auth_type`); frontend admin Composer + Credentials tab + catalog/detail badge.

## Problem

The portal authenticates API consumers with APISIX **key-auth** only. Many
organizations standardize on OAuth2/OIDC and want to call portal APIs with
bearer tokens from their existing identity provider. This adds **OAuth2
(client-credentials grant) against a bring-your-own OIDC provider** as a second,
per-product auth method, while preserving the portal's subscription gate as the
source of truth for authorization.

## Locked decisions (from brainstorming)

- **Model:** client-credentials grant validated against a **generic / bring-your-own
  OIDC provider** (no bundled IdP). APISIX validates bearer JWTs with the
  `openid-connect` plugin against the configured issuer's JWKS.
- **Client provisioning:** **manual / BYO** — the developer registers the client
  in their own IdP and enters its `client_id` into the portal. **The portal never
  sees or stores the client secret.** Tokens are obtained by the developer
  directly from their IdP; the portal does not issue or proxy tokens.
- **Authorization:** **portal-managed `client_id` whitelist on the route** — the
  gateway validates the token AND restricts to the `client_id`s of the product's
  active subscribers. Subscription stays the portal's source of truth (mirrors
  the existing `consumer-restriction` model). NOT IdP-managed scopes; NOT
  validate-only.
- **Auth method scope:** **per-product** — the admin marks each product
  `key-auth` (default, unchanged) or `oauth2`; all subscribers use that method.
  A route runs one auth scheme.
- **Enforcement mechanism (APISIX 3.9.1):** `openid-connect` (bearer_only,
  JWKS) for validation + a **`serverless-pre-function`** (access phase) that
  enforces the client_id whitelist. APISIX 3.9.1 has no stock plugin to whitelist
  a dynamic set of claim values (`claim_validator` is 3.14+ and only matches
  `aud == the route's own client_id`), so a small generated Lua check is required.

## Architecture overview

A product's admin chooses `auth_type`. For an **`oauth2`** product, its APISIX
route validates bearer JWTs via `openid-connect` and a `serverless-pre-function`
restricts to the **client_ids of the product's active subscribers whose app has a
non-empty `oidc_client_id`**. Each app carries one `oidc_client_id` (the app = one
OIDC client). No APISIX consumer or key is created for OAuth2 apps. The portal
regenerates the route's allow-list on every subscription change — the OAuth2
analogue of today's key-auth `consumer-restriction` whitelist.

**Core invariant (OAuth2-route whitelist rule):** an `oauth2` product's route
admits exactly the `client_id`s of its **active** subscribers whose app has a
non-empty `oidc_client_id`. Every lifecycle hook maintains this.

## Config — `internal/config`

- `OIDC_ISSUER` (e.g. `https://idp.example.com/realms/dev`) — the trusted issuer.
  Its `{issuer}/.well-known/openid-configuration` → JWKS drives `openid-connect`
  validation.
- `OIDC_CLIENT_ID_CLAIM` (default `azp`) — the JWT claim carrying the caller's
  client id (providers vary: `azp`, `client_id`, `clientId`).
- `func (c Config) OIDCConfigured() bool` → `OIDC_ISSUER != ""`. When false, the
  admin cannot set `auth_type='oauth2'` (400) and OAuth2 UI surfaces are inert
  (mirrors `SandboxConfigured()`). The dev-secrets guard is unchanged (no new
  secret — the portal holds no client secret).

## Data model — migration `0012_oauth2.sql`

- `api_products.auth_type TEXT NOT NULL DEFAULT 'key-auth'` — `'key-auth'` |
  `'oauth2'` (a CHECK constraint enforces the two values).
- `applications.oidc_client_id TEXT NOT NULL DEFAULT ''` — the app's OIDC client
  id (plaintext; not a secret). `''` = not set.

## Backend — gateway

### Route composition (`internal/apisix`)

New `Gateway` method (the existing `EnsureRoute`/`EnsureConsumer`/`DeleteRoute`
are unchanged and still serve key-auth products):

```
EnsureOAuthRoute(ctx, routeID, contextPath, upstreamURL, issuer, claimName string, allowedClientIDs []string) error
```

It builds a route with:
- **`openid-connect`**: `bearer_only: true` + `discovery: "{issuer}/.well-known/openid-configuration"` so the gateway fetches the JWKS and validates the token's signature/issuer/expiry (reject missing/invalid → 401); `ssl_verify` defaults true (an env override allows a self-signed test issuer in dev). The exact attribute names are confirmed against the APISIX 3.9.1 `openid-connect` schema as the first implementation step (Task 1 verifies a hand-minted JWT validates before the rest is built on it).
- **`serverless-pre-function`** (`phase: "access"`): generated Lua that reads the
  validated token's `claimName` claim and returns `403` unless it is a key in an
  allow-list table. The allow-list is built from `allowedClientIDs`.
- Same context-path/upstream/prefix-strip behavior as `EnsureRoute`.

**Injection safety (mandatory):** `allowedClientIDs` are inserted into the Lua as
**table keys built from validated data**, never string-concatenated into code.
Every `client_id` is validated at the portal boundary against a strict charset
(e.g. `^[A-Za-z0-9._:-]{1,200}$`); anything else is rejected before it can reach
the gateway. A unit test asserts a hostile client_id is refused.

`DeleteRoute` is reused (delete when the whitelist is empty, as today).

### Repo + Service (`internal/subscriptions`)

- `ProductInfo` gains `AuthType string`. `GetProduct` selects `auth_type`.
- New `OAuthClientsForProduct(ctx, productID) ([]string, error)` — `client_id`s of
  active subscribers whose app has a non-empty `oidc_client_id` (the OAuth2
  analogue of `ConsumersForProduct`).
- New `OAuthProductsForApp(ctx, appID) ([]ProductInfo, error)` — `oauth2` products
  the app is actively subscribed to (id, context_path, upstream) — used to rebuild
  routes when the app's `oidc_client_id` changes.
- `reprovisionRoute` **branches on `prod.AuthType`**: `key-auth` → today's
  consumer + `consumer-restriction` route (`EnsureRoute`); `oauth2` →
  `EnsureOAuthRoute` with `OAuthClientsForProduct` (delete when empty). The service
  holds the configured `oidcIssuer` + `oidcClaim` (passed from config) to fill
  `EnsureOAuthRoute`.
- New `SetOIDCClientID(ctx, appID, clientID string) error` — validates charset,
  persists `applications.oidc_client_id`, then reprovisions each product in
  `OAuthProductsForApp` (so the new client_id enters/updates the whitelists).

### Lifecycle (branches on `auth_type`)

- **Subscribe:** key-auth → `GetOrCreateCredential` (issue the key, today). **oauth2
  → no credential/key issued.** The pending subscription is recorded as usual.
- **Approve / Reject / Unsubscribe:** the reprovision step branches as above; for
  oauth2 it rebuilds the OAuth2 route whitelist (an app with no `oidc_client_id`
  is simply absent — no-op).
- **App `oidc_client_id` set/changed:** `SetOIDCClientID` reprovisions the app's
  oauth2 product routes.
- **Admin switches a product's `auth_type`:** delete the existing route, then
  provision under the new scheme (key-auth → consumer-restriction route; oauth2 →
  OAuth route). Existing subscriptions are preserved; key-auth consumers left
  on the gateway are harmless (no route references them).

## Backend — HTTP endpoints

- **Admin product create/update** payload gains `authType` (∈ `key-auth` |
  `oauth2`; default `key-auth`). Reject `oauth2` with **400** when
  `!OIDCConfigured`. Changing it reprovisions the product route. (`internal/admin`.)
- **`PUT /api/applications/{id}/oidc-client`** (owner-scoped, behind requireAuth)
  `{ "clientId": "..." }` → 200; **400** on invalid charset; clears when empty.
  (`internal/subscriptions` handler, beside the credentials/sandbox routes.)
- **`GET /api/applications/{id}`** gains `oidcClientId string`,
  `oauthEligible bool` (app has ≥1 active oauth2 subscription), and
  `oidcIssuer string` (from config, `""` when unconfigured — for the "request your
  token here" hint). Per-subscription `authType` is exposed via the subscriptions
  list.
- **Catalog** (`GET /api/products*`) exposes `authType` so the UI can badge OAuth2
  products. (Read-only; `internal/catalog` `baseSelect` + `Product.AuthType`.)

## Frontend

- **Admin Composer** (`web/src/pages/admin/ProductsPage.tsx`): an "Auth method"
  selector (Key-auth / OAuth2), shown only when OIDC is configured; a note that
  OAuth2 routes validate bearer tokens against the configured issuer. Flows
  through create + update; edit prefills.
- **Credentials tab** (`web/src/pages/application/CredentialsTab.tsx`): alongside
  the key-auth Production card, an **OAuth2 card** shown when `oauthEligible` — a
  `client_id` text input (save → `PUT …/oidc-client`), the **issuer** to request
  tokens from, and a `grant_type=client_credentials` reminder. The portal never
  asks for the secret.
- **Catalog/detail:** an "OAuth2" badge on `authType==='oauth2'` products
  (`ApiCard` + `ProductDetailPage`); the detail page explains the
  client-credentials flow rather than showing a key.
- **Types/client:** `Product.authType?`, `AdminProduct.authType?`,
  `AppDetail.oidcClientId?`/`oauthEligible?`/`oidcIssuer?`; `setOidcClient(token,
  appId, clientId)`. New optional fields default safely (no fixture churn),
  consistent with the sandbox feature's approach.

## Testing

### Backend (Go, Fake gateway)

- `EnsureOAuthRoute` emits `openid-connect` (bearer_only + discovery from issuer)
  + `serverless-pre-function`, with client_ids as an escaped table; a **hostile
  client_id is rejected** at the validation boundary (injection guard).
- `OAuthClientsForProduct` returns active subscribers with a non-empty
  `oidc_client_id` only (excludes pending/rejected and clientless apps).
- `reprovisionRoute` branches correctly on `auth_type` (key-auth route vs OAuth
  route); Approve/Reject/Unsubscribe rebuild the OAuth whitelist; `SetOIDCClientID`
  reprovisions the app's oauth2 routes; an `auth_type` switch tears down + rebuilds.
- Subscribe to an oauth2 product issues **no** credential/key.
- Admin `authType` validation: accepted values; **400** for `oauth2` when OIDC
  unconfigured.

### Frontend (vitest)

- Admin auth-method selector (present when OIDC configured; flows into
  create/update).
- OAuth2 credentials card: shown when `oauthEligible`, client_id save calls
  `setOidcClient`, issuer displayed.
- `authType==='oauth2'` badge on card/detail.
- Client fns/types hit the right URLs.

### Live (controller)

Stand up a **disposable test OIDC issuer** (a minimal JWKS endpoint + a
hand-minted client-credentials JWT whose `azp` is the test client_id — *not*
bundled in the product; BYO stays the model). Point `OIDC_ISSUER` at it. Mark a
product `oauth2`; set an app's `client_id`; subscribe + approve → call the gateway
with a token whose `azp` = that client_id → **200**; a token with a different
`azp` → **403**; missing/invalid token → **401**. Switch the product back to
key-auth → the OAuth route is replaced.

## Out of scope (deferred)

- **Per-app OAuth2 rate-limiting** — `limit-count` is consumer-keyed; OAuth2 has no
  consumer, so per-app limits need claim-keyed `limit-count` (added complexity).
  Key-auth keeps its plan limits; OAuth2 rate-limiting is a fast follow.
- **Interactive Try-it for OAuth2 products** — the portal can't fetch a token
  without the secret. Docs still render; a "paste your bearer token" Try-it is a
  fast follow.
- **Portal-issued tokens / Dynamic Client Registration (RFC 7591)** — explicitly
  not this V1 (BYO/manual was chosen).
- **Bundling an IdP** — generic/BYO only.

## Implementation note

This will be split into **two plans** (backend + infra, then frontend), like the
sandbox feature, each producing independently testable software.
