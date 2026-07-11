# Email Verification for Registrations — Design

**Date:** 2026-07-11
**Status:** Approved, ready for planning
**Surface:** `internal/config` (new flag + fail-fast), `internal/auth` (register/login/verify/resend + repo), `internal/db/migrations` (one migration), `internal/notify` (verification email kind), `internal/server` (wiring), `web/src` (register/login adjustments + `/verify-email` route), `docker-compose.full.yml` + README (optional flag documentation).

## Problem

Registration accepts any email and immediately issues a JWT — nothing proves the
mailbox exists or belongs to the registrant. Deployments that want real,
reachable subscriber emails (approval notifications go to those addresses) need
an opt-in verification gate.

## Locked decisions (from brainstorming)

- **Hard gate:** with the feature on, an unverified user **cannot log in at
  all**; registration no longer returns a token.
- **Explicit opt-in flag:** `REQUIRE_EMAIL_VERIFICATION=1`. At startup, setting
  it without `SMTPConfigured()` is a **fatal error** (fail-fast; never lock
  users out on a deployment that cannot send the verification email).
- **Grandfathering:** the migration marks all existing accounts verified; only
  post-migration registrations must verify.
- **Token lifecycle:** link valid **24 h**; single-use; resend invalidates the
  previous token; verifying an already-verified account is a friendly no-op.
- **Approach:** random token stored **hashed** in the DB (not a signed/JWT
  token — single-use and invalidate-on-resend must be enforceable, and the auth
  signing machinery must not gain a second purpose).

## Config

- `RequireEmailVerification bool` from `REQUIRE_EMAIL_VERIFICATION` (`"1"` = on,
  default off) in `internal/config/config.go`.
- `main`/server startup: `if cfg.RequireEmailVerification && !cfg.SMTPConfigured() { log.Fatal(...) }`.
- Feature off ⇒ every behavior below is inert; registration/login are unchanged.

## Data model (migration `0019_email_verification.sql`)

```sql
ALTER TABLE users
  ADD COLUMN email_verified BOOLEAN NOT NULL DEFAULT TRUE,
  ADD COLUMN verify_token_hash TEXT,
  ADD COLUMN verify_token_expires_at TIMESTAMPTZ;
```

`DEFAULT TRUE` grandfathers every existing row. When the feature is **on**, the
register path explicitly inserts `email_verified = FALSE`; when **off**, the
default keeps new users verified (so later enabling the flag never strands
users created while it was off).

## Flows

### Register (feature on)

1. Create user with `email_verified = FALSE`.
2. Generate a random token (same 128-bit hex generator style as API keys);
   store SHA-256 hex in `verify_token_hash`, expiry `now() + 24h`.
3. Send localized (fr/en, per request language) verification email via the
   existing `notify.SMTPSender`: link `PORTAL_BASE_URL/verify-email?token=<t>`.
   Send failures are logged, registration still returns 201 (resend covers it).
4. Respond **201 without a JWT**: `{"user": ..., "verificationRequired": true}`.

### Login (feature on)

- Credentials valid but `email_verified = FALSE` → **403** with i18n code
  `auth.login.emailNotVerified`. (401 stays for bad credentials.)

### Verify — `POST /api/auth/verify` `{token}` (public)

- Hash the token, look up the user by `verify_token_hash`.
- Match + unexpired → set `email_verified = TRUE`, clear both token columns →
  204.
- No match or expired → **410** `auth.verify.invalidOrExpired` (the UI offers
  resend). Already-verified users have no stored hash, so a stale link lands
  here; the verify page copy covers "already verified? just log in".

### Resend — `POST /api/auth/resend-verification` `{email}` (public)

- Always **204** regardless of account existence or state (no account-existence
  oracle — consistent with the login timing discipline, M3).
- If the account exists and is unverified: new token + expiry overwrite the old
  ones (previous link dies), email re-sent.
- Rate-limited per email via a `httpx.RateLimiter` like the login limiter.

## Email content

New kind alongside the three notify kinds, fr + en:
subject "Vérifiez votre adresse e-mail" / "Verify your email address"; body:
greeting (user name), the link, the 24 h validity note, ignore-if-not-you note.

## Frontend (`web/src`)

- **Register page:** when the 201 response has `verificationRequired: true`,
  show "check your inbox" instead of auto-login (feature off keeps auto-login).
- **Login page:** on the 403 code, show the message + a **Resend email** button
  (calls resend with the typed email; always shows "sent if the account exists").
- **New route `/verify-email`:** reads `?token=`, calls verify on mount; states:
  success ("you can now log in" + link), invalid/expired (resend form), loading.
- i18n strings in `fr.ts`/`en.ts` for all of the above.

## Error handling

- Startup misconfiguration: fatal, explicit message naming both variables.
- SMTP send failure at register/resend: logged, request still succeeds
  (best-effort like existing notify; resend is the recovery path).
- Verify/resend endpoints sit under `/api/auth/` and inherit the per-IP limiter.

## Testing

- **Config:** flag parsing; fatal combination checked at server construction.
- **Auth handler (Go):** register on/off (token withheld/issued, verified flag),
  login 403 branch, verify happy/expired/garbage/reused, resend for
  existing-unverified / existing-verified / unknown email (all 204, side
  effects only in the first case) — sender faked, no real SMTP in unit tests.
- **Migration:** existing rows end up verified (pattern of `migrate_teams_test`).
- **Frontend (vitest):** register "check inbox" state, login resend button on
  403, `/verify-email` three states.
- **Manual/e2e:** full-stack pass with Mailpit (register → link in Mailpit →
  verify → login).

## Out of scope (YAGNI)

- Admin UI to manually verify a user (resend covers recovery).
- Verifying email **changes** (the portal has no change-email flow today).
- Retroactive verification of grandfathered accounts.
- Applying the gate to OAuth2/OIDC-federated identities (portal-local
  registration only).
