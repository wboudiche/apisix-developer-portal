# Runtime-Editable Portal Settings — Design

**Date:** 2026-07-13
**Status:** Approved, ready for planning
**Surface:** new `internal/settings` package + migration `0021_portal_settings.sql`; refactor of `internal/server` boot wiring (swappable bindings); `internal/auth` verification gate + `internal/notify` sender/base-URL reads become dynamic; new admin API under `/api/admin/settings`; new "Paramètres" tab in the admin UI (`web/src`).

## Problem

Every parameter is env-var-only and wired once at boot in `server.New`. Changing
SMTP, gateway endpoints, or policy flags requires editing the deployment and
restarting. Admins need to view every parameter and edit every *possible* one
from the UI, applied live, without a restart.

## Locked decisions (from brainstorming)

- **Scope:** everything editable **except** boot-critical: `DATABASE_URL`,
  `PORTAL_ADDR`, `PORTAL_ENV`, `JWT_SECRET`, `CREDENTIAL_ENC_KEY`. Those still
  appear in the UI, read-only (secrets shown only as "set", never a value).
- **Precedence:** a UI-saved (DB) value wins over the env var; env is the
  seed/default; per-setting "reset to env default" (= DELETE the DB row).
- **Secrets are write-only:** the API never returns stored secret values
  (`SMTP_PASSWORD`, `APISIX_ADMIN_KEY`, `APISIX_SANDBOX_ADMIN_KEY`); stored
  encrypted with the existing credential cipher (`crypto.New(CREDENTIAL_ENC_KEY)`).
- **Save = validate + live probe:** format validation always; APISIX and SMTP
  groups additionally health-probe with the candidate values and refuse to save
  on failure unless the request carries an explicit force flag.
- **Architecture:** Approach A — settings service with an atomic effective-config
  snapshot and swappable service bindings. No restart, atomic apply.

## Settings registry (single source of truth)

One Go table in `internal/settings/registry.go` declaring every parameter:
`key` (the env var name), `group`, `type` (`string|bool|port|url|hostport|email|csv`),
`secret bool`, `editable bool`, and a validation rule. Groups and members:

| Group | Keys | Editable |
|---|---|---|
| Server | `PORTAL_ADDR`, `PORTAL_ENV`, `DATABASE_URL`, `JWT_SECRET`, `CREDENTIAL_ENC_KEY` | no (read-only display) |
| Portal | `PORTAL_BASE_URL`, `ADMIN_EMAIL`, `TRUSTED_PROXIES`, `UPSTREAM_ALLOW_PRIVATE` | yes |
| APISIX (prod) | `APISIX_ADMIN_URL`, `APISIX_GATEWAY_URL`, `APISIX_ADMIN_KEY`* | yes |
| APISIX (sandbox) | `APISIX_SANDBOX_ADMIN_URL`, `APISIX_SANDBOX_GATEWAY_URL`, `APISIX_SANDBOX_ADMIN_KEY`* | yes |
| SMTP | `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD`*, `SMTP_FROM` | yes |
| Policy | `REQUIRE_EMAIL_VERIFICATION` | yes |
| OIDC | `OIDC_ISSUER`, `OIDC_CLIENT_ID_CLAIM` | yes |
| Observability | `PROMETHEUS_URL` | yes |

`*` = secret (write-only). `UPSTREAM_ALLOW_PRIVATE` is today read via
`os.Getenv` directly in `server.New`; it joins `Config` and the registry.

## Data model (migration `0021_portal_settings.sql`)

```sql
CREATE TABLE IF NOT EXISTS portal_settings (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL,          -- secrets: ciphertext from the credential cipher
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_by BIGINT REFERENCES users(id) ON DELETE SET NULL
);
```

Only **overridden** settings have rows; absence = env default. Reset-to-env is
a DELETE. Unknown keys in the table (from a future downgrade) are ignored with
a boot-time log line.

## Runtime architecture

### Snapshot

`settings.Service` holds `atomic.Pointer[Effective]` where `Effective` is an
immutable struct mirroring `config.Config` plus a `Source(key)` map
(`env|db`). Built at boot: start from `config.Load()`, overlay decrypted DB
rows. Readers (request paths) do one atomic load — no locks. Writers serialize
on a mutex: validate → probe → persist → rebuild snapshot → swap → notify.

### Swappable bindings (what "notify" rebuilds)

- **APISIX gateways:** `apisix.SwappableGateway` wraps `atomic.Pointer[*apisix.Client]`
  and implements the existing `apisix.Gateway` interface by delegating. Prod and
  sandbox each get one; on APISIX-group change the inner client is replaced.
  Empty sandbox URLs ⇒ the sandbox holder reports "disabled" and the
  subscriptions service treats sandbox as unconfigured — same semantics as
  boot-time absence today, now consulted dynamically per operation.
- **SMTP sender:** `notify.SMTPSender` already dials per Send; replace the
  boot-constructed instance with a `notify.DynamicSender` that reads
  host/port/user/password/from from the snapshot on each Send. Notifier and
  verification email share it. `SMTPConfigured` becomes a snapshot query.
