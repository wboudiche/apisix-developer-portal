# Email Notifications — Design

**Date:** 2026-06-30
**Status:** Approved, ready for planning
**Surface:** new `internal/notify` package; `internal/config` (SMTP); `internal/subscriptions` (Service hooks); `internal/server`/`cmd/portal` (wiring); `docker-compose.yml` (Mailpit for dev/test).

## Problem

The portal runs an approval workflow (subscribe → pending → admin approves/rejects)
but only surfaces status in-app. Developers don't learn their subscription was
approved/rejected unless they check the UI, and admins don't learn a new request
is waiting. This adds **email notifications for the approval loop**, a standard
WSO2/Gravitee table-stakes capability.

## Locked decisions (from brainstorming)

- **Scope (approval loop only):** developer ← subscription **approved** /
  **rejected**; admins ← a **new subscription request** is pending review. No
  emails for key rotation, app-created, subscribe-confirmation, or unsubscribe
  (deferred).
- **Delivery: SMTP, bring-your-own.** Configurable host/port/credentials/from
  (Go stdlib `net/smtp`). The operator points it at any mail server (or a local
  Mailpit in dev). **Inert when unconfigured** (mirrors `OIDCConfigured()` /
  `SandboxConfigured()`).
- **Semantics: best-effort, async.** Send in a background goroutine after the
  action commits; on any error, log and drop (no retry). The in-app approval
  status stays the source of truth, so a dropped email is recoverable by the
  developer checking the UI. Never slows or fails the approve/reject/subscribe
  action.
- Recipients: developer = the app **owner's** email; admins = users with
  `role='admin'`.
- Backend-only (no UI). Single implementation plan.

## Config — `internal/config`

- `SMTP_HOST` (`""`), `SMTP_PORT` (default `587`), `SMTP_USERNAME` (`""`),
  `SMTP_PASSWORD` (`""`), `SMTP_FROM` (the `From:` address, `""`).
- `PORTAL_BASE_URL` (e.g. `http://localhost:5173`) — used to build links in the
  email bodies.
- `func (c Config) SMTPConfigured() bool` → `SMTP_HOST != "" && SMTP_FROM != ""`.
  When false, the notifier is unset and every notification call is a no-op. The
  dev-secrets guard is unchanged (SMTP credentials are operator-supplied, with no
  built-in portal default to guard against).

## Package `internal/notify`

### Sender (SMTP)

```
type Sender interface {
    Send(ctx context.Context, to []string, subject, body string) error
}
```
- `SMTPSender` implements it over `net/smtp`: builds an RFC 5322 message via a
  **pure** `buildMessage(from string, to []string, subject, body string) []byte`
  (headers `From`, `To`, `Subject`, `Date`, `MIME-Version`, `Content-Type:
  text/plain; charset=utf-8`; CRLF line endings; the plaintext body), then dials
  the server. Uses STARTTLS and `smtp.PlainAuth` when `SMTP_USERNAME` is set;
  plain (no auth) otherwise — for a local Mailpit. A send timeout (~20s) via the
  passed context.

### Notifier

```
func NewNotifier(sender Sender, repo *Repo, baseURL string) *Notifier
```
The higher-level type the subscriptions Service depends on. Methods:

- `SubscriptionRequested(appID, productID, planID int64)` — recipients: all
  admins. Subject/body: a new request from `{appName}` for `{product} ({plan})`,
  link `{baseURL}/admin/approvals`.
- `SubscriptionApproved(appID, productID, planID int64)` — recipient: the app
  owner. Subject/body: `{appName}`'s subscription to `{product} ({plan})` is
  approved, link `{baseURL}/applications`.
- `SubscriptionRejected(appID, productID int64)` — recipient: the app owner.
  Subject/body: `{appName}`'s subscription request to `{product}` was rejected,
  link `{baseURL}/` (catalog).

Each method launches a goroutine that calls a **synchronous, testable**
`deliver(ev event)`: resolve recipients + names, render the template, and
`sender.Send`. The goroutine uses a fresh `context.Background()` with a ~20s
timeout (the request context is gone once the handler returns). All errors are
`log.Printf`'d and dropped; an empty recipient list is skipped.

### Templates

