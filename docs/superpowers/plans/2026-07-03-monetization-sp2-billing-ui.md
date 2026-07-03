# Monetization SP2 — Billing UI (frontend) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Surface billing in the UI — priced plans, an admin invoices page (pay/void), and a read-only developer billing page — reusing the SP1 backend.

**Architecture:** A shared locale-aware money formatter; the TS `Plan` type gains price + an `Invoice` type + 4 billing client fns; the admin `PlansPage` gains price fields (which also fixes a create/edit 400), a new admin `InvoicesPage`, a new dev `BillingPage`, and priced `SubscribeModal` options. All copy through the existing fr/en i18n catalogs.

**Tech Stack:** React 19 + TS + Vite + react-router + vitest, in `web/`. No backend change.

## Global Constraints

- Frontend-only. No backend change. Developers get a **read-only** billing page; **admins settle** via the existing `POST /api/admin/invoices/{id}/pay|void`.
- Money is displayed via `Intl.NumberFormat(lang==='en'?'en-US':'fr-FR', { style:'currency', currency })` over integer cents (`cents/100`). **`priceCents === 0` renders as free** ("Gratuit"), never "0,00 €".
- All new user-facing copy goes through the i18n catalogs `web/src/i18n/fr.ts` + `web/src/i18n/en.ts` (nested namespaces; a missing/mismatched key is a `tsc` error via `Messages = typeof fr`, and a runtime parity test exists). Add matching `fr`/`en` keys in the SAME task.
- Verify each task: `cd web && pnpm exec vitest run <files> --no-file-parallelism && pnpm build`. **`tsc --noEmit` in web/ is a NO-OP — the real typecheck is `pnpm build` (`tsc -b && vite build`).**
- The billing endpoints return a **plain `Invoice[]`** (NOT `Paginated<>`), unlike the admin subscription list.

---

## Task 1: Money formatter + types + client fns

**Files:**
- Create: `web/src/money.ts`, `web/src/money.test.ts`
- Modify: `web/src/api/types.ts`, `web/src/api/client.ts`

**Interfaces:**
- Produces: `formatMoney(cents, currency, lang)`, `useFormatMoney()`, `priceLabel(cents, currency, lang, freeLabel, perSuffix?)`; `Plan.priceCents`/`.currency`; `Invoice`; `getBillingInvoices`/`adminGetInvoices`/`adminPayInvoice`/`adminVoidInvoice`.

- [ ] **Step 1: Write the failing money test**

Create `web/src/money.test.ts`:
```ts
import { describe, it, expect } from 'vitest'
import { formatMoney, priceLabel } from './money'

describe('money', () => {
  it('formats cents as localized currency', () => {
    expect(formatMoney(2900, 'EUR', 'en')).toBe('€29.00')
    // fr-FR uses a non-breaking space + comma; assert the pieces to avoid whitespace flakiness
    const fr = formatMoney(2900, 'EUR', 'fr')
    expect(fr).toMatch(/29,00/)
    expect(fr).toContain('€')
  })
  it('priceLabel returns the free label for 0', () => {
    expect(priceLabel(0, 'EUR', 'fr', 'Gratuit', '/mois')).toBe('Gratuit')
  })
  it('priceLabel appends the per-suffix for a nonzero price', () => {
    expect(priceLabel(2900, 'EUR', 'en', 'Free', '/mo')).toBe('€29.00/mo')
  })
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && pnpm exec vitest run src/money.test.ts --no-file-parallelism`
Expected: FAIL — `./money` missing.

- [ ] **Step 3: Implement `money.ts`**

