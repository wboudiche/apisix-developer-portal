# Real API Key Rotation — Design

**Date:** 2026-06-27
**Status:** Approved, ready for planning
**Surface:** Application detail → Credentials (`web/src/pages/application/CredentialsTab.tsx`), `internal/subscriptions`.

## Problem

The Credentials tab shows a **demo** key-rotation experience: the Production
"Régénérer" button only toasts "Rotation à venir", and the Sandbox key + its
rotation are entirely client-side fakes (`demo.ts`). This makes a real, visible
surface dishonest. We make key rotation real, server-side, and remove the demo
Sandbox card (a true sandbox needs a sandbox-upstream concept on products — a
separate, larger feature, explicitly out of scope here).

## Locked decisions (from brainstorming)

- **Scope:** key rotation only. Real sandbox (sandbox-upstream) is deferred to a
  separate future feature.
- **Rotation mode:** **immediate revoke** — one key-auth key per app consumer;
  installing the new key makes the old one 401 instantly. No grace window.
- **Sandbox card:** **removed** from the Credentials tab (it was a client-side
  fake). The card can return when the real sandbox feature is built.
- **Limit on rotation:** preserved by re-deriving the app's current rate limit
  from its most-recent **active** subscription's plan (matching the existing
  "consumer-per-app, last-write-wins" model that Approve uses).

## Backend

### Endpoint

`POST /api/applications/{id}/credentials/rotate` — added to the existing
subscriptions handler (`subscriptions.Handler`), already mounted under
`/api/applications/` behind `requireAuth` with the owner-scoping `owns` check
(so a non-owner gets 403/404). Returns `200 { "apiKey": "<new key>" }`.

### Service: `Service.RotateKey(ctx, appID int64) (string, error)`

1. Load the app's credential via `store.GetCredential(appID)`. If none →
   `ErrNoCredential` → **409** ("no key to rotate" — a credential/consumer only
   exists after a subscription is approved).
2. Derive the app's current limit: `store.ActivePlanForApp(appID)` (new) returns
   the plan of the app's most-recent **active** subscription. If none →
   `ErrNoActiveSubscription` → **409**.
3. Generate a new key with `genKey()` (the same `GenerateKey` used elsewhere).
4. `gw.EnsureConsumer(consumerUsername, newKey, {plan.Count, plan.WindowSeconds})`
   — APISIX PUT replaces the consumer's key-auth key; the old key 401s
   immediately. **Call the gateway BEFORE committing the new key to the DB** so
   that on gateway failure the DB still holds the old (still-live) key — the
   invariant "DB key == gateway key" holds.
5. Persist the new key: `store.UpdateCredentialKey(appID, newKey)` (new;
   encrypts at rest like the existing credential write).
6. Log a `key_rotated` activity event (new event kind) for the app.
7. Return the new key.

Ordering note: step 4 (gateway) before step 5 (DB) — if the gateway succeeds but
the DB write fails, the gateway has the new key while the DB has the old; a
follow-up rotation re-converges (idempotent PUT). The opposite ordering would
risk the DB advertising a key the gateway never accepted. Document this; the
chosen order fails toward "gateway ahead of DB", which a retry fixes.

### Store additions (`subscriptions.Repo`)

- `ActivePlanForApp(ctx, appID) (Plan, error)` — the plan of the most-recent
  active subscription for the app (`ORDER BY created_at DESC LIMIT 1`), or
  `ErrNoActiveSubscription`.
- `UpdateCredentialKey(ctx, appID, newKey string) error` — updates the encrypted
  `credentials.api_key` for the app (mirrors the existing encryption on write).

### Events

- New kind `events.KindKeyRotated = "key_rotated"` in `internal/events`, logged
  by `RotateKey` (product/plan ids nil — it's app-scoped). Surfaces in the app
  activity feed and powers the real "Dernière rotation" timestamp.

## Frontend

### `CredentialsTab.tsx`

- **Remove the Sandbox `KeyCard`** and all sandbox state (`sbxKey`,
  `sbxRevealed`). One **Production** card remains.
- Delete the now-unused `DEMO_SANDBOX_KEY`, `DEMO_ROTATION`, and `demoRotatedKey`
  from `demo.ts` (keep `demoBarWidth`/`demoRpm`/`DEMO_QUICKSTART` if still used
  elsewhere; remove only what rotation/sandbox owned).
- Wire **Régénérer** to a real flow: confirm modal (existing copy — it already
  says the old key is revoked immediately) → on confirm call
  `rotateKey(token, appId)` → set the displayed key to the returned key,
  auto-reveal once, toast "Nouvelle clé générée". On error, toast the backend
  message.
- "Dernière rotation" shows a real timestamp: the latest `key_rotated` event
  time from the app detail (or "—" when never rotated), replacing `DEMO_ROTATION`.

### Plumbing

- `CredentialsTab` receives `appId` (and `token`) from `AppDetailPage` so it can
  call the endpoint; on success it updates the shown key in place (no full
  reload required), and the app-detail activity feed reflects the new event on
  its next refresh.
- New client fn `rotateKey(token, appId): Promise<{ apiKey: string }>` →
  `POST /api/applications/{id}/credentials/rotate`.

## Testing

- **Go**
  - `RotateKey` happy path: returns a key different from the old; `EnsureConsumer`
    called with the new key and the active plan's limit; `UpdateCredentialKey`
    persists it; a `key_rotated` event is logged.
  - No credential → `ErrNoCredential` → 409; no active subscription →
    `ErrNoActiveSubscription` → 409.
  - Gateway failure → the DB key is unchanged (old key preserved).
  - Handler: route is owner-scoped (non-owner → 403/404); happy path 200 with
    `{apiKey}`.
  - Repo (DB): `ActivePlanForApp` returns the active sub's plan and errors when
    none; `UpdateCredentialKey` round-trips through encryption.
- **Frontend (vitest)**
  - CredentialsTab renders ONLY the Production card (no `key-sbx`).
  - Régénérer → confirm → `rotateKey` called → the revealed key updates to the
    returned value.
  - `rotateKey` client fn POSTs to the right URL with auth.
- **Live (controller)**
  - Against the running stack, rotate the key of an approved app (reuse the
    try-it/echo product): confirm the OLD key now 401s at the gateway and the
    NEW key returns 200.

## Out of scope (deferred)

- Real sandbox environment (sandbox-upstream on products + dual provisioning).
- Zero-downtime / multi-secret rotation with a grace window.
- Key rotation for OAuth2/JWT credentials (only key-auth exists today).
