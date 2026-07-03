# i18n — Sub-project 3: User Language Preference + Email i18n — Design

**Date:** 2026-07-03
**Status:** Approved, ready for planning
**Surface:** migration `0015`; `internal/auth` (users.language, register seed, `PUT /api/me/language`); `internal/notify` (per-recipient localized emails); frontend `AuthProvider`/`LanguageProvider`/`TopBar` (login sync + toggle-persist) + `api/client.ts`/`types.ts`.

## Problem

After sub-projects 1 (frontend UI i18n) and 2 (backend API-message i18n), a user's
language lives only in **`localStorage['lang']`** — per-device and invisible to the
server. So (a) logging in on another device/browser doesn't carry your language,
and (b) approval-loop **emails are French-only** regardless of the recipient's
language. This sub-project adds a **stored `users.language`** that syncs the UI
across devices on login and drives **per-recipient email localization**.

## Decomposition (whole i18n feature — for context)

Three sub-projects: (1) frontend UI i18n **[DONE]**; (2) backend API-message i18n
**[DONE]**; (3) **user-preference + email i18n (THIS spec)**. This closes the
feature.

## Locked decisions (from brainstorming)

- **Account preference wins on login** (cross-device sync): login returns the
  user's stored language; the client writes it into `localStorage['lang']` so the
  UI switches and `Accept-Language` follows. Toggling **while logged in** also
  persists to the server. Anonymous behavior (localStorage + `navigator.language`)
  is unchanged.
- **`language` travels in the login/register response body** (the existing `user`
  object), NOT in the JWT claims — no JWT churn; a toggle never needs to reissue a
  token.
- **Emails localize per recipient** from each recipient's stored `users.language`
  (fallback `fr`).
- Values `'fr'` | `'en'`; default `fr`.

## Data — migration `0015_user_language.sql`

- `ALTER TABLE users ADD COLUMN language TEXT NOT NULL DEFAULT 'fr'
   CHECK (language IN ('fr','en'))`. Existing rows → `fr`.
- `auth.Repo`: return `language` when loading a user (so it serializes in the
  `user` object of the login/register response); add
  `SetLanguage(ctx, userID, lang) error`.

## Register seed

- On `POST /api/auth/register`, seed the new row's `language` from the request's
  **`Accept-Language`** (the frontend already sends it from `localStorage['lang']`
  via sub-project 2's `langHeaders`), parsed with the SAME rule as the middleware
  (`en*`→`en`, else `fr`). So you register in the language you were browsing in.
  Reuse `i18n.FromContext(r.Context())` (the middleware already populated it) —
  no re-parsing.

## The preference endpoint — `PUT /api/me/language`

- Behind `auth.RequireAuth`. Body `{ "language": "fr" | "en" }`; validate the enum
  (else `400` via `httpx.ErrorT`, a new `common.*`/`auth.*` key). Calls
  `SetLanguage(userID, lang)` → `204 No Content`.
- The userID comes from the JWT (`auth.UserID(ctx)`); no body user-id.

## Frontend — login sync + toggle persist

- **Types/client:** `User` gains `language?: 'fr' | 'en'` (optional → no fixture
  churn); new `setMyLanguage(token, lang)` client fn (`PUT /api/me/language` via
  `sendAuthed`, best-effort — a failure must NOT block the toggle).
- **Login sync:** `AuthProvider` (mounted INSIDE `LanguageProvider`, so it can call
  `useLang().setLang`) — on a successful `login`/`register`, if
  `resp.user.language` is set, call `setLang(resp.user.language)`. That writes
  `localStorage['lang']` + `<html lang>` (existing `LanguageProvider` effect) and
  flips the UI; `Accept-Language` follows on subsequent requests.
- **Toggle persist:** the `TopBar` FR/EN toggle (inside `AuthProvider`, so it has
  the token) calls `setLang(next)` AND, when a `token` exists,
  `setMyLanguage(token, next)` (best-effort, `.catch` swallowed). Anonymous → only
  `setLang`.
- No change to `LanguageProvider`'s detect/persist internals; anonymous first-visit
  detection is unchanged.

## Backend API messages — unchanged

`Accept-Language` stays the transport (sub-project 2). Because login sync makes
`localStorage['lang']` (hence `Accept-Language`) mirror the account preference,
authenticated API errors are already in the right language. **No middleware
change.** Emails have no request context, so they read the stored preference
directly (below).

## Email localization — `internal/notify`

- **Recipient languages:** the recipient-resolving repo methods now return each
  recipient's language alongside the email:
  - `OwnerEmailsForApp` → `[]Recipient{Email, Lang}` (was `[]string` + name).
  - `AdminEmails` → `[]Recipient{Email, Lang}` (was `[]string`).
  A recipient row's `language` column feeds `Lang`; NULL is impossible (NOT NULL
  default), but an unknown value falls back to `fr` at render time.
- **Templates:** the three approval-loop messages (Requested→admins,
  Approved→owner, Rejected→owner) get **`fr` + `en`** subject+body variants. Emails
  are multi-line subject+body (not single error strings), so a dedicated
  `emailTemplates` keyed by `(kind, lang)` in `internal/notify` is cleaner than
  reusing the `internal/i18n` error-string catalog. Each variant keeps the current
  French copy verbatim as its `fr` and a faithful `en`, with the same
  `{baseURL}/admin/approvals`|`/applications`|`/` links + app/product/plan names.
- **Render per recipient:** `deliver` resolves recipients (each with a `Lang`),
  then renders + sends **one message per recipient in that recipient's language**
  (a French admin and an English admin on the same pending request each get their
  own language). Unknown/blank lang → `fr`. Everything stays best-effort + async +
  panic-safe (unchanged delivery semantics from sub-project's email work).

## Testing

### Backend (Go)

- **Migration/repo:** a user loads with `language`; `SetLanguage` round-trips;
  `CHECK` rejects a bad value.
- **Register seed:** register under `Accept-Language: en` → row `language='en'`;
  under `fr`/absent → `fr`.
- **`PUT /api/me/language`:** `en`/`fr` persist (204); bad value → 400; no auth →
  401 (middleware).
- **notify:** `emailTemplates` has both langs for all three kinds (a parity-style
  check); `deliver` renders each recipient in their own language and falls back to
  `fr` for an unknown lang; a two-admin case (one fr, one en) produces two
  correctly-localized messages (assert via a fake `Sender` capturing per-recipient
  subject/body).

### Frontend (vitest)

- `setMyLanguage` PUTs `/api/me/language` with the token + body.
- On login returning `user.language='en'`, the provider ends up `lang==='en'` +
  `localStorage['lang']==='en'`.
- The toggle, when a token is present, calls `setMyLanguage`; when anonymous, does
  not (only `setLang`).

### Live (controller)

Log in as a user, toggle to English → reload → still English; **log in as the same
user in a fresh browser profile (empty localStorage) → UI comes up English**
(cross-device sync). Trigger the approval loop with two admins of different stored
languages (or a French owner) → **each recipient's email is in their own
language** (check Mailpit: subject + body + the correct link). **Look at both
emails.**

## Out of scope (this sub-project)

- More than `fr`/`en`; HTML emails; per-event opt-out / unsubscribe; digests;
  a full profile/account-settings page (only the language field is added — no
  name/password/email editing here).
- Changing the API-message transport (stays `Accept-Language`).
- Retroactively translating already-sent emails; a durable outbox.
