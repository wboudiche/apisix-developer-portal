# Monetization — Sub-project 1: Priced Plans + Billing Ledger (backend) — Design

**Date:** 2026-07-03
**Status:** Approved, ready for planning
**Surface:** migration `0016`; `internal/plans` + `internal/admin` (plan price fields); new `internal/billing` package; `internal/subscriptions` (a nil-safe `Biller` hook in `Approve`); `internal/server` (wiring + billing/admin-invoice routes); `internal/catalog`/`internal/plans` (expose price).

## Problem

The portal is feature-complete for API access (discover → subscribe → approve →
provisioned → docs/try-it/usage/ratings) but has **no monetization**: plans are
pure rate-limit tiers with no price, and there is no billing record anywhere.
This sub-project adds the **pricing + billing foundation**: plans carry a price,
approving a paid subscription records an **invoice** against the subscriber's
**team**, and a pluggable `BillingProvider` (with a built-in **manual** default)
settles invoices — no external payment provider required.

## Decomposition (whole monetization feature — for context)

- **SP1 — Priced plans + billing ledger (backend) [THIS spec].**
- **SP2 — Billing UI (frontend):** pricing on catalog/plans, a per-team
  billing/invoices page, the pay/checkout affordance.
- **SP3 (later, optional):** a real PSP adapter (Stripe) + webhooks; recurring
  invoice scheduling; usage-based metered overage; suspend-on-unpaid.

Built SP1 → SP2 → SP3 (backend-then-frontend, matching prior features).

## Locked decisions (from brainstorming)

- **Billing model: flat recurring plan fee.** Each plan carries a price; the
  existing rate limits stay as the *technical* enforcement. Usage-based overage is
  a later sub-project.
- **Payment mechanism: a pluggable `BillingProvider` with a built-in manual
  default** — the portal owns the billing ledger; the manual provider records
  invoices and is settled by an admin mark-paid. No external PSP, no config, no
  live secrets. A real Stripe adapter can slot in later (SP3) without touching the
  domain. (Same pluggable/pragmatic pattern as BYO SMTP / BYO OIDC.)
- **Bill-after-provision:** admin approval issues the invoice AND provisions the
  subscription as it does today. The invoice is tracked as `pending`; settlement
  is separate. The existing approve→provision flow is unchanged. (Suspend-on-unpaid
  is SP3.)
- **Billing entity: the team.** One billing account per team (personal team for
  solo users); invoices belong to the team; any team owner/member can see them.
- **Money is integer minor units (cents) + an ISO currency code** — never floats.
- **`price_cents = 0` ⇒ a free plan** (no billing; current behavior preserved).

## Data model — migration `0016_billing.sql`

- `plans`: add `price_cents INTEGER NOT NULL DEFAULT 0` (CHECK `>= 0`) +
  `currency TEXT NOT NULL DEFAULT 'EUR'`.
- `billing_accounts`: `id BIGSERIAL PK, team_id BIGINT NOT NULL UNIQUE REFERENCES
  teams(id) ON DELETE CASCADE, created_at TIMESTAMPTZ NOT NULL DEFAULT now()`.
  Created **lazily** on the first paid activation for a team.
- `invoices`: `id BIGSERIAL PK, billing_account_id BIGINT NOT NULL REFERENCES
  billing_accounts(id) ON DELETE CASCADE, team_id BIGINT NOT NULL, subscription_id
  BIGINT NOT NULL REFERENCES subscriptions(id) ON DELETE CASCADE, plan_name TEXT
  NOT NULL, price_cents INTEGER NOT NULL, currency TEXT NOT NULL, status TEXT NOT
  NULL DEFAULT 'pending' CHECK (status IN ('pending','paid','void')), created_at
  TIMESTAMPTZ NOT NULL DEFAULT now(), paid_at TIMESTAMPTZ NULL`, plus an index on
  `(team_id, created_at DESC)`. **The `plan_name`/`price_cents`/`currency` are
  snapshotted onto the invoice** so later plan edits never rewrite billing history.

## The `internal/billing` package

- `type Invoice struct { ID, BillingAccountID, TeamID, SubscriptionID int64;
  PlanName string; PriceCents int; Currency, Status string; CreatedAt time.Time;
  PaidAt *time.Time }`; statuses `StatusPending/Paid/Void`.
- `Repo` (Postgres): `EnsureAccount(ctx, teamID) (accountID int64, err error)`
  (upsert `ON CONFLICT (team_id) DO NOTHING` + return id); `CreateInvoice(...)`;
  `PendingInvoiceExists(ctx, subID) (bool, error)` (idempotency guard);
  `ListByTeam(ctx, teamIDs []int64)`; `ListAll(ctx, status string)` (admin);
  `Get(ctx, id)`; `MarkPaid(ctx, id)` / `Void(ctx, id)` (guarded transitions →
  `ErrInvalidTransition`; `MarkPaid` only from `pending`).
- `type BillingProvider interface { Charge(ctx, inv Invoice) (ref string, err error) }`
  — a real PSP creates a payment intent/checkout and returns its ref; the built-in
  **`ManualProvider`** returns `("", nil)` (records nothing external; the invoice
  stays `pending` until an admin marks it paid). SP1 uses `ManualProvider`.
