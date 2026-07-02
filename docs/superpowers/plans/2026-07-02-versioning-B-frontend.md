# API Versioning / Changelog — Plan B (Frontend) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Surface the lifecycle status + changelog in the UI — catalog/detail status badges + notices, a changelog timeline, an admin status selector + changelog editor, and a disabled-subscribe with reason — against the shipped versioning backend.

**Architecture:** One small backend tweak (expose the changelog entry `id` on the read shape so the admin editor can delete). Then frontend types + client fns; developer-facing surfacing (badge/notice/timeline/disabled-subscribe); and the admin Composer status selector + changelog editor. React 19 + TS + Vite + vitest in `web/`; the backend tweak is Go.

**Tech Stack:** Go 1.25 (catalog), React 19, TypeScript, react-router-dom, vitest + @testing-library/react. French copy, existing Atlas CSS.

## Global Constraints

- Frontend under `web/` (verify with `pnpm exec vitest run --exclude 'e2e/**'` + `pnpm exec tsc --noEmit`); the one backend change verifies with `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/catalog/`.
- Backend contract (already shipped): `Product`/`AdminProduct` responses carry `lifecycleStatus` (`active|deprecated|sunset`) + `sunsetDate` (ISO `YYYY-MM-DD` or null). `GET /api/products/{slug}/changelog` → `[{version,kind,notes,date}]` newest-first (this plan adds `id`). `POST /api/admin/products/{id}/changelog {version,kind,notes,date}` → the created entry (incl. `id`); `DELETE /api/admin/products/{id}/changelog/{entryId}` → 204. `adminCreateProduct`/`adminUpdateProduct` already send the whole `AdminProduct`, so adding `lifecycleStatus`/`sunsetDate` to that type carries them.
- Statuses: `active` (no badge), `deprecated` (badge + "n'accepte plus de nouveaux abonnements" + subscribe disabled), `sunset` (badge + "sera retirée le {sunsetDate}" + subscribe disabled). Kind tags: `added|changed|fixed|removed|deprecated|security`.
- New type fields are **optional** (`?`) — no fixture churn (established pattern).
- French copy; reuse existing Atlas classes/tokens (`.pill`, `.btn`, etc.). Client helpers to reuse: `authHeaders`/`parse`/`sendAuthed`; pages read auth via `useAuth()`.

---

## Task 1: Backend — expose changelog entry `id` on the read shape

**Files:**
- Modify: `internal/catalog/product.go` (ChangelogEntry), `internal/catalog/repo.go` (ListChangelogBySlug query)
- Test: `internal/catalog/repo_test.go` (extend)

**Interfaces:**
- Produces: catalog `ChangelogEntry` gains `ID int64` (`json:"id"`); `ListChangelogBySlug` selects `ce.id`.

- [ ] **Step 1: Extend the failing test**

In `internal/catalog/repo_test.go`, in the existing `TestListChangelogBySlug` (added in the backend plan), add an assertion that the entries carry a non-zero id:
```go
	if entries[0].ID == 0 {
		t.Errorf("changelog entry ID not populated: %+v", entries[0])
	}
```

- [ ] **Step 2: Run to verify it fails**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/catalog/ -run TestListChangelogBySlug -v`
Expected: FAIL — `ID` field undefined / zero.

- [ ] **Step 3: Add the field + select it**

In `internal/catalog/product.go`, add to `ChangelogEntry` (FIRST field, before `Version`):
```go
	ID int64 `json:"id"`