Create `web/src/money.ts`:
```ts
import { useLang } from './i18n/LanguageProvider'

export function formatMoney(cents: number, currency: string, lang: 'fr' | 'en'): string {
  return new Intl.NumberFormat(lang === 'en' ? 'en-US' : 'fr-FR', {
    style: 'currency',
    currency,
  }).format(cents / 100)
}

// priceLabel renders a plan price: the free label when cents===0, else the
// formatted amount plus an optional per-period suffix. Callers pass the i18n
// strings so this stays framework-agnostic.
export function priceLabel(cents: number, currency: string, lang: 'fr' | 'en', freeLabel: string, perSuffix = ''): string {
  return cents === 0 ? freeLabel : formatMoney(cents, currency, lang) + perSuffix
}

export function useFormatMoney() {
  const { lang } = useLang()
  return (cents: number, currency: string) => formatMoney(cents, currency, lang)
}
```

- [ ] **Step 4: Extend types**

In `web/src/api/types.ts`, add to `Plan` + add `Invoice`:
```ts
export interface Plan {
  id: number
  name: string
  rateLimit: number
  windowSeconds: number
  priceCents: number
  currency: string
}

export interface Invoice {
  id: number
  teamId: number
  subscriptionId: number | null
  planName: string
  priceCents: number
  currency: string
  status: 'pending' | 'paid' | 'void'
  createdAt: string
  paidAt: string | null
}
```

- [ ] **Step 5: Add the billing client fns**

In `web/src/api/client.ts`, add (reusing the existing `parse`, `langHeaders`, `sendAuthed`; import `Invoice` in the types import):
```ts
// --- Billing ---
export async function getBillingInvoices(token: string): Promise<Invoice[]> {
  return parse<Invoice[]>(await fetch('/api/billing/invoices', { headers: langHeaders(token) }), '/api/billing/invoices')
}
export async function adminGetInvoices(token: string, status?: string): Promise<Invoice[]> {
  const url = status ? `/api/admin/invoices?status=${encodeURIComponent(status)}` : '/api/admin/invoices'
  return parse<Invoice[]>(await fetch(url, { headers: langHeaders(token) }), url)
}
export async function adminPayInvoice(token: string, id: number): Promise<void> {
  return sendAuthed('POST', `/api/admin/invoices/${id}/pay`, token)
}
export async function adminVoidInvoice(token: string, id: number): Promise<void> {
  return sendAuthed('POST', `/api/admin/invoices/${id}/void`, token)
}
```

- [ ] **Step 6: Run test + build**

Run: `cd web && pnpm exec vitest run src/money.test.ts --no-file-parallelism && pnpm build`
Expected: money test PASS. **`pnpm build` will FAIL** — adding required `priceCents`/`currency` to `Plan` breaks `PlansPage.tsx` + `SubscribeModal.tsx` (they build `Plan` objects without the new fields) and any `Plan` fixtures. That is expected and fixed in Tasks 2 + 4. To keep Task 1 independently green, ALSO do the minimal type-satisfying edits now: in `PlansPage.tsx`'s payload literal add `priceCents: 0, currency: 'EUR'` (Task 2 makes them real inputs), and in any test/fixture constructing a `Plan`, add the two fields. Re-run `pnpm build` → PASS.

- [ ] **Step 7: Commit**

```bash
git add web/src/money.ts web/src/money.test.ts web/src/api/types.ts web/src/api/client.ts web/src/pages/admin/PlansPage.tsx
git commit -m "feat(billing-ui): money formatter + Invoice type + billing client fns"
```

---

## Task 2: Admin PlansPage price fields (restores create/edit)

**Files:**
- Modify: `web/src/pages/admin/PlansPage.tsx`
- Test: `web/src/pages/admin/PlansPage.test.tsx` (add/adjust)
- i18n: `web/src/i18n/fr.ts`, `web/src/i18n/en.ts`

**Interfaces:**
- Consumes: `Plan.priceCents`/`.currency` (Task 1); `priceLabel`/`useFormatMoney` (Task 1).
- Produces: the plan create/edit payload now carries `priceCents` + `currency`.

- [ ] **Step 1: Write/adjust the failing test**

