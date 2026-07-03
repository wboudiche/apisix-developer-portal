# i18n — Sub-project 2: Backend API Messages — Design

**Date:** 2026-07-03
**Status:** Approved, ready for planning
**Surface:** new `internal/i18n` package; `internal/server` (Accept-Language middleware); `internal/httpx` (`ErrorT`); the ~164 `httpx.Error` call sites across `auth`/`catalog`/`subscriptions`/`applications`/`admin`/`teams`/`tryit`; `web/src/api/client.ts` (Accept-Language header).

## Problem

The frontend UI is now bilingual (sub-project 1), but the API returns
user-facing **error messages** as hardcoded (mostly English) strings that the UI
displays verbatim. So the French UI shows English errors, and neither language is
consistent. This localizes the backend's ~164 error messages by the request
locale, so an error surfaces in the same language as the UI.

## Decomposition (whole i18n feature — for context)

Three sub-projects: (1) frontend UI i18n **[DONE, merged]**; (2) **backend
API-message i18n (THIS spec)**; (3) stored `users.language` preference + email
localization **[later]**. This spec is sub-project 2 only.

## Locked decisions (from brainstorming)

- **The backend localizes** (a Go message catalog); the frontend stays dumb (it
  keeps displaying `ApiError.message`, now already-translated).
- **Transport: `Accept-Language`.** The frontend sends its active locale on every
  request; a middleware parses it into the request context. Works for
  authenticated AND anonymous requests (no dependency on the stored user pref,
  which is sub-project 3).
- **Scope: all ~164 messages** (including the 5xx "could not X" internal ones) —
  uniform, no per-site judgment, no English leaking into the French UI.
- The JSON error **shape is unchanged** (`{"error": "..."}`).

## Locale transport — `internal/server` middleware

- A middleware wraps the mux: read `Accept-Language`; take the first language tag;
  `en*` → `en`, else `fr`; absent/unparseable → **`fr`** (the default). Store the
  resolved `Lang` in the request `context` (via an `i18n` context key/setter).
- Applied to the whole API mux (before the handlers), so every request carries a
  locale. Simple first-tag parsing — no q-value weighting (2 languages).

## The `internal/i18n` package

- `type Lang string` (`"fr"` | `"en"`); a `DefaultLang = "fr"`.
- Catalogs `fr` + `en` as `map[string]string`, keyed by dotted keys namespaced by
  area (e.g. `subscribe.deprecated`, `teams.lastOwner`, `product.notFound`,
  `common.badAppID`). One entry per current error message.
- `func T(lang Lang, key string, args ...any) string` — look up the key in the
  active-lang catalog; fall back to `fr`, then the key itself (never blank);
  apply `fmt.Sprintf`-style interpolation when `args` are given (for the few
  messages with dynamic parts — most are static).
- `func FromContext(ctx) Lang` + `func WithLang(ctx, Lang) ctx` — the context
  plumbing the middleware uses.
- **Parity test:** `fr` and `en` have identical key sets (mirrors the frontend's
  compile/runtime guard; here a runtime test since Go maps aren't compile-checked).

## `httpx` integration

- `func ErrorT(w http.ResponseWriter, r *http.Request, status int, key string, args ...any)`:
  reads `i18n.FromContext(r.Context())`, calls `i18n.T`, and writes
  `{"error": <localized>}` by delegating to the existing `Error`.
- The existing `httpx.Error(w, status, msg)` stays (for genuinely non-user-facing
  or already-dynamic cases), but the 164 user-facing sites migrate to `ErrorT`.
- Handlers already have `r` in scope at every `httpx.Error` call, so **no handler
  signature changes** — each site is a literal-string → `(r, key)` swap.

## The sweep (per package)

Each `httpx.Error(w, status, "message")` → `httpx.ErrorT(w, r, status, "key")`,
adding a catalog entry:
- Today's messages are **English**, so **`en[key]` = the current string verbatim**
  and **`fr[key]` = its faithful French translation**.
- The one existing French straggler (`"abonnez-vous pour noter cette API"`) is
  handled in reverse (`fr` = current, `en` = translation).
- Keys are namespaced by area; identical strings across sites reuse one key.
- Done package-by-package: **foundation** (i18n + middleware + `ErrorT` + frontend
  header) → `auth` → `catalog` → `subscriptions` → `applications` → `admin` →
  `teams` → `tryit`. "Package complete" = no literal message strings remain in
  that package's `httpx.Error`/`ErrorT` calls (all reference keys).

## Frontend — `web/src/api/client.ts`

- A `langHeaders(token?)` helper returns `{ 'Accept-Language': localStorage.getItem('lang') || 'fr', ...(token ? Authorization) }`, used by **every** request (replacing the raw/`authHeaders` header construction) so both authenticated and anonymous calls send the active locale.
- No other frontend change; the UI already renders `ApiError.message`.

## Testing

### Backend (Go)

- **i18n package:** `T` resolves keys, falls back active→fr→key, interpolates
  with `args`; the `fr`/`en` parity test passes.
- **Middleware:** `Accept-Language: fr` → `fr`; `en-US,en;q=0.9` → `en`; absent →
  `fr`; garbage → `fr`. The resolved locale is in the context.
- **`httpx.ErrorT`:** with a context locale of `en` vs `fr`, writes the
  corresponding localized `{"error": …}`.
- **Representative handler tests:** a 4xx (e.g. subscribe to a deprecated product;
  add a duplicate team member; unknown login) returns the **French** message when
  the request carries `Accept-Language: fr` and the **English** message under
  `en`. (Confirms the middleware→context→ErrorT chain end-to-end.)
- Existing handler tests that assert an exact English message keep passing under
  the default (`fr`? — NO: those tests don't set Accept-Language, so they get the
  default `fr`; update them to either set `Accept-Language: en` or assert the
  French default. The plan will specify: existing message assertions get
  `Accept-Language: en` added so they keep asserting the English string, OR are
  updated to the French default — chosen per test to minimise churn.)

### Frontend (vitest)

- `langHeaders` includes `Accept-Language` from `localStorage['lang']` (and
  `Authorization` when a token is given); a representative client call sends it.

### Live (controller)

With the UI in **French**, trigger a user error (subscribe to the deprecated
`currencyconverterapi`, or add a duplicate member to a team) → the error toast is
**French**. Toggle to **English**, repeat → the same error is **English**.
Confirm via the network tab that requests carry `Accept-Language`. **Look at the
error in both languages.**

## Out of scope (this sub-project)

- Sub-project 3 (stored `users.language` preference + email-template localization).
- The frontend's own fallback strings (e.g. `request failed (N)` when the API
  returns no body) — a frontend concern, not swept here.
- Localizing `log.Printf` / internal log messages (ops-facing, not user-visible).
- `Accept-Language` q-value weighting (simple first-tag parsing suffices).
- A third language; RTL.