```
In `internal/catalog/repo.go` `ListChangelogBySlug`, add `ce.id` to the SELECT (first column) and scan it first:
```go
		`SELECT ce.id, ce.version, ce.kind, ce.notes, to_char(ce.entry_date,'YYYY-MM-DD')
		 FROM changelog_entries ce JOIN api_products p ON p.id = ce.product_id
		 WHERE p.slug=$1 ORDER BY ce.entry_date DESC, ce.id DESC`, slug)
	...
		if err := rows.Scan(&e.ID, &e.Version, &e.Kind, &e.Notes, &e.Date); err != nil {
```

- [ ] **Step 4: Run to verify it passes**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/catalog/ && go build ./... && go vet ./internal/catalog/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/catalog/
git commit -m "feat(versioning): expose changelog entry id on the read endpoint"
```

---

## Task 2: Frontend types + client functions

**Files:**
- Modify: `web/src/api/types.ts`, `web/src/api/client.ts`
- Test: `web/src/api/client.changelog.test.ts` (create)

**Interfaces:**
- Produces (types): `Product` + `AdminProduct` gain `lifecycleStatus?: 'active'|'deprecated'|'sunset'` + `sunsetDate?: string | null`; `ChangelogEntry { id: number; version: string; kind: string; notes: string; date: string }`.
- Produces (client): `getChangelog(slug): Promise<ChangelogEntry[]>`; `addChangelogEntry(token, productId, entry): Promise<ChangelogEntry>` (entry = `{version,kind,notes,date}`); `deleteChangelogEntry(token, productId, entryId): Promise<void>`.

- [ ] **Step 1: Write the failing test**

Create `web/src/api/client.changelog.test.ts` (mirror `client.teams.test.ts` — `vi.stubGlobal('fetch', …)`, 204 uses a `null` body):
```ts
import { describe, it, expect, vi, afterEach } from 'vitest'
import { getChangelog, addChangelogEntry, deleteChangelogEntry } from './client'

function mockFetch(body: unknown, status = 200) {
  return vi.fn(async () => new Response(status === 204 ? null : JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } }))
}
afterEach(() => vi.unstubAllGlobals())

describe('changelog client', () => {
  it('getChangelog GETs the public endpoint', async () => {
    const f = mockFetch([{ id: 1, version: 'v1', kind: 'added', notes: 'n', date: '2026-01-01' }])
    vi.stubGlobal('fetch', f)
    const entries = await getChangelog('slug-x')
    expect(entries[0].kind).toBe('added')
    expect(f).toHaveBeenCalledWith('/api/products/slug-x/changelog', expect.anything())
  })

  it('addChangelogEntry POSTs to the admin endpoint', async () => {
    const f = mockFetch({ id: 5, version: 'v2', kind: 'fixed', notes: 'p', date: '2026-02-01' }, 201)
    vi.stubGlobal('fetch', f)
    const e = await addChangelogEntry('jwt', 7, { version: 'v2', kind: 'fixed', notes: 'p', date: '2026-02-01' })
    expect(e.id).toBe(5)
    expect(f).toHaveBeenCalledWith('/api/admin/products/7/changelog', expect.objectContaining({ method: 'POST', body: JSON.stringify({ version: 'v2', kind: 'fixed', notes: 'p', date: '2026-02-01' }) }))
  })

  it('deleteChangelogEntry DELETEs the admin endpoint', async () => {
    const f = mockFetch(null, 204)
    vi.stubGlobal('fetch', f)
    await deleteChangelogEntry('jwt', 7, 5)
    expect(f).toHaveBeenCalledWith('/api/admin/products/7/changelog/5', expect.objectContaining({ method: 'DELETE' }))
  })
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && pnpm exec vitest run src/api/client.changelog.test.ts`
Expected: FAIL — fns not exported.

- [ ] **Step 3: Add the types + client fns**

In `web/src/api/types.ts`: add to BOTH `Product` and `AdminProduct`:
```ts
  lifecycleStatus?: 'active' | 'deprecated' | 'sunset'
  sunsetDate?: string | null
```
And add:
```ts
export interface ChangelogEntry {
  id: number
  version: string
  kind: string
  notes: string
  date: string
}
```

In `web/src/api/client.ts`, add `ChangelogEntry` to the `./types` import, then:
```ts
export async function getChangelog(slug: string): Promise<ChangelogEntry[]> {
  const url = `/api/products/${slug}/changelog`
  return parse<ChangelogEntry[]>(await fetch(url), url)
}

export async function addChangelogEntry(token: string, productId: number, entry: { version: string; kind: string; notes: string; date: string }): Promise<ChangelogEntry> {
  const url = `/api/admin/products/${productId}/changelog`
  return parse<ChangelogEntry>(await fetch(url, { method: 'POST', headers: authHeaders(token), body: JSON.stringify(entry) }), url)
}

export async function deleteChangelogEntry(token: string, productId: number, entryId: number): Promise<void> {
  return sendAuthed('DELETE', `/api/admin/products/${productId}/changelog/${entryId}`, token)
}
```
(`getChangelog` is public — no auth header, like `getProductSpec`.)

- [ ] **Step 4: Run to verify it passes**

Run: `cd web && pnpm exec vitest run src/api/client.changelog.test.ts && pnpm exec tsc --noEmit`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/api/types.ts web/src/api/client.ts web/src/api/client.changelog.test.ts
git commit -m "feat(web): changelog types + lifecycle fields + client fns"
```

---

## Task 3: Catalog badge + product-detail status/notice/changelog + disabled subscribe

**Files:**
- Modify: `web/src/components/ApiCard.tsx`, `web/src/pages/ProductDetailPage.tsx`, `web/src/styles/catalog.css` (badge/notice/timeline styles)
- Test: `web/src/components/ApiCard.test.tsx`, `web/src/pages/ProductDetailPage.test.tsx` (extend)

**Interfaces:**
- Consumes: `Product.lifecycleStatus`/`sunsetDate` (T2); `getChangelog` (T2); `ChangelogEntry` (T2).
- Produces: a `LifecycleBadge` inline in ApiCard + ProductDetail; a notice banner + changelog timeline + disabled subscribe on ProductDetail.

- [ ] **Step 1: Write the failing tests**

Extend `web/src/components/ApiCard.test.tsx` — a deprecated product shows a "Déprécié" badge, an active one shows none:
```tsx
it('shows a lifecycle badge for a deprecated product', () => {
  render(<MemoryRouter><ApiCard p={{ ...baseProduct, lifecycleStatus: 'deprecated' }} /></MemoryRouter>)
  expect(screen.getByText('Déprécié')).toBeInTheDocument()
})
it('shows no lifecycle badge for an active product', () => {
  render(<MemoryRouter><ApiCard p={{ ...baseProduct, lifecycleStatus: 'active' }} /></MemoryRouter>)
  expect(screen.queryByText('Déprécié')).not.toBeInTheDocument()
})
```
(Use the file's existing product fixture as `baseProduct`; read it first.)

Extend `web/src/pages/ProductDetailPage.test.tsx` — for a deprecated product, the notice renders, the changelog timeline shows an entry, and the subscribe button is disabled:
```tsx
it('shows the deprecation notice, changelog, and a disabled subscribe for a deprecated product', async () => {
  vi.spyOn(client, 'getProduct').mockResolvedValue({ ...baseProduct, slug: 'dep', lifecycleStatus: 'deprecated' })
  vi.spyOn(client, 'getChangelog').mockResolvedValue([{ id: 1, version: 'v1.2', kind: 'deprecated', notes: 'moved to v2', date: '2026-07-01' }])
  // (stub getRatings/getProductSpec/getTryContext as the existing tests do)
  renderDetail('dep')
  expect(await screen.findByText(/n'accepte plus de nouveaux abonnements/i)).toBeInTheDocument()
  expect(screen.getByText('v1.2')).toBeInTheDocument()
  expect(screen.getByText(/moved to v2/)).toBeInTheDocument()
  const subBtn = screen.getByRole('button', { name: /S'abonner/i })
  expect(subBtn).toBeDisabled()
})
```
**NOTE for the implementer:** read `ProductDetailPage.test.tsx` first to reuse its exact render helper + the full set of client stubs it already sets up (getProduct/getRatings/getProductSpec/getTryContext), and its `baseProduct` fixture. Add `getChangelog` to the stubs. Adapt queries to the real markup.

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && pnpm exec vitest run src/components/ApiCard.test.tsx src/pages/ProductDetailPage.test.tsx`
Expected: FAIL — no badge/notice/changelog/disabled-subscribe.

- [ ] **Step 3: Implement**

Add a tiny shared helper — in `web/src/components/ApiCard.tsx` (exported) or a small `web/src/components/LifecycleBadge.tsx`:
```tsx
export function LifecycleBadge({ status }: { status?: string }) {
  if (status === 'deprecated') return <span className="pill lifecycle deprecated">Déprécié</span>
  if (status === 'sunset') return <span className="pill lifecycle sunset">Sunset</span>
  return null
}
```
- **ApiCard**: render `<LifecycleBadge status={p.lifecycleStatus} />` in the card meta row (near the name/rating — read the JSX and place it alongside the existing pills).
- **ProductDetailPage**:
  - In the `.sub` line (currently `…· v{version} · {rating}{oauth pill}`), append `{product.lifecycleStatus && product.lifecycleStatus !== 'active' && <> · <LifecycleBadge status={product.lifecycleStatus} /></>}`.
  - Add a **notice banner** right under the header when non-active:
    ```tsx
    {product.lifecycleStatus === 'deprecated' && <div className="notice deprecated">Cette API est dépréciée et n'accepte plus de nouveaux abonnements.</div>}
    {product.lifecycleStatus === 'sunset' && <div className="notice sunset">Cette API sera retirée{product.sunsetDate ? ` le ${product.sunsetDate}` : ''}.</div>}
    ```
  - **Disabled subscribe**: change the S'abonner button so it's disabled for non-active status:
    ```tsx
    const blocked = product.lifecycleStatus === 'deprecated' || product.lifecycleStatus === 'sunset'
    <button className="btn btn-primary" disabled={blocked} title={blocked ? "Cette API n'accepte plus de nouveaux abonnements" : undefined}
      onClick={() => { if (blocked) return; user ? setSubOpen(true) : nav('/login') }}>S'abonner</button>
    ```
  - **Changelog timeline**: fetch `getChangelog(slug)` in an effect (`const [changelog, setChangelog] = useState<ChangelogEntry[]>([])`; `getChangelog(slug).then(setChangelog).catch(() => {})` in the existing load effect keyed on `slug`), and render a section when non-empty:
    ```tsx
    {changelog.length > 0 && (
      <section className="changelog">
        <h2>Journal des modifications</h2>
        <ul>{changelog.map(e => (
          <li key={e.id}>
            <span className={`ctag ${e.kind}`}>{e.kind}</span>
            <b>{e.version}</b> <span className="cdate mono">{e.date}</span>
            {e.notes && <p>{e.notes}</p>}
          </li>
        ))}</ul>
      </section>
    )}
    ```
- **CSS** (`web/src/styles/catalog.css`): add `.pill.lifecycle.deprecated`/`.sunset` (muted/amber tags), `.notice.deprecated`/`.sunset` (a banner with a warning tint), `.changelog` list + `.ctag` colored kind tags. Reuse existing color tokens; keep it consistent with the `.pill.oauth` styling already there.

- [ ] **Step 4: Run to verify it passes**

Run: `cd web && pnpm exec vitest run src/components/ApiCard.test.tsx src/pages/ProductDetailPage.test.tsx && pnpm exec tsc --noEmit`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/ web/src/pages/ProductDetailPage.tsx web/src/styles/catalog.css
git commit -m "feat(web): lifecycle badge + notice + changelog timeline + disabled subscribe"
```

---

## Task 4: Admin — status selector + sunset date + changelog editor

**Files:**
- Modify: `web/src/pages/admin/ProductsPage.tsx`
- Test: `web/src/pages/admin/ProductsPage.test.tsx` (extend)

**Interfaces:**
- Consumes: `AdminProduct.lifecycleStatus`/`sunsetDate` (T2); `getChangelog`/`addChangelogEntry`/`deleteChangelogEntry` (T2).
- Produces: a lifecycle status select + conditional sunset-date input in the Composer; a changelog editor (list + add + delete) shown when editing an existing product.

- [ ] **Step 1: Write the failing test**

Extend `web/src/pages/admin/ProductsPage.test.tsx` (read it first for the render helper + how it opens the Composer + how `adminCreateProduct` is spied):
```tsx
it('sends lifecycleStatus + sunsetDate when creating', async () => {
  const create = vi.spyOn(api, 'adminCreateProduct').mockResolvedValue({} as never)
  renderPage(); openComposer() // use the file's real helpers
  fireEvent.change(screen.getByLabelText(/nom/i), { target: { value: 'X' } })
  fireEvent.change(screen.getByLabelText(/context path/i), { target: { value: '/x' } })
  fireEvent.change(screen.getByLabelText(/statut/i), { target: { value: 'sunset' } })
  fireEvent.change(screen.getByLabelText(/date de retrait/i), { target: { value: '2026-12-31' } })
  fireEvent.click(screen.getByRole('button', { name: /créer|enregistrer/i }))
  await waitFor(() => expect(create.mock.calls[0][1]).toMatchObject({ lifecycleStatus: 'sunset', sunsetDate: '2026-12-31' }))
})
```
(Adapt to the file's actual helpers + field labels — this test's job is: the status select + sunset-date input feed the create payload.)

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && pnpm exec vitest run src/pages/admin/ProductsPage.test.tsx`
Expected: FAIL — no status/sunset fields.

- [ ] **Step 3: Implement**

In `web/src/pages/admin/ProductsPage.tsx`:
- Add `lifecycleStatus: string; sunsetDate: string` to `FormState` (line ~13-16) and to `EMPTY` (`lifecycleStatus: 'active', sunsetDate: ''`).
- In the edit prefill (`setForm({ … })` at line ~74) add `lifecycleStatus: p.lifecycleStatus ?? 'active', sunsetDate: p.sunsetDate ?? ''`; in the import-draft prefill (line ~65) default `lifecycleStatus: 'active', sunsetDate: ''`.
- In the `payload` (line ~82-91) add `lifecycleStatus: form.lifecycleStatus, sunsetDate: form.sunsetDate || null`.
- In the Composer form (near the auth-method / version fields), add:
  ```tsx
  <label htmlFor="f-lifecycle">Statut</label>
  <select id="f-lifecycle" className="ipt" value={form.lifecycleStatus} onChange={e => set('lifecycleStatus', e.target.value)}>
    <option value="active">Actif</option>
    <option value="deprecated">Déprécié</option>
    <option value="sunset">Sunset</option>
  </select>
  {form.lifecycleStatus === 'sunset' && (<>
    <label htmlFor="f-sunset">Date de retrait</label>
    <input id="f-sunset" type="date" className="ipt" value={form.sunsetDate} onChange={e => set('sunsetDate', e.target.value)} />
  </>)}
  ```
- **Changelog editor** — shown only when editing an existing product (`editing?.id`). Add local state + a small subcomponent that, on mount for the editing product, `getChangelog(editing.slug)` → lists entries with a "Supprimer" button (calls `deleteChangelogEntry(token, editing.id, e.id)` then refetches), plus an add form (version, kind `<select>` of the 6 kinds, date, notes → `addChangelogEntry(token, editing.id, {…})` then refetch). Render it inside the Composer below the product fields when `editing?.id != null`. Keep the markup consistent with the rest of the Composer (labels + `.ipt`). French copy ("Journal des modifications", "Ajouter", "Supprimer").

- [ ] **Step 4: Run to verify it passes**

Run: `cd web && pnpm exec vitest run src/pages/admin/ProductsPage.test.tsx && pnpm exec tsc --noEmit`
Expected: PASS (update any existing ProductsPage test that asserts the exact payload shape to tolerate the two new fields).

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/admin/ProductsPage.tsx web/src/pages/admin/ProductsPage.test.tsx
git commit -m "feat(web): admin lifecycle status selector + changelog editor"
```

---

## Task 5: Live verification (browser)

**Files:** none (verification only).

- [ ] **Step 1: Full frontend suite + typecheck + build**

Run: `cd web && pnpm exec vitest run --exclude 'e2e/**' && pnpm exec tsc --noEmit && pnpm build`
Expected: all green.

- [ ] **Step 2: Browser walkthrough**

Bring the stack up + run the portal (`:8090`) and vite (`:5173`, `PORTAL_PROXY=http://localhost:8090`). The dev DB already has a **deprecated** product (`currencyconverterapi`) with a changelog entry and a **sunset** product (`demo-orders…`) from the backend live test — use them.
1. As a developer (or logged out), open the **catalog** → the deprecated/sunset products show a status badge on their cards.
2. Open `/catalog/currencyconverterapi` → the header shows the **Déprécié** badge + the "n'accepte plus de nouveaux abonnements" notice + the **Journal des modifications** timeline (kind tag + version + date + notes); the **S'abonner** button is **disabled**.
3. Open the sunset product → the "sera retirée le 2026-12-31" notice; subscribe disabled.
4. As **admin**, open Admin → Products → edit a product → set **Statut = Déprécié** (and try Sunset → the date field appears), save; add a changelog entry via the editor → it appears; delete it → it disappears.
5. Open an **active** product as a developer → no badge/notice, subscribe works.
**Take a screenshot of the deprecated product detail (badge + notice + changelog) and the admin editor; look at them.**

- [ ] **Step 3: No commit** (verification only; note results in the progress ledger).

---

## Self-Review notes

- **Spec coverage (frontend section):** catalog card badge (T3) ✅; product-detail badge + notice + changelog timeline + disabled subscribe (T3) ✅; admin status selector + conditional sunset date + changelog editor (T4) ✅; types + client fns incl. the `id` the editor needs (T1+T2) ✅. The backend `id` exposure (T1) is the one dependency the spec's admin-delete implies but the Plan-A read shape omitted — added here.
- **Placeholder scan:** none — concrete code or read-and-match instructions with exact insertion points throughout.
- **Type consistency:** `ChangelogEntry {id,version,kind,notes,date}` is identical across the Go view (T1), the TS type (T2), and both consumers (T3 timeline, T4 editor). `lifecycleStatus?`/`sunsetDate?` names match across `Product`/`AdminProduct` (T2) and every consumer. `getChangelog(slug)`/`addChangelogEntry(token,productId,entry)`/`deleteChangelogEntry(token,productId,entryId)` signatures match their calls in T3/T4 and the T2 test. The `LifecycleBadge` is defined once (T3) and reused by ApiCard + ProductDetail.
- **Implementer notes:** read each component/test for real fixtures + helpers before adapting (ApiCard `baseProduct`, ProductDetailPage's render helper + client stubs, ProductsPage's Composer helpers + field labels + `FormState`/`EMPTY`/`payload` at the noted line numbers). Existing ProductsPage payload-shape tests may need the two new fields tolerated.