- **Email-verification gate:** verify/resend routes are ALWAYS mounted when the
  handler is built; each request first consults the snapshot — feature off ⇒
  404 (preserves today's "routes absent" contract), on ⇒ normal behavior.
  Register/login consult the snapshot the same way. `EnableEmailVerification`'s
  one-shot wiring is replaced by a provider interface the auth handler holds.
- **OIDC / Prometheus / base URL / trusted proxies / allow-private:** consumers
  (`subscriptions.Service.ConfigureOIDC`, try-it handler, metrics reader,
  notifier links, rate-limiter proxy list, admin product validation) read from
  the snapshot per use or are re-armed by the notify hook — each is a small,
  local change listed per-consumer in the plan.
- **`ADMIN_EMAIL` change:** on save, immediately re-run the promote logic
  (`EnsureAdminRole`) for the new address (existing role assignments are not
  revoked — same as today's boot behavior).

### Cross-field invariants (enforced at save, mirrors boot `Validate()`)

- `REQUIRE_EMAIL_VERIFICATION=1` requires effective SMTP configured (host+from),
  considering the candidate values in the same request. Violation ⇒ 422, not
  forceable.
- Disabling SMTP while verification is on ⇒ same 422.

## Admin API (all behind `requireAdmin`)

- `GET /api/admin/settings` → `[{group, items: [{key, value|null, masked bool,
  set bool, source: "env"|"db", envDefault|null, editable, type}]}]`. Secret
  items: `value:null, masked:true, set:true/false`; boot-critical secrets show
  only `set`.
- `PUT /api/admin/settings` body `{values: {KEY: "..."}, force?: bool}` —
  partial update, all-or-nothing (one transaction + one snapshot swap). All
  wire values are strings (they are env vars); `bool`-typed keys accept
  exactly `"1"` (on) or `""` (off), matching env semantics.
  Responses: 204; 400 unknown/read-only key; 422 `{fields: {KEY: "reason"}}`
  for validation/invariant failures; 422 `{probe: {...}}` for probe failures
  (retryable with `force:true` — probes only, never invariants).
- `DELETE /api/admin/settings/{key}` → reset to env (404 unknown, 400 read-only).
- `POST /api/admin/settings/test` body `{values}` → run the applicable probes
  with candidate values overlaid on the current snapshot, no persistence;
  returns per-probe `{ok, detail}`.

### Probes (5 s timeout each)

- APISIX prod/sandbox: `GET {ADMIN_URL}/apisix/admin/routes?page_size=1` with
  the candidate key; ok = HTTP 200.
- SMTP: TCP dial + EHLO (no AUTH attempt); ok = greeting + EHLO accepted.
- Probes run only when the PUT touches keys of that group.

### Audit

Every successful PUT/DELETE writes one entry per changed key to the existing
events log: key, old → new (secrets logged as `(secret)` → `(secret)`),
source transition, admin user id.

## Admin UI — "Paramètres" tab

New `AdminShell` tab (`/admin/settings`), Atlas vocabulary. Grouped cards per
registry group; each row: label (the env var name + one-line description),
input typed per setting (text / URL / port / toggle for bools / write-only
password field showing `••••• (défini)` with a replace affordance), a source
badge (`env` gray / `modifié` accent) and a per-row reset action (shown only
when source=db, confirms, shows the env default it will return to). Boot-
critical group renders read-only with a lock hint. Sticky save bar appears when
dirty: "Tester" (runs `/test` with the dirty values, inline results) and
"Enregistrer" (PUT; on probe failure shows the probe detail with a
"Enregistrer quand même" force option; invariant failures are non-forceable
field errors). All strings in fr + en catalogs.

## Error handling

- Cipher unavailable (bad `CREDENTIAL_ENC_KEY`): portal already fatals at boot.
- DB row that fails to decrypt: logged, treated as absent (env default), shown
  in the UI as source=env with a warning badge; saving overwrites it.
- Probe network errors surface their detail string to the UI.
- Concurrent PUTs: writer mutex serializes; last write wins (no optimistic
  concurrency — single-admin reality, YAGNI).

## Testing

- **Registry:** every key round-trips typed validation; read-only keys refuse
  writes; secret keys never appear in GET payloads (unit test walks the JSON).
- **Service:** precedence (env-only / overridden / reset), snapshot atomicity
  (concurrent readers during a swap — race detector), decrypt-failure fallback.
- **Bindings:** gateway swap (fake gateway asserts post-save calls use new
  URL/key), sandbox enable/disable at runtime, verification gate toggling
  (routes 404 when off, work when on — reuses Task-5-style handler tests),
  dynamic SMTP sender reads candidate values.
- **API:** 204/400/422 matrix, force semantics (probe yes, invariant no),
  DELETE reset, audit entries written.
- **UI (vitest):** groups render from GET fixture; secret field write-only
  behavior; dirty-state save bar; probe-failure force flow; reset flow.
- **Live compose pass:** change SMTP host to a bogus value with force, watch
  verification email fail, reset, watch it recover; swap sandbox URLs off and
  on; verify no restart occurred.

## Out of scope (YAGNI)

- Optimistic concurrency / setting-level locking.
- Multi-environment profiles or scheduled changes.
- Editing boot-critical settings with deferred "apply on restart".
- Import/export of settings.