In `web/src/pages/admin/PlansPage.test.tsx`, add a test that fills the form (including the new Prix + Devise inputs), submits, and asserts `adminCreatePlan` was called with a payload containing `priceCents` + `currency`; and that opening edit on a priced plan prefills those inputs. (Match the file's existing render/mock harness — it mocks `../../api/client`.) Example assertion core:
```ts
expect(createSpy).toHaveBeenCalledWith(expect.any(String),
  expect.objectContaining({ name: 'Gold', rateLimit: 100, windowSeconds: 60, priceCents: 2900, currency: 'EUR' }))
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && pnpm exec vitest run src/pages/admin/PlansPage.test.tsx --no-file-parallelism`
Expected: FAIL — no Prix/Devise inputs; payload lacks the fields.

- [ ] **Step 3: Add price state + inputs + payload + prefill + row**

In `web/src/pages/admin/PlansPage.tsx`:
- Add state: `const [priceCents, setPriceCents] = useState(0)` + `const [currency, setCurrency] = useState('EUR')`.
- In `openEdit(p)`, add `setPriceCents(p.priceCents); setCurrency(p.currency)`. In the "new plan" open path, reset them to `0`/`'EUR'`.
- Change the payload to:
  ```ts
  const payload: Plan = { id: editing?.id ?? 0, name: name.trim(), rateLimit: limit || 100, windowSeconds: windowS || 60, priceCents: priceCents || 0, currency: (currency || 'EUR').toUpperCase() }
  ```
- Add two form controls next to rateLimit/windowSeconds (reuse the existing input classes), labelled via new i18n keys:
  ```tsx
  <label>{t('admin.plan.priceLabel')}
    <input type="number" min={0} value={priceCents} onChange={e => setPriceCents(Number(e.target.value))} />
  </label>
  <label>{t('admin.plan.currencyLabel')}
    <input value={currency} onChange={e => setCurrency(e.target.value.toUpperCase())} maxLength={3} />
  </label>
  ```
- In the plan row (near the rate-limit display, ~line 131), show the price:
  ```tsx
  <span className="price">{priceLabel(p.priceCents, p.currency, lang, t('billing.free'), t('billing.perMonthSuffix'))}</span>
  ```
  Add `import { priceLabel } from '../../money'` + `const { lang } = useLang()` (import `useLang` from `../../i18n/LanguageProvider`).

- [ ] **Step 4: Add the i18n keys**

Add to BOTH `web/src/i18n/fr.ts` and `web/src/i18n/en.ts`, in the `admin` namespace add a `plan` sub-object (or flat keys matching the codebase's convention) + a `billing` namespace:
- `admin.plan.priceLabel`: fr `"Prix (centimes)"` / en `"Price (cents)"`
- `admin.plan.currencyLabel`: fr `"Devise"` / en `"Currency"`
- `billing.free`: fr `"Gratuit"` / en `"Free"`
- `billing.perMonthSuffix`: fr `"/mois"` / en `"/mo"`
(Match the existing nesting shape of `fr.ts`; keep `en.ts` in exact key parity or `tsc -b` fails.)

- [ ] **Step 5: Run tests + build**

Run: `cd web && pnpm exec vitest run src/pages/admin/PlansPage.test.tsx --no-file-parallelism && pnpm build`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src/pages/admin/PlansPage.tsx web/src/pages/admin/PlansPage.test.tsx web/src/i18n/fr.ts web/src/i18n/en.ts
git commit -m "feat(billing-ui): admin plan price + currency fields (restores plan create)"
```

---

## Task 3: Admin InvoicesPage + route + subnav link

**Files:**
- Create: `web/src/pages/admin/InvoicesPage.tsx`, `web/src/pages/admin/InvoicesPage.test.tsx`
- Modify: `web/src/App.tsx`, `web/src/pages/admin/AdminShell.tsx`
- i18n: `web/src/i18n/fr.ts`, `web/src/i18n/en.ts`

**Interfaces:**
- Consumes: `adminGetInvoices`/`adminPayInvoice`/`adminVoidInvoice` (Task 1); `useFormatMoney`/`priceLabel` (Task 1); `AdminGuard`; `AdminShell` (`active`/`title` props).
- Produces: route `/admin/invoices`; an `active="invoices"` subnav entry.

- [ ] **Step 1: Write the failing test**

Create `web/src/pages/admin/InvoicesPage.test.tsx` (mock `../../api/client`): render inside a `MemoryRouter`; assert it lists a seeded invoice (plan name + formatted amount + a status label); clicking the status filter re-calls `adminGetInvoices` with the status; clicking **Payer** on a pending row calls `adminPayInvoice(token, id)` then reloads; a rejected `adminPayInvoice` (throws `ApiError`) surfaces the message via `role="alert"`; a `paid` row shows NO Payer/Annuler buttons. Match the mock/harness style of `PlansPage.test.tsx` / `ApprovalsPage.test.tsx`.

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && pnpm exec vitest run src/pages/admin/InvoicesPage.test.tsx --no-file-parallelism`
Expected: FAIL — page missing.

- [ ] **Step 3: Implement `InvoicesPage.tsx`**

Create `web/src/pages/admin/InvoicesPage.tsx` (mirror `ApprovalsPage`/`PlansPage` structure — `useAuth` token, `useT`, `AdminShell` wrapper, load-on-mount, error state):
```tsx
import { useEffect, useState } from 'react'
import { useAuth } from '../../auth/AuthProvider'
import { useT, useLang } from '../../i18n/LanguageProvider'
import { useFormatMoney } from '../../money'
import { formatDate } from '../application/helpers'
import { adminGetInvoices, adminPayInvoice, adminVoidInvoice, ApiError } from '../../api/client'
import type { Invoice } from '../../api/types'
import AdminShell from './AdminShell'

const STATUSES = ['', 'pending', 'paid', 'void'] as const

export default function InvoicesPage() {
  const { token } = useAuth()
  const t = useT(); const { lang } = useLang()
  const money = useFormatMoney()
  const [invoices, setInvoices] = useState<Invoice[]>([])
  const [status, setStatus] = useState<string>('')
  const [error, setError] = useState('')

  async function reload(s: string) {
    if (!token) return
    try { setInvoices(await adminGetInvoices(token, s || undefined)); setError('') }
    catch (e) { setError(e instanceof ApiError ? e.message : String(e)) }
  }
  useEffect(() => { reload(status) /* eslint-disable-next-line */ }, [token, status])

  async function act(fn: (tk: string, id: number) => Promise<void>, id: number) {
    if (!token) return
    try { await fn(token, id); await reload(status) }
    catch (e) { setError(e instanceof ApiError ? e.message : String(e)) }
  }

  const label = (st: string) => t(`billing.status.${st}`)

  return (
    <AdminShell active="invoices" title={t('billing.admin.title')} description={t('billing.admin.desc')}>
      {error && <p className="err" role="alert">{error}</p>}
      <div className="filters">
        {STATUSES.map(s => (
          <button key={s || 'all'} className={status === s ? 'chip active' : 'chip'} onClick={() => setStatus(s)}>
            {s ? label(s) : t('billing.filterAll')}
          </button>
        ))}
      </div>
      {invoices.length === 0 ? <p className="empty">{t('billing.admin.none')}</p> : (
        <table className="invoices">
          <thead><tr>
            <th>{t('billing.col.plan')}</th><th>{t('billing.col.amount')}</th>
            <th>{t('billing.col.team')}</th><th>{t('billing.col.status')}</th>
            <th>{t('billing.col.created')}</th><th></th>
          </tr></thead>
          <tbody>
            {invoices.map(inv => (
              <tr key={inv.id}>
                <td>{inv.planName}</td>
                <td>{money(inv.priceCents, inv.currency)}</td>
                <td>{inv.teamId}</td>
                <td><span className={`pill ${inv.status}`}>{label(inv.status)}</span></td>
                <td>{formatDate(inv.createdAt, lang)}</td>
                <td>
                  {inv.status === 'pending' && (
                    <>
                      <button className="btn-sm" onClick={() => act(adminPayInvoice, inv.id)}>{t('billing.admin.pay')}</button>
                      <button className="btn-sm ghost" onClick={() => act(adminVoidInvoice, inv.id)}>{t('billing.admin.void')}</button>
                    </>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </AdminShell>
  )
}
```

- [ ] **Step 4: Add the route + subnav link + i18n**

- `web/src/App.tsx`: add `import InvoicesPage from './pages/admin/InvoicesPage'` and a route beside the other admin routes:
  ```tsx
  <Route path="/admin/invoices" element={<AdminGuard><InvoicesPage /></AdminGuard>} />
  ```
- `web/src/pages/admin/AdminShell.tsx`: add a subnav link after the approvals one:
  ```tsx
  <Link className={active === 'invoices' ? 'active' : ''} to="/admin/invoices">{t('admin.invoicesNavLabel')}</Link>
  ```
  (No badge needed. **If `AdminShell`'s `active` prop is a typed union** like `'products' | 'plans' | 'approvals'`, add `'invoices'` to that union or `tsc -b` fails.)
- Add i18n keys to BOTH catalogs:
  - `admin.invoicesNavLabel`: fr `"Factures"` / en `"Invoices"`
  - `billing.admin.title`: fr `"Factures"` / en `"Invoices"`; `billing.admin.desc`: fr `"Gérez les factures d'abonnement."` / en `"Manage subscription invoices."`
  - `billing.admin.none`: fr `"Aucune facture."` / en `"No invoices."`
  - `billing.admin.pay`: fr `"Payer"` / en `"Pay"`; `billing.admin.void`: fr `"Annuler"` / en `"Void"`
  - `billing.filterAll`: fr `"Toutes"` / en `"All"`
  - `billing.status.pending`: fr `"En attente"` / en `"Pending"`; `billing.status.paid`: fr `"Payée"` / en `"Paid"`; `billing.status.void`: fr `"Annulée"` / en `"Void"`
  - `billing.col.plan`/`amount`/`team`/`status`/`created`: fr `"Offre"`/`"Montant"`/`"Équipe"`/`"Statut"`/`"Créée"` — en `"Plan"`/`"Amount"`/`"Team"`/`"Status"`/`"Created"`

- [ ] **Step 5: Run tests + build**

Run: `cd web && pnpm exec vitest run src/pages/admin/InvoicesPage.test.tsx src/pages/admin/AdminShell.test.tsx --no-file-parallelism && pnpm build`
Expected: PASS. (If `AdminShell.test.tsx` asserts an exact set of subnav links, update it for the new Factures link.)

- [ ] **Step 6: Commit**

```bash
git add web/src/pages/admin/InvoicesPage.tsx web/src/pages/admin/InvoicesPage.test.tsx web/src/App.tsx web/src/pages/admin/AdminShell.tsx web/src/i18n/fr.ts web/src/i18n/en.ts
git commit -m "feat(billing-ui): admin invoices page (list, filter, pay/void)"
```

---

## Task 4: Dev BillingPage + nav + priced SubscribeModal

**Files:**
- Create: `web/src/pages/billing/BillingPage.tsx`, `web/src/pages/billing/BillingPage.test.tsx`
- Modify: `web/src/App.tsx`, `web/src/components/TopBar.tsx`, `web/src/components/SubscribeModal.tsx`
- Test: `web/src/components/SubscribeModal.test.tsx` (adjust)
- i18n: `web/src/i18n/fr.ts`, `web/src/i18n/en.ts`

**Interfaces:**
- Consumes: `getBillingInvoices` (Task 1); `useFormatMoney`/`priceLabel` (Task 1); the `billing.status.*`/`billing.col.*`/`billing.free`/`billing.perMonthSuffix` keys (Tasks 2-3).
- Produces: route `/billing`; a "Facturation" TopBar link.

- [ ] **Step 1: Write the failing tests**

Create `web/src/pages/billing/BillingPage.test.tsx` (mock `../../api/client`): with `getBillingInvoices` resolving to one pending invoice, assert the plan name + formatted amount + the "En attente" pill render; with it resolving to `[]`, assert the empty state "Aucune facture" shows. Also adjust `web/src/components/SubscribeModal.test.tsx`: a plan with `priceCents: 2900` shows a formatted price in its option; a `priceCents: 0` plan shows "Gratuit".

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && pnpm exec vitest run src/pages/billing/BillingPage.test.tsx src/components/SubscribeModal.test.tsx --no-file-parallelism`
Expected: FAIL — page missing / option has no price.

- [ ] **Step 3: Implement `BillingPage.tsx`**

Create `web/src/pages/billing/BillingPage.tsx`:
```tsx
import { useEffect, useState } from 'react'
import { useAuth } from '../../auth/AuthProvider'
import { useT, useLang } from '../../i18n/LanguageProvider'
import { useFormatMoney } from '../../money'
import { formatDate } from '../application/helpers'
import { getBillingInvoices, ApiError } from '../../api/client'
import type { Invoice } from '../../api/types'

export default function BillingPage() {
  const { token } = useAuth()
  const t = useT(); const { lang } = useLang()
  const money = useFormatMoney()
  const [invoices, setInvoices] = useState<Invoice[]>([])
  const [error, setError] = useState('')

  useEffect(() => {
    if (!token) return
    getBillingInvoices(token).then(setInvoices).catch(e => setError(e instanceof ApiError ? e.message : String(e)))
  }, [token])

  return (
    <div className="billing-page">
      <h1>{t('billing.title')}</h1>
      <p className="hint">{t('billing.hint')}</p>
      {error && <p className="err" role="alert">{error}</p>}
      {invoices.length === 0 ? <p className="empty">{t('billing.none')}</p> : (
        <table className="invoices">
          <thead><tr>
            <th>{t('billing.col.plan')}</th><th>{t('billing.col.amount')}</th>
            <th>{t('billing.col.status')}</th><th>{t('billing.col.created')}</th>
          </tr></thead>
          <tbody>
            {invoices.map(inv => (
              <tr key={inv.id}>
                <td>{inv.planName}</td>
                <td>{money(inv.priceCents, inv.currency)}</td>
                <td><span className={`pill ${inv.status}`}>{t(`billing.status.${inv.status}`)}</span></td>
                <td>{formatDate(inv.createdAt, lang)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}
```

- [ ] **Step 4: Route + nav link + i18n**

- `web/src/App.tsx`: `import BillingPage from './pages/billing/BillingPage'` + `<Route path="/billing" element={<BillingPage />} />` (it self-guards on `token`; keep it inside the authed area — mirror how `/applications` is placed).
- `web/src/components/TopBar.tsx`: add a nav link after Teams (user-gated), mirroring line ~200:
  ```tsx
  {user && <Link className={tab(pathname.startsWith('/billing'))} to="/billing"><IconDoc />{t('nav.billing')}</Link>}
  ```
- Add i18n keys to BOTH catalogs:
  - `nav.billing`: fr `"Facturation"` / en `"Billing"`
  - `billing.title`: fr `"Facturation"` / en `"Billing"`
  - `billing.hint`: fr `"Une facture est émise lorsqu'un abonnement payant est approuvé."` / en `"An invoice is issued when a paid subscription is approved."`
  - `billing.none`: fr `"Aucune facture."` / en `"No invoices."`

- [ ] **Step 5: Price the SubscribeModal options**

In `web/src/components/SubscribeModal.tsx`, add `import { priceLabel } from '../money'` + `import { useLang } from '../i18n/LanguageProvider'` + `const { lang } = useLang()`; change the plan `<option>` (~line 81) to include the price:
```tsx
{plans.map(p => (
  <option key={p.id} value={p.id}>
    {p.name} — {p.rateLimit}/{p.windowSeconds}s · {priceLabel(p.priceCents, p.currency, lang, t('billing.free'), t('billing.perMonthSuffix'))}
  </option>
))}
```

- [ ] **Step 6: Run tests + build**

Run: `cd web && pnpm exec vitest run src/pages/billing/BillingPage.test.tsx src/components/SubscribeModal.test.tsx --no-file-parallelism && pnpm build`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add web/src/pages/billing/ web/src/App.tsx web/src/components/TopBar.tsx web/src/components/SubscribeModal.tsx web/src/i18n/fr.ts web/src/i18n/en.ts
git commit -m "feat(billing-ui): developer billing page + Facturation nav + priced subscribe options"
```

---

## Task 5: Full suite + live verification

**Files:** none (verification; small CSS only if a surface is visibly unstyled).

- [ ] **Step 1: Full frontend suite + build**

Run: `cd web && pnpm exec vitest run --exclude 'e2e/**' --no-file-parallelism && pnpm build`
Expected: all green. Also `grep -rn "priceCents\|currency" web/src/api/types.ts` to confirm the types shipped.

- [ ] **Step 2: Live**

Bring up the stack (`make up`; `make run` on `PORTAL_ADDR` if `:8080` is held; `docker compose up -d etcd apisix` so approval provisioning works; vite). In the browser:
1. As **admin** → `/admin/plans`: create a plan with **Prix 2900** + **Devise EUR** → succeeds (this was 400 before SP2); the row shows `29,00 €/mois`.
2. As a **developer**: open a product's `SubscribeModal` → plan options show prices (and `Gratuit` for a free plan).
3. Subscribe an app to the paid plan; as **admin** approve it.
4. `/admin/invoices`: the invoice appears; filter pending/paid/void; **Payer** it → moves to Payée; **Annuler** another → Annulée; a re-Payer shows the 409 error inline.
5. As that **developer**: `/billing` (via the Facturation nav) shows the invoice with the En attente/Payée pill + the localized amount; a user in another team sees none.
6. Toggle **FR/EN** → amounts + labels localize (`29,00 €` ↔ `€29.00`, En attente ↔ Pending).
**Look at `/billing` and `/admin/invoices` in the browser, in both languages.**

- [ ] **Step 3: No commit** (verification; note results in the ledger).

---

## Self-Review notes

- **Spec coverage:** money formatter + `useFormatMoney` + `priceLabel` (T1) ✅; `Plan` price + `Invoice` type + 4 client fns (T1) ✅; admin PlansPage price fields restoring create (T2) ✅; admin InvoicesPage + `/admin/invoices` + Factures subnav + pay/void (T3) ✅; dev BillingPage + `/billing` + Facturation nav (T4) ✅; priced SubscribeModal (T4) ✅; i18n fr/en for all copy (T2-T4) ✅; unit + live incl per-locale (T1-T5) ✅.
- **Type consistency:** `Invoice` (T1) shape is consumed unchanged by T3/T4; `Plan.priceCents`/`.currency` (T1) used by T2/T4; `useFormatMoney()(cents, currency)` + `priceLabel(cents, currency, lang, free, suffix)` signatures fixed in T1 and used verbatim later; the `billing.status.*`/`billing.col.*`/`billing.free`/`billing.perMonthSuffix` keys are introduced in T2/T3 and reused by T4 (add each key once, in the first task that needs it).
- **Implementer notes:** `pnpm build` is the ONLY real typecheck (bare `tsc --noEmit` is a no-op in web/). The billing endpoints return a bare `Invoice[]`, not `Paginated`. Adding required fields to `Plan` (T1) is a deliberate compile-break that T1 Step 6 resolves minimally and T2/T4 complete — the i18n catalog keys must land in BOTH `fr.ts` and `en.ts` or `tsc -b` fails on the `Messages` parity type. If a stale vite dev cache throws phantom "missing export" errors during live, `rm -rf node_modules/.vite` and restart.
