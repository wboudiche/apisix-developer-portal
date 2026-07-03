# Monetization — Sub-project 2: Billing UI (frontend) — Design

**Date:** 2026-07-03
**Status:** Approved, ready for planning
**Surface:** `web/` only — money helper; `api/types.ts` + `api/client.ts` (Invoice + billing fns); admin `PlansPage` (price fields) + a new admin `InvoicesPage`; `AdminNav` (Factures link); a new dev `BillingPage` + `/billing` route + TopBar nav; `SubscribeModal` (plan price); i18n catalogs.

## Problem

Monetization SP1 shipped the billing backend (priced plans, invoices on paid
approval, a manual provider, admin pay/void, a team-scoped invoice read API) but
it is **invisible and partly unusable from the UI**: plans show no price, there is
no billing page, admins can't settle invoices from the app, and — because SP1 made
plan `currency` mandatory server-side while the admin plan form still sends none —
**admin plan create/edit now 400s**. SP2 is the frontend that surfaces billing and
restores plan management.

## Decomposition (whole monetization feature — for context)

- SP1 — priced plans + billing ledger (backend) **[DONE, merged]**.
- **SP2 — billing UI (frontend) [THIS spec].**
- SP3 (later) — real PSP/Stripe + webhooks + recurring + usage-overage +
  suspend-on-unpaid + developer self-serve checkout.

## Locked decisions (from brainstorming)

- **Frontend-only.** No backend change. Developers get a **read-only** team
  billing page; **admins settle** invoices (the existing admin pay/void endpoints).
  Developer self-serve checkout is SP3 (needs a real PSP).
- Money is displayed with **`Intl.NumberFormat`** (locale-aware, from the active
  i18n language) over the integer-cents backend values. `priceCents === 0` renders
  as **free** ("Gratuit"), never "0,00 €".
- All new copy goes through the existing i18n catalogs (fr/en).
- Plans are **global** (not per-product), so pricing appears where a plan is
  *chosen* or *managed* — not as a "from €X" on catalog product cards.

## Money formatting

- A new shared `web/src/money.ts` (money is used across admin/billing/subscribe,
  so it lives at the top level, not under `pages/application`) exports
  `formatMoney(cents: number, currency: string, lang: 'fr'|'en'): string` =
  `new Intl.NumberFormat(lang === 'en' ? 'en-US' : 'fr-FR', { style: 'currency',
  currency }).format(cents / 100)`, plus a `useFormatMoney()` hook binding the
  active `lang` (mirrors the existing `useFormatDate()`).
- A `priceLabel(plan, lang, t)` convenience returns `t('billing.free')` when
  `priceCents === 0`, else `formatMoney(...) + t('billing.perMonthSuffix')`
  (e.g. `29,00 €/mois`).

## Types + client — `api/types.ts` / `api/client.ts`

- Extend the TS `Plan` type with `priceCents: number` + `currency: string`
  (SP1 already returns them; the type currently omits them — **this is why the
  admin plan payload sends no currency and 400s**).
- New `Invoice` type mirroring the backend JSON: `{ id: number; teamId: number;
  subscriptionId: number | null; planName: string; priceCents: number; currency:
  string; status: 'pending'|'paid'|'void'; createdAt: string; paidAt: string | null }`.
- Client fns (reuse `langHeaders`/`sendAuthed`/`parse`):
  - `getBillingInvoices(token): Promise<Invoice[]>` → `GET /api/billing/invoices`.
  - `adminGetInvoices(token, status?): Promise<Invoice[]>` → `GET /api/admin/invoices?status=`.
  - `adminPayInvoice(token, id): Promise<void>` → `POST /api/admin/invoices/{id}/pay`.
  - `adminVoidInvoice(token, id): Promise<void>` → `POST /api/admin/invoices/{id}/void`.

## Admin

### PlansPage — price fields (restores create/edit)

