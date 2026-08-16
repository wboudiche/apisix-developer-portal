# APISIX Developer Portal — User Guide

A complete, task-oriented guide to every feature of the portal, for the two kinds
of users:

- **Developers** — discover APIs, subscribe, get credentials, call the gateway.
- **Admins** — publish and manage API products, plans, approvals, and invoices.

The interface is bilingual: a **FR / EN** toggle sits in the top bar (this guide
uses the English labels). Everything below describes the portal UI; where a
feature also has a direct API or gateway call, it's shown as `curl`.

## Contents

- [Concepts at a glance](#concepts-at-a-glance)
- [Getting started](#getting-started)
- **Developer guide**
  - [1. Discover APIs](#1-discover-apis)
  - [2. Explore an API](#2-explore-an-api)
  - [3. Create an application](#3-create-an-application)
  - [4. Subscribe to an API](#4-subscribe-to-an-api)
  - [5. Get and use your API key](#5-get-and-use-your-api-key)
  - [6. Rotate keys](#6-rotate-keys)
  - [7. OAuth2 applications](#7-oauth2-applications)
  - [8. Try an API in the browser](#8-try-an-api-in-the-browser)
  - [9. Monitor usage and quota](#9-monitor-usage-and-quota)
  - [10. Rate and review an API](#10-rate-and-review-an-api)
  - [11. Teams](#11-teams)
  - [12. Billing (developer view)](#12-billing-developer-view)
  - [13. Profile and language](#13-profile-and-language)
- **Admin guide**
  - [14. Becoming an admin](#14-becoming-an-admin)
  - [15. Publish an API product](#15-publish-an-api-product)
  - [16. Import from an OpenAPI spec](#16-import-from-an-openapi-spec)
  - [17. Choose or upload an icon](#17-choose-or-upload-an-icon)
  - [18. Changelog and lifecycle](#18-changelog-and-lifecycle)
  - [19. Manage plans and pricing](#19-manage-plans-and-pricing)
  - [20. Approve or reject subscriptions](#20-approve-or-reject-subscriptions)
  - [21. Manage invoices](#21-manage-invoices)
  - [22. Runtime settings](#22-runtime-settings)
- [How it works under the hood](#how-it-works-under-the-hood)
- [Reference](#reference)

---

## Concepts at a glance

| Term | Meaning |
|------|---------|
| **API product** | A published API in the catalog — has a context path, an upstream, an auth method, plans, docs, and a lifecycle status. |
| **Application** | A developer's app that holds credentials (an API key, and optionally an OAuth2 client id) and subscriptions. |
| **Subscription** | A link between one application and one product on a chosen **plan**. Starts **pending**, becomes **active** after an admin approves. |
| **Plan** | The tier a subscription runs on: a rate limit (requests per window) and an optional recurring price. |
| **Gateway** | Apache APISIX. Approved subscriptions are provisioned here; you call your API through it (default `http://localhost:9080`). |
| **Sandbox** | A separate try-it gateway (`http://localhost:9081`) for safe experimentation. |

**The core loop:** discover an API → create an app → subscribe (pending) →
admin approves → your key works at the gateway.

---

## Getting started

1. Open the portal at **`http://localhost:8088`** (the single URL when running the
   full stack — see the project README's "Full test stack") or wherever it's
   deployed.
2. Click **Log in** (top-right) → **Register** to create a local account
   (email + password, min 8 characters). You're signed in immediately —
   *unless* the deployment requires email verification (below).
3. Use the **FR / EN** toggle in the top bar to switch language at any time; your
   choice is remembered and syncs to your account.

**If email verification is required** (an admin turned on
`REQUIRE_EMAIL_VERIFICATION`, §22): registering shows a **"Check your inbox"**
screen instead of signing you in. Click the link in the email to verify and
then log in normally. An expired or already-used link shows an error page
with a field to resend a fresh one.

The top navigation shows **APIs** always, and **Applications**, **Teams**, and
**Billing** once you're signed in. Admins additionally see **Admin**.

---

## 1. Discover APIs

The home page (**APIs**) is the catalog.

- **Search** — type in the top-bar search box (press `/` to focus it) to match
  by name, description, or tags.
- **Filter** — the left sidebar lists **categories** (with counts) and **tags**;
  click to filter. Click again to clear.
- **Sort** — the **Sort** dropdown orders by rating or alphabetically.
- **View** — toggle between **grid** and **list** layouts.

Each card shows the API's icon, category, name, star rating, and a short
description. Click a card to open its detail page.

---

## 2. Explore an API

The product detail page (`/catalog/<slug>`) gathers everything about one API:

- **Overview** — description, category, version, and a **lifecycle badge**
  (Active / Deprecated / Sunset) with the sunset date when set.
- **API reference** — interactive **OpenAPI / Swagger documentation** rendered
  inline (when the product has a spec), so you can read every endpoint, model,
  and example.
- **Changelog** — a dated list of versioned changes (Added / Changed / Fixed …).
- **Ratings & reviews** — the community's star rating and written reviews.
- **Subscribe** — the **Subscribe** button opens the subscription flow (see §4).
- **Try it** — an in-browser call panel (see §8).

---

## 3. Create an application

An **application** is the container for your credentials and subscriptions.

1. Go to **Applications** → **New application** (or create one inline while
   subscribing).
2. Give it a name (and optional description) → **Create**.
3. Open the app to see its tabs: **Overview**, **Credentials**, **Subscriptions**,
   **Usage**, **Settings**.

When an application is created, the portal automatically provisions a matching
**consumer** and an **API key** at the gateway.

---

## 4. Subscribe to an API

1. On an API's detail page (or from its catalog card), click **Subscribe**.
2. In the modal, **choose an application** (or create one on the spot).
3. **Choose a plan** — each option shows its rate limit and price (free plans say
   *Free*).
4. Confirm. The subscription is created in **Pending** state.
5. An **admin approves** it (§20). You'll see the status change to **Active** on
   the app's **Subscriptions** tab, and approval emails are sent to admins.

Until approval, the gateway will **not** serve calls for that subscription
(you'll get `404` — no route exists yet). After approval, your key works.

To **unsubscribe**, open the app's **Subscriptions** tab and remove the
subscription; the gateway access is revoked immediately.

---

## 5. Get and use your API key

1. Open your application → **Credentials** tab. Your **API key** is shown (with a
   copy button), along with the **gateway URL**.
2. The **Overview** tab shows a ready-to-run **Quickstart** using your real key
   and the subscribed product's real path:

   ```bash
   curl http://localhost:9080/<contextPath> -H "apikey: <your-key>"
   ```

3. Expected responses once your subscription is **active**:
   - **200** — success (proxied to the upstream).
   - **401** — missing or wrong `apikey`.
   - **429** — you exceeded your plan's rate limit (see §9).
   - **404** — no active subscription/route for that path.

The API key authenticates you as your app's gateway **consumer**; rate limits are
enforced per consumer according to your plan.

---

## 6. Rotate keys

If a key leaks or you rotate on a schedule:

1. Application → **Credentials** (or **Settings**) → **Rotate key**.
2. A new key is generated and provisioned at the gateway; the old one stops
   working. Update your clients with the new key.

Sandbox keys can be enabled and rotated independently (see §8).

---

## 7. OAuth2 applications

Some products use **OAuth2** (client-credentials) instead of an API key — the
gateway validates a signed **JWT bearer token** against the identity provider's
JWKS, and only lets through client ids that are subscribed.

**As a developer:**

1. Open your application → register its **OAuth2 client id** (the id issued to you
   by the identity provider) via the app's settings.
2. Subscribe to the OAuth2 product and wait for approval (§4, §20).
3. Mint a token from the identity provider and call the gateway with it:

   ```bash
   # Mint a client-credentials token from the IdP
   TOKEN=$(curl -s -u <client_id>:<client_secret> \
     -d 'grant_type=client_credentials&scope=openid' \
     http://auth.example.com/oauth2/token | jq -r .access_token)

   # Call the gateway with the bearer token
   curl http://localhost:9080/<contextPath> -H "Authorization: Bearer $TOKEN"
   ```

   - **200** — your client id is subscribed and the token is valid.
   - **403** — valid token, but that client id isn't subscribed to this product.
   - **401** — missing or invalid token.

The client id claim the gateway checks is configurable (default matches the IdP's
`client_id` claim).

---

## 8. Try an API in the browser

The product detail page has a **Try it** panel (visible when signed in):

1. Pick one of your **applications** (it must have an active subscription to this
   product).
2. Choose the environment: **Production** or **Sandbox**.
3. Send the request; the response is shown inline.

**Sandbox** calls go to a separate sandbox gateway (`:9081`) so you can
experiment without touching production traffic or quotas. Enable a **sandbox
key** from the app's Credentials/Settings if prompted; it can be rotated
separately from the production key.

---

## 9. Monitor usage and quota

Open an application → **Usage** tab:

- **Usage cards** — request counts over a selectable range (24h / 7d / 30d).
- **Traffic chart** — requests over time.
- **Quota meter** — how much of your plan's rate limit you're consuming.

Metrics come from the gateway via Prometheus. If you hit **429** responses,
you're over your plan's limit — upgrade to a higher plan (re-subscribe) or wait
for the window to reset.

---

## 10. Rate and review an API

On a product detail page, the **Reviews** section lets subscribers leave a
**star rating and a written review**. Aggregated ratings appear on catalog cards
and the detail page, helping other developers choose.

---

## 11. Teams

Teams let a group share ownership and billing.

- Open **Teams** to create a team and **invite members** (by user).
- A team is the **billing entity** — invoices for paid subscriptions are scoped
  to the team (see §12).
- Every user gets a personal team by default; create additional teams as needed.
- Manage members (add / remove) from the team page.

---

## 12. Billing (developer view)

Open **Billing** to see your team's **invoices** (read-only):

- Each invoice shows the plan name, amount (formatted in its currency), status
  (**Pending / Paid / Void**), and dates.
- Invoices are generated automatically when a **paid** subscription is approved.
- Settlement (marking paid / voiding) is done by admins (§21). Free plans never
  generate an invoice.

---

## 13. Profile and language

Click your avatar (top-right) for the profile menu:

- See your **email** and **role** (Admin / Developer).
- **Log out**.
- Switch **FR / EN** from the top-bar toggle — the choice persists and follows
  your account across devices; notification emails are sent in your language.

---

# Admin guide

Admin features live under **Admin** in the top bar, with a sub-navigation:
**Products**, **Plans**, **Approvals**, **Invoices**, **Settings**.

## 14. Becoming an admin

The admin role is granted to the configured admin email at startup:

1. **Register** the admin email (the deployment's `ADMIN_EMAIL`, e.g.
   `admin@portal.local`) like any user.
2. **Restart the portal** — on boot it promotes that email to the **admin** role.
3. Log back in; the **Admin** menu now appears.

(New role takes effect on the next login after the restart.)

## 15. Publish an API product

**Admin → Products → New product** opens the Composer. Fill in:

- **Name, slug, category, version** — identity and catalog placement.
- **Context path** — the gateway path prefix (e.g. `/pizzashack`); calls to
  `http://localhost:9080/<contextPath>/…` are proxied to the upstream.
- **Upstream URL** — the backend the gateway forwards to (`host:port`, e.g.
  `echo:8080`).
- **Sandbox upstream** *(optional)* — a separate backend for try-it/sandbox.
- **Auth method** — **key-auth** (API key) or **oauth2** (bearer JWT, §7).
- **Version / lifecycle / sunset** — see §18.
- **OpenAPI spec** *(optional)* — paste or upload; renders as interactive docs
  and can pre-fill the form (§16).
- **Published** — toggle on to make it visible in the catalog.

**Save.** On approval of a subscription, the portal provisions the matching route
and consumers at the gateway automatically.

Notes:
- **Editing the upstream or auth method** of a product with active subscriptions
  re-provisions its gateway route immediately.
- **Deleting** a product that still has active subscriptions is blocked (**409**);
  remove subscriptions first, or unpublish (which keeps existing access).

## 16. Import from an OpenAPI spec

Instead of typing the form, **Products → Import an API**:

1. Provide the spec by **file upload** (JSON/YAML) or **URL** (OpenAPI 3.x or
   Swagger 2.0).
2. The portal parses it and **pre-fills the Composer** — name, version,
   description, slug, context path, and upstream (from `servers`/`host`).
3. Review the draft and click **Create** to publish through the normal validated
   path. (URL imports are SSRF-guarded: only public HTTP(S) hosts, size-capped,
   no redirects.)

## 17. Choose or upload an icon

In the product Composer, the **Icon** field lets you brand the API:

- **Built-in glyphs** — pick one of the built-in icons, or **Default** (a generic
  mark).
- **Custom upload** *(edit mode)* — save the product first, then upload a
  **PNG, JPEG, or WebP** (≤ 256 KB). The image is re-encoded server-side to a
  clean PNG; SVGs and non-images are rejected for security.
- A preview shows the current icon. Switching back to a built-in glyph removes the
  uploaded image.

The chosen icon appears on catalog cards and the product detail page.

## 18. Changelog and lifecycle

- **Changelog** — add dated entries (version + kind: Added/Changed/Fixed… +
  notes) from the product's Composer. They appear on the public detail page.
- **Lifecycle status** — mark a product **Active**, **Deprecated**, or **Sunset**,
  and set a **sunset date**. A badge is shown to developers so they can plan
  migrations.

## 19. Manage plans and pricing

**Admin → Plans** manages the tiers subscriptions run on:

- **Create / edit a plan** — set the **name**, **rate limit** (requests) and
  **window** (seconds), and the **price** (in cents) + **currency** (3-letter ISO,
  e.g. `EUR`, `USD`). Price `0` = a free plan (no billing).
- Changing a plan's rate limit/window **re-provisions** the affected active
  subscribers so the new limits take effect.
- A plan that is still in use by subscriptions **cannot be deleted** (**409**).

Plans are global (shared across products).

## 20. Approve or reject subscriptions

**Admin → Approvals** is the pending-subscription queue:

- Each row shows the application, owner, product, version, and requested plan.
- **Approve** — provisions the consumer + gateway route (and, for a paid plan,
  issues an invoice); the subscription becomes **Active** and the developer's key
  starts working.
- **Reject** — the subscription is marked rejected and never provisioned.

Approvals/rejections send an email to the developer (in their language).

## 21. Manage invoices

**Admin → Invoices** is the billing ledger:

- List all invoices; **filter** by status (All / Pending / Paid / Void).
- On a **Pending** invoice: **Pay** (mark settled) or **Void** (cancel).
- Paid/void invoices have no further actions.

Invoices are created automatically when a paid subscription is approved and are
scoped to the subscriber's **team**. The ledger survives unsubscribes (a paid
invoice is retained).

## 22. Runtime settings

**Admin → Settings** is where every configurable parameter of the running
deployment lives, grouped by area:

- **Portal** — base URL, `ADMIN_EMAIL`, trusted-proxy CIDRs, the private-upstream
  guard.
- **APISIX** / **Sandbox** — the admin/gateway URLs and admin keys for each
  gateway.
- **SMTP** — host, port, credentials, and the from-address used for the
  approval-loop and verification emails.
- **Policy** — `REQUIRE_EMAIL_VERIFICATION` (see [Getting started](#getting-started))
  and other account rules.
- **OIDC** — the OAuth2 identity provider's issuer and client-id claim (§7).
- **Observability** — the Prometheus URL usage/quota metrics are read from.

For each setting, the row shows its current **source** — env var (the
deployment's default) or an override saved from this UI — and a **reset**
button clears an override back to the env default. **Secrets** (keys,
passwords) are write-only: the field never displays the current value, only
lets you replace it.

Before a change is applied, **Test** runs a live health probe against
anything that touched APISIX or SMTP; a failing probe blocks **Save** unless
you deliberately click **Save anyway** to force it through.

A handful of boot-critical parameters (`DATABASE_URL`, `PORTAL_ADDR`,
`PORTAL_ENV`, `JWT_SECRET`, `CREDENTIAL_ENC_KEY`) are fixed at process start
and shown read-only here — they can only be changed via the environment and a
restart.

---

## How it works under the hood

You don't need this to use the portal, but it explains the behavior:

- **The portal owns its data** (products, apps, subscriptions, plans, invoices in
  PostgreSQL) and **provisions Apache APISIX** downstream via its Admin API.
- **key-auth** products: each app is an APISIX **consumer** with an API key; each
  product is a **route** whose consumer allow-list is the set of active
  subscribers. Rate limiting is per-consumer per the plan.
- **oauth2** products: the route validates the **bearer JWT** against the identity
  provider's **JWKS** and a serverless function enforces the subscriber
  **client-id allow-list** (200 subscribed / 403 not-subscribed / 401 no token).
- **Usage/quota** come from **Prometheus** scraping the gateway.
- **Emails** (approval loop) are sent over SMTP; in the test stack they land in
  **Mailpit**.
- Subscriptions are **approval-gated**: subscribing issues the key but grants no
  gateway access until an admin approves.

---

## Reference

**Default URLs (full test stack)**

| Surface | URL |
|---------|-----|
| Portal (web) | `http://localhost:8088` |
| Portal API | `http://localhost:8090/api` |
| Gateway (production) | `http://localhost:9080` |
| Gateway (sandbox) | `http://localhost:9081` |
| Mailpit (email inbox) | `http://localhost:8025` |
| Identity provider (OAuth2) | `http://auth.example.com` (via `:8081`) |

**Subscription statuses:** `pending` → `active` (approved) or `rejected`.
Unsubscribing removes the subscription and its gateway access.

**Invoice statuses:** `pending` → `paid` or `void`.

**Auth methods:** `key-auth` (send `apikey: <key>`) · `oauth2` (send
`Authorization: Bearer <jwt>`).

**Gateway response cheatsheet:** `200` served · `401` missing/invalid credential ·
`403` valid OAuth2 token but client id not subscribed · `404` no active route ·
`429` over the plan's rate limit.

**Related docs:** running/testing the stack — see the project
[`README.md`](../README.md) and [`docs/testing.md`](testing.md).