Small functions returning `(subject, body string)`, one per event. Plaintext,
French copy, including the app/product/plan names and the portal link. (No HTML
in V1.)

### Read repo (`internal/notify`)

A tiny repo over the pool:
- `OwnerEmailForApp(ctx, appID) (email, appName string, err error)` — joins
  `applications` → `users`.
- `AdminEmails(ctx) ([]string, error)` — `SELECT email FROM users WHERE role='admin'`.
- `ProductName(ctx, productID) (string, error)` and `PlanName(ctx, planID)
  (string, error)` — for the body (or a combined lookup). Missing/empty results
  degrade gracefully (skip or use a fallback label).

## Integration — `internal/subscriptions/service.go`

- The Service gains a `notifier Notifier` field (a **narrow interface** with the
  three methods above; satisfied by `*notify.Notifier`) and a `SetNotifier(Notifier)`
  setter (mirrors `SetUsageReader`/`SetOIDCIssuer` — no `NewService` signature
  change). `nil` notifier = disabled.
- A best-effort guard (`if s.notifier != nil`) invokes the matching method right
  after the corresponding `logEvent`:
  - `Subscribe` (both the key-auth and oauth2 paths, after `SaveSubscription`) →
    `SubscriptionRequested(appID, productID, planID)`.
  - `Approve` (after `SetSubscriptionStatus(active)`) →
    `SubscriptionApproved(rec.AppID, rec.ProductID, rec.PlanID)`.
  - `Reject` (after `SetSubscriptionStatus(rejected)`) →
    `SubscriptionRejected(rec.AppID, rec.ProductID)`.
- Because the Notifier methods are async internally, these calls return
  immediately and cannot affect the action's outcome.

## Wiring — `internal/server` / `cmd/portal`

In `server.New`, when `cfg.SMTPConfigured()`: build the `notify.SMTPSender`
(from the SMTP config), a `notify.Repo` (over the pool), a `notify.Notifier`
(sender + repo + `cfg.PortalBaseURL`), and call `subSvc.SetNotifier(notifier)`.
Otherwise leave the notifier unset. (Same nil-gated pattern as the sandbox/OIDC
wiring.)

## Infrastructure — docker-compose (dev/test only)

Add a **Mailpit** service (`axllent/mailpit`) — a catch-all SMTP sink on
`:1025` with a web inbox on `:8025` — so the live verification can confirm
emails without a real mail server. NOT a product dependency (the portal is
BYO-SMTP); it's a dev convenience like `echo`.

## Testing

### Backend (Go)

- **Notifier (fake Sender + fake repo):** each method sends to the right
  recipients (admins for *Requested*; the owner for *Approved*/*Rejected*) with
  a subject + body containing the product/plan/app names and the portal link;
  an empty recipient list and a nil/failing Sender are handled (no panic, error
  logged, action unaffected). Tests call the synchronous `deliver` to avoid
  goroutine races.
- **SMTP message build:** `buildMessage` produces correct RFC 5322 headers
  (From/To/Subject/Date/MIME) + body with CRLF — asserted without a live server.
- **Service (fake Notifier):** `Subscribe`/`Approve`/`Reject` invoke the matching
  Notifier method with the right ids; a nil notifier is safe (no call, no panic);
  the existing approve/reject behavior is unchanged.
- **Config:** `SMTPConfigured()` true only when host+from are set; port default 587.

### Live (controller)

Bring up Mailpit (`docker compose up -d mailpit`); restart the portal with
`SMTP_HOST=mailpit`/`SMTP_PORT=1025`/`SMTP_FROM=portal@local`/`PORTAL_BASE_URL=…`.
As a developer, subscribe an app to a product (→ admins get a "new request"
email in Mailpit); as admin, approve it (→ the developer gets an "approved"
email) and reject another (→ "rejected" email). Confirm the three emails in the
Mailpit web inbox (`:8025`), with correct recipients, subjects, and links.
**Look at the inbox.**

## Out of scope (deferred)

- HTML emails (plaintext only in V1).
- Per-user notification preferences / opt-out / unsubscribe links.
- Digests / batching.
- Retries / durable outbox (best-effort only).
- Emails for non-approval events (key rotation, app-created, subscribe
  confirmation, unsubscribe).
- A real bundled mail server (BYO SMTP; Mailpit is dev/test only).