- `Service` wraps `Repo` + `BillingProvider`:
  - `SubscriptionActivated(ctx, appID, subID, planID int64) error` — the hook
    target (this IS the `Biller` interface method, so its signature matches the
    interface exactly; the subscriptions layer passes only the IDs it has).
    Internally: resolve `PlanPricing(planID) → (planName, priceCents, currency)`;
    if `priceCents == 0` → no-op. Else: `TeamForApp(appID) → teamID`; idempotency
    guard (`PendingInvoiceExists(subID)`); `EnsureAccount(teamID)`; `Charge`
    (manual = no-op); `CreateInvoice(status=pending)` with the snapshot.
  - `MarkPaid(ctx, id)` / `Void(ctx, id)`; `ListForTeams`; `ListAll`.

## The `Approve` hook — `internal/subscriptions`

- Mirror the existing `Notifier` decoupling: declare a narrow interface IN
  `subscriptions/service.go` and add a nil-safe setter.
  ```go
  // Biller records billing for a newly-activated paid subscription. Left unset
  // (nil) = disabled. Synchronous; an error fails the approval.
  type Biller interface {
      SubscriptionActivated(ctx context.Context, appID, subID, planID int64) error
  }
  func (s *Service) SetBiller(b Biller) { s.biller = b }
  ```
  (The billing side resolves team-from-app + price-from-plan itself, so the
  subscriptions `Store` gains no billing columns. To keep the resolution inside
  `billing` while the hook passes only IDs, the `billing.Service` used as the
  `Biller` wraps a small adapter that reads `applications.team_id` + the plan's
  `price_cents/currency/name` — provided via `billing.Repo` helpers
  `TeamForApp(appID)` + `PlanPricing(planID) (name, priceCents, currency)`.)
- In `Service.Approve`, **after** the subscription is set `active` (in BOTH the
  oauth2 and key-auth branches — at the single convergence point before
  `return nil`), call:
  ```go
  if s.biller != nil {
      if err := s.biller.SubscriptionActivated(ctx, rec.AppID, subID, rec.PlanID); err != nil {
          return err
      }
  }
  ```
  Billing is **synchronous and error-returning** (unlike best-effort email): a paid
  subscription must not silently go unbilled. The manual provider's work is a
  single DB insert, so this is reliable; the idempotency guard makes a retry safe
  (re-approving won't duplicate a pending invoice). Free plans (`price_cents==0`)
  make the hook a no-op, so the common path is unaffected.

## Endpoints

### Admin (behind `requireAdmin`)

- Plan create/update (`internal/admin/plan.go` + `plan_handler.go`) gain
  `priceCents int` + `currency string` on the JSON + `validate` (price `>= 0`;
  currency a 3-letter A–Z code; default `EUR` when empty) → 400 via `httpx.ErrorT`.
- `GET /api/admin/invoices` (all invoices, `?status=pending|paid|void` filter,
  newest-first).
- `POST /api/admin/invoices/{id}/pay` → `MarkPaid` (409 on illegal transition).
- `POST /api/admin/invoices/{id}/void` → `Void`.

### Team / developer (behind `requireAuth`)

- `GET /api/billing/invoices` → the caller's team(s)' invoices (resolved via
  team membership, newest-first) — no cross-team leak.

### Catalog / plans

- `GET /api/plans` and the plan shape returned with products expose
  `priceCents`/`currency` (add to `plans.Plan` + `admin.Plan` JSON). Free plans
  report `priceCents: 0`.

## Testing

### Backend (Go)

- **Migration/repo:** plans round-trip `price_cents`/`currency`; `EnsureAccount`
  is idempotent (one row per team); invoice create/`MarkPaid`/`Void` round-trip;
  `MarkPaid` from `paid`/`void` → `ErrInvalidTransition`; `PendingInvoiceExists`.
- **Approve hook:** approving a **paid**-plan subscription creates exactly one
  `pending` invoice for the app's team with the **snapshotted** plan name/price;
  approving a **free**-plan subscription creates **no** invoice; changing the
  plan's price AFTER the invoice exists does NOT change the invoice (snapshot);
  re-approving does not duplicate the pending invoice (idempotency); a `Biller`
  error propagates out of `Approve`.
- **Endpoints:** admin invoice list + `pay`/`void` (+ 409 on illegal transition);
  team invoice list is team-scoped (a user in team A cannot see team B's invoices);
  plan create/update validates price/currency (bad currency/negative price → 400).

### Live (controller)

Bring up the stack. Price a plan (`priceCents>0`) via the admin API; subscribe an
app to it and approve → a `pending` invoice appears for the team with the right
snapshot; `POST …/pay` → `paid` + `paid_at`. Approve a **free**-plan subscription
→ no invoice. Confirm `GET /api/billing/invoices` returns only the caller's team's
invoices. Verify plan JSON now carries `priceCents`/`currency`.

## Out of scope (this sub-project)

- The **frontend** (SP2): pricing display, billing page, checkout affordance.
- A **real PSP / Stripe** adapter + webhooks; **recurring** invoice scheduling;
  **usage-based** metered overage; **suspend-on-unpaid** enforcement (all SP3).
- Proration, taxes/VAT, refunds, credit notes, multi-currency conversion, dunning,
  partial payments. (Invoices are single-shot `pending`→`paid`/`void`.)
- Emailing invoices (the notify package is untouched here).