`web/src/pages/admin/PlansPage.tsx`: the form state + payload gain
`priceCents` (a number input; default `0`) and `currency` (default `'EUR'`). Edit
prefills from the plan. The payload becomes
`{ id, name, rateLimit, windowSeconds, priceCents, currency }`. A row shows the
price (via `priceLabel`) alongside the rate limit. This alone fixes the current
create/edit `400` (the backend requires a valid 3-letter currency).

### InvoicesPage — settle invoices

New `web/src/pages/admin/InvoicesPage.tsx` at **`/admin/invoices`**, wrapped in
`<AdminGuard>`; a **Factures** item added to `AdminNav`. Lists all invoices via
`adminGetInvoices(token, status)`, with a status filter (all / pending / paid /
void). Each row: plan name, formatted amount, team id, status pill, created date
(+ paid date when paid). **Payer** and **Annuler** buttons on `pending` rows call
`adminPayInvoice`/`adminVoidInvoice` then reload; the backend error (409/404) is
surfaced inline via `role="alert"`. Non-pending rows show no actions.

## Developer

### BillingPage — read-only team invoices

New `web/src/pages/billing/BillingPage.tsx` at **`/billing`**; a **Facturation**
TopBar nav link, shown only when logged in (like Applications/Teams). Lists the
caller's team invoices via `getBillingInvoices(token)`: plan name, formatted
amount, a **status pill** (En attente / Payée / Annulée), created date. Empty
state: "Aucune facture". Read-only (no pay button — settlement is admin-side in
SP1). A short note explains invoices are issued when a paid subscription is
approved.

### SubscribeModal — show the price

`web/src/components/SubscribeModal.tsx`: each plan `<option>` shows its price —
`{name} — {rateLimit}/{windowSeconds}s · {priceLabel}` (`Gratuit` when free) — so
the cost is visible at subscribe time. No behavior change; the chosen plan id is
unchanged.

## Styling

Reuse the Atlas tokens + existing patterns (the admin list/table styles for
InvoicesPage; the appdetail/teams card look for BillingPage). Status pills reuse
the existing pill convention (like the subscription `statusLabel` pills). A small
scoped stylesheet only if needed; prefer existing classes.

## Testing

### Unit (vitest)

- `formatMoney`: `2900,'EUR','fr'` → `29,00 €` (or the runtime's fr grouping);
  `'EUR','en'` → `€29.00`; a `priceLabel` free case → "Gratuit".
- **BillingPage:** renders invoices with the right pills + formatted amounts;
  empty → "Aucune facture"; calls `getBillingInvoices` with the token.
- **InvoicesPage:** lists invoices, the status filter re-queries, **Payer**/
  **Annuler** call the right client fns and reload; a rejected pay surfaces the
  error; non-pending rows have no actions.
- **PlansPage:** the create/edit payload includes `priceCents` + `currency`
  (default `0`/`EUR`); edit prefills them; a row shows the price.
- **SubscribeModal:** a priced plan option shows the formatted price; a free plan
  shows "Gratuit".
- Existing tests that assert the old SubscribeModal option text / PlansPage payload
  shape are updated to the new shape.

### Live (controller)

As **admin**: price a plan (Prix + Devise) → create succeeds (was 400); open
`/admin/invoices` → see invoices, filter by status, **Payer** a pending one →
flips to Payée (+ paid date), **Annuler** another → Annulée. As a **developer**:
`SubscribeModal` shows plan prices; after an admin approves a paid subscription,
`/billing` shows that invoice with the En attente pill and correct amount; a user
in another team does not see it. Toggle FR/EN → amounts + labels localize.
**Look at the billing page and the admin invoices page in the browser.**

## Out of scope (this sub-project)

- Developer self-serve pay/checkout (SP3 — needs a real PSP + a new backend
  endpoint); recurring/overage UI; suspend-on-unpaid banners.
- Per-product "from €X" pricing on catalog cards (plans are global).
- Any backend change (the read-only team endpoint stays read-only).
- Invoice PDF/print, downloadable receipts, billing-address/tax fields.
