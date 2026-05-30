# Plan 4d — Admin UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A role-gated `/admin` UI in the React app: manage products (incl. upstream + publish), manage plans, and approve/reject pending subscriptions — all against the now-complete `/api/admin/*` backend. Also surface each subscription's status on the developer Applications page.

**Architecture:** New typed admin functions in the existing `api/client.ts` (Bearer-authed, mirroring the existing authed calls). A small `AdminGuard` redirects non-admins. Three pages (`AdminProductsPage`, `AdminPlansPage`, `AdminApprovalsPage`) share an `AdminNav` subnav and reuse the existing Atlas CSS classes (`subbtn`, `tag`, `card`, `chead`, `autherr`, `pill`, etc.). The TopBar shows an "Admin" link only when `user.role === 'admin'`.

**Tech Stack:** Vite + React 19 + TypeScript, react-router-dom v7, Vitest + React Testing Library + jsdom. Tests mock the `api/client` module with `vi.spyOn`. Run from `web/`.

---

## Context the implementer needs

- **Run tests:** from `web/`, `npx vitest run` (all) or `npx vitest run src/pages/AdminProductsPage.test.tsx` (one file). Type-check/build: `npx tsc -b` then `npm run build` (or just `npx tsc --noEmit`). Dev proxy sends `/api`→:8080.
- **Auth:** `useAuth()` (`src/auth/AuthProvider.tsx`) returns `{ user, token, login, register, logout }`. `user` is `User { id, email, name, role }` (role already present). `token` is the JWT string. Tests seed `localStorage` `token` + `user` before rendering (see `ApplicationsPage.test.tsx`).
- **API client pattern** (`src/api/client.ts`): `parse<T>(res)` returns the JSON body or throws `Error(body.error || 'request failed (status)')`. `authHeaders(token)` returns `{ 'Content-Type': 'application/json', Authorization: 'Bearer '+token }`. Authed example: `getApplications(token)`. Deletes that return 204 use a manual `if (!res.ok) throw` (see `unsubscribe`).
- **Backend admin endpoints** (all behind RequireAdmin; 401 if no/!admin token, 403 if non-admin):
  - Products: `GET /api/admin/products` (all, incl. unpublished), `POST /api/admin/products`, `GET /api/admin/products/{id}`, `PUT /api/admin/products/{id}`, `DELETE /api/admin/products/{id}` (409 "product has active subscriptions" if blocked). Body/response shape = admin Product: `{id, name, slug, category, version, contextPath, description, tags[], icon, upstreamUrl, published}`.
  - Plans: `GET/POST /api/admin/plans`, `PUT/DELETE /api/admin/plans/{id}` (409 "plan is referenced by subscriptions"). Shape = `Plan {id, name, rateLimit, windowSeconds}` (already in types.ts).
  - Subscriptions: `GET /api/admin/subscriptions?status=pending` → `AdminSubscriptionView { id, applicationName, ownerEmail, productName, version, planName, status, createdAt }`; `POST /api/admin/subscriptions/{id}/approve` (204); `POST /api/admin/subscriptions/{id}/reject` (204).
- **Routing** (`src/App.tsx`): a flat `<Routes>`. `main.tsx` wraps in `BrowserRouter > ThemeProvider > AuthProvider`. Tests render pages inside `<MemoryRouter><ThemeProvider><AuthProvider>…`. Use `Navigate` / `useNavigate` from react-router-dom v7.
- **Existing CSS classes** (`src/styles/catalog.css`/`base.css`): `topbar`, `nav-tabs`, `content`, `chead`, `titlewrap`, `card`, `subbtn`, `subbtn ghost`, `tag`/`tag active`, `pill`, `autherr` (role="alert"), `rescount`, `apikey`, `cfoot`, `ctx`. Inline styles are used freely elsewhere (see ApplicationsPage) — fine to reuse that approach for admin forms/tables.
- **Frontend status field:** the backend now returns `status` on each `SubscriptionView`; `types.ts` must add it and the Applications page must display it.
- **Commit trailer:** end every commit with `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`. Branch `master`.

## File structure (what this plan creates / modifies)

- **Modify** `web/src/api/types.ts` — add `AdminProduct`, `AdminSubscription`; add `status` to `SubscriptionView`.
- **Modify** `web/src/api/client.ts` — add admin product/plan/subscription functions.
- **Modify** `web/src/api/client.test.ts` — cover a representative admin call (auth header + URL + 204 handling).
- **Create** `web/src/admin/AdminGuard.tsx` — redirect non-admins.
- **Create** `web/src/admin/AdminGuard.test.tsx`.
- **Create** `web/src/components/AdminNav.tsx` — TopBar + admin subnav shared by the three pages.
- **Create** `web/src/pages/AdminProductsPage.tsx` + `.test.tsx`.
- **Create** `web/src/pages/AdminPlansPage.tsx` + `.test.tsx`.
- **Create** `web/src/pages/AdminApprovalsPage.tsx` + `.test.tsx`.
- **Modify** `web/src/components/TopBar.tsx` — admin link when role admin (+ test or assert in an existing test).
- **Modify** `web/src/App.tsx` — admin routes under the guard.
- **Modify** `web/src/pages/ApplicationsPage.tsx` + `.test.tsx` — show subscription status.

---

## Task 1: Admin API types + client functions

**Files:**
- Modify: `web/src/api/types.ts`
- Modify: `web/src/api/client.ts`
- Modify: `web/src/api/client.test.ts`

- [ ] **Step 1: Write the failing tests**

Add to `web/src/api/client.test.ts` — update the import line to include the new functions, and add a describe block:

Change the top import to:
```ts
import { getProducts, login, register, getPlans, getApplications, createApplication, getApplicationDetail, subscribe, adminGetProducts, adminCreateProduct, adminDeleteProduct, adminGetSubscriptions, adminApproveSubscription } from './client'
```

Add at the end of the file:
```ts
describe('admin endpoints', () => {
  it('adminGetProducts sends Bearer and hits /api/admin/products', async () => {
    mockFetch(200, [{ id: 1, name: 'P', slug: 'p', category: 'C', version: '1.0.0', contextPath: '/p', description: '', tags: [], icon: '', upstreamUrl: '', published: true }])
    const out = await adminGetProducts('tok')
    expect(out).toHaveLength(1)
    const [url, opts] = (globalThis.fetch as any).mock.calls[0]
    expect(url).toBe('/api/admin/products')
    expect(opts.headers.Authorization).toBe('Bearer tok')
  })

  it('adminCreateProduct POSTs the product with auth', async () => {
    mockFetch(201, { id: 2, name: 'New', slug: 'new', category: 'C', version: '', contextPath: '/new', description: '', tags: [], icon: '', upstreamUrl: 'echo:8080', published: true })
    const p = await adminCreateProduct('tok', { name: 'New', slug: 'new', category: 'C', version: '', contextPath: '/new', description: '', tags: [], icon: '', upstreamUrl: 'echo:8080', published: true })
    expect(p.id).toBe(2)
    const [url, opts] = (globalThis.fetch as any).mock.calls[0]
    expect(url).toBe('/api/admin/products')
    expect(opts.method).toBe('POST')
    expect(opts.headers.Authorization).toBe('Bearer tok')
  })

  it('adminDeleteProduct throws the server error on 409', async () => {
    mockFetch(409, { error: 'product has active subscriptions' })
    await expect(adminDeleteProduct('tok', 5)).rejects.toThrow('product has active subscriptions')
  })

  it('adminGetSubscriptions passes the status filter', async () => {
    mockFetch(200, [])
    await adminGetSubscriptions('tok', 'pending')
    const url = (globalThis.fetch as any).mock.calls[0][0] as string
    expect(url).toBe('/api/admin/subscriptions?status=pending')
  })

  it('adminApproveSubscription POSTs to the approve URL and resolves on 204', async () => {
    mockFetch(204, {})
    await adminApproveSubscription('tok', 7)
    const [url, opts] = (globalThis.fetch as any).mock.calls[0]
    expect(url).toBe('/api/admin/subscriptions/7/approve')
    expect(opts.method).toBe('POST')
    expect(opts.headers.Authorization).toBe('Bearer tok')
  })
})
```

- [ ] **Step 2: Run to verify RED**

Run (from `web/`): `npx vitest run src/api/client.test.ts`
Expected: failure — the new functions are not exported.

- [ ] **Step 3: Add types**

In `web/src/api/types.ts`:

Add `status` to `SubscriptionView`:
```ts
export interface SubscriptionView {
  productId: number
  productName: string
  version: string
  contextPath: string
  planId: number
  planName: string
  status: string
}
```

Add two new interfaces at the end:
```ts
export interface AdminProduct {
  id?: number
  name: string
  slug: string
  category: string
  version: string
  contextPath: string
  description: string
  tags: string[]
  icon: string
  upstreamUrl: string
  published: boolean
}

export interface AdminSubscription {
  id: number
  applicationName: string
  ownerEmail: string
  productName: string
  version: string
  planName: string
  status: string
  createdAt: string
}
```

- [ ] **Step 4: Add client functions**

In `web/src/api/client.ts`, add (after the existing authed functions). First add a small helper for authed no-body-response requests, then the admin functions:

```ts
async function sendAuthed(method: string, url: string, token: string, body?: unknown): Promise<void> {
  const res = await fetch(url, {
    method,
    headers: authHeaders(token),
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!res.ok) {
    const b = await res.json().catch(() => ({}))
    throw new Error((b as { error?: string }).error || `request failed (${res.status})`)
  }
}

// --- Admin: products ---
export async function adminGetProducts(token: string): Promise<AdminProduct[]> {
  return parse<AdminProduct[]>(await fetch('/api/admin/products', { headers: authHeaders(token) }))
}
export async function adminCreateProduct(token: string, p: AdminProduct): Promise<AdminProduct> {
  return parse<AdminProduct>(await fetch('/api/admin/products', { method: 'POST', headers: authHeaders(token), body: JSON.stringify(p) }))
}
export async function adminUpdateProduct(token: string, id: number, p: AdminProduct): Promise<AdminProduct> {
  return parse<AdminProduct>(await fetch(`/api/admin/products/${id}`, { method: 'PUT', headers: authHeaders(token), body: JSON.stringify(p) }))
}
export async function adminDeleteProduct(token: string, id: number): Promise<void> {
  return sendAuthed('DELETE', `/api/admin/products/${id}`, token)
}

// --- Admin: plans ---
export async function adminGetPlans(token: string): Promise<Plan[]> {
  return parse<Plan[]>(await fetch('/api/admin/plans', { headers: authHeaders(token) }))
}
export async function adminCreatePlan(token: string, p: Plan): Promise<Plan> {
  return parse<Plan>(await fetch('/api/admin/plans', { method: 'POST', headers: authHeaders(token), body: JSON.stringify(p) }))
}
export async function adminUpdatePlan(token: string, id: number, p: Plan): Promise<Plan> {
  return parse<Plan>(await fetch(`/api/admin/plans/${id}`, { method: 'PUT', headers: authHeaders(token), body: JSON.stringify(p) }))
}
export async function adminDeletePlan(token: string, id: number): Promise<void> {
  return sendAuthed('DELETE', `/api/admin/plans/${id}`, token)
}

// --- Admin: subscriptions (approval) ---
export async function adminGetSubscriptions(token: string, status?: string): Promise<AdminSubscription[]> {
  const qs = status ? `?status=${encodeURIComponent(status)}` : ''
  return parse<AdminSubscription[]>(await fetch(`/api/admin/subscriptions${qs}`, { headers: authHeaders(token) }))
}
export async function adminApproveSubscription(token: string, id: number): Promise<void> {
  return sendAuthed('POST', `/api/admin/subscriptions/${id}/approve`, token)
}
export async function adminRejectSubscription(token: string, id: number): Promise<void> {
  return sendAuthed('POST', `/api/admin/subscriptions/${id}/reject`, token)
}
```

Also update the top import of `client.ts` to include the new types:
```ts
import type { Product, AuthResponse, ProductQuery, Plan, Application, Credential, AppDetail, AdminProduct, AdminSubscription } from './types'
```

- [ ] **Step 5: Run tests + typecheck**

Run: `npx vitest run src/api/client.test.ts` (PASS).
Run: `npx tsc --noEmit` (clean).

- [ ] **Step 6: Commit**

```bash
git add web/src/api/
git commit -m "feat(web): admin API client (products, plans, subscriptions) + status field

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: AdminGuard + routes + TopBar admin link

**Files:**
- Create: `web/src/admin/AdminGuard.tsx`
- Create: `web/src/admin/AdminGuard.test.tsx`
- Modify: `web/src/components/TopBar.tsx`
- Modify: `web/src/App.tsx` (routes added in Task 3–5; here add the guard + a placeholder is NOT needed — we add routes when pages exist. This task only ships the guard + TopBar link.)

- [ ] **Step 1: Write the failing test**

Create `web/src/admin/AdminGuard.test.tsx`:
```tsx
import { describe, it, expect, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { AdminGuard } from './AdminGuard'
import { ThemeProvider } from '../theme/ThemeProvider'
import { AuthProvider } from '../auth/AuthProvider'

beforeEach(() => localStorage.clear())

function renderAt(role: string | null) {
  if (role) {
    localStorage.setItem('token', 'tok')
    localStorage.setItem('user', JSON.stringify({ id: 1, email: 'a@b.c', name: '', role }))
  }
  return render(
    <MemoryRouter initialEntries={['/admin']}>
      <ThemeProvider><AuthProvider>
        <Routes>
          <Route path="/" element={<div>CATALOG</div>} />
          <Route path="/admin" element={<AdminGuard><div>ADMIN AREA</div></AdminGuard>} />
        </Routes>
      </AuthProvider></ThemeProvider>
    </MemoryRouter>
  )
}

describe('AdminGuard', () => {
  it('renders children for an admin', () => {
    renderAt('admin')
    expect(screen.getByText('ADMIN AREA')).toBeInTheDocument()
  })
  it('redirects a developer to the catalog', () => {
    renderAt('developer')
    expect(screen.getByText('CATALOG')).toBeInTheDocument()
    expect(screen.queryByText('ADMIN AREA')).not.toBeInTheDocument()
  })
  it('redirects an anonymous visitor to the catalog', () => {
    renderAt(null)
    expect(screen.getByText('CATALOG')).toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run to verify RED**

Run: `npx vitest run src/admin/AdminGuard.test.tsx`
Expected: failure — `AdminGuard` does not exist.

- [ ] **Step 3: Write the guard**

Create `web/src/admin/AdminGuard.tsx`:
```tsx
import type { ReactNode } from 'react'
import { Navigate } from 'react-router-dom'
import { useAuth } from '../auth/AuthProvider'

// AdminGuard renders its children only for an authenticated admin; everyone else
// is redirected to the catalog.
export function AdminGuard({ children }: { children: ReactNode }) {
  const { user } = useAuth()
  if (!user || user.role !== 'admin') return <Navigate to="/" replace />
  return <>{children}</>
}
```

- [ ] **Step 4: Add the TopBar admin link**

In `web/src/components/TopBar.tsx`, change the `nav-tabs` line to also show an Admin link for admins:
```tsx
      <nav className="nav-tabs"><Link className="active" to="/">APIs</Link>{user && <Link to="/applications">Applications</Link>}{user?.role === 'admin' && <Link to="/admin/products">Admin</Link>}</nav>
```

- [ ] **Step 5: Run tests + typecheck**

Run: `npx vitest run src/admin/AdminGuard.test.tsx` (PASS).
Run: `npx tsc --noEmit` (clean). `npx vitest run` (whole suite still green — the TopBar change is additive).

- [ ] **Step 6: Commit**

```bash
git add web/src/admin/ web/src/components/TopBar.tsx
git commit -m "feat(web): AdminGuard + admin nav link for admin role

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: AdminProductsPage

**Files:**
- Create: `web/src/components/AdminNav.tsx`
- Create: `web/src/pages/AdminProductsPage.tsx`
- Create: `web/src/pages/AdminProductsPage.test.tsx`

- [ ] **Step 1: Write the failing test**

Create `web/src/pages/AdminProductsPage.test.tsx`:
```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { AdminProductsPage } from './AdminProductsPage'
import { ThemeProvider } from '../theme/ThemeProvider'
import { AuthProvider } from '../auth/AuthProvider'
import * as api from '../api/client'
import type { AdminProduct } from '../api/types'

const sample: AdminProduct[] = [
  { id: 1, name: 'PizzaShackAPI', slug: 'pizzashackapi', category: 'Engineering', version: '1.0.0', contextPath: '/pizzashack', description: 'demo', tags: ['pizza'], icon: '', upstreamUrl: 'echo:8080', published: true },
]

beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('token', 'tok')
  localStorage.setItem('user', JSON.stringify({ id: 1, email: 'admin@portal.local', name: '', role: 'admin' }))
  vi.restoreAllMocks()
})

function renderPage() {
  return render(<MemoryRouter><ThemeProvider><AuthProvider><AdminProductsPage /></AuthProvider></ThemeProvider></MemoryRouter>)
}

describe('AdminProductsPage', () => {
  it('lists all products including unpublished state', async () => {
    vi.spyOn(api, 'adminGetProducts').mockResolvedValue(sample)
    renderPage()
    await waitFor(() => expect(screen.getByText('PizzaShackAPI')).toBeInTheDocument())
    expect(screen.getByText('echo:8080')).toBeInTheDocument()
  })

  it('creates a product from the form', async () => {
    vi.spyOn(api, 'adminGetProducts').mockResolvedValue([])
    const create = vi.spyOn(api, 'adminCreateProduct').mockResolvedValue({ ...sample[0], id: 9 })
    renderPage()
    await waitFor(() => expect(screen.getByLabelText('Nom')).toBeInTheDocument())
    await userEvent.type(screen.getByLabelText('Nom'), 'NewAPI')
    await userEvent.type(screen.getByLabelText('Slug'), 'newapi')
    await userEvent.type(screen.getByLabelText('Catégorie'), 'Engineering')
    await userEvent.type(screen.getByLabelText('Context path'), '/new')
    await userEvent.click(screen.getByRole('button', { name: 'Créer le produit' }))
    await waitFor(() => expect(create).toHaveBeenCalled())
    expect(create.mock.calls[0][1]).toMatchObject({ name: 'NewAPI', slug: 'newapi', contextPath: '/new' })
  })

  it('shows the 409 message when a delete is blocked', async () => {
    vi.spyOn(api, 'adminGetProducts').mockResolvedValue(sample)
    vi.spyOn(api, 'adminDeleteProduct').mockRejectedValue(new Error('product has active subscriptions'))
    renderPage()
    await waitFor(() => expect(screen.getByText('PizzaShackAPI')).toBeInTheDocument())
    await userEvent.click(screen.getByRole('button', { name: 'Supprimer PizzaShackAPI' }))
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent(/active subscriptions/i))
  })
})
```

- [ ] **Step 2: Run to verify RED**

Run: `npx vitest run src/pages/AdminProductsPage.test.tsx`
Expected: failure — page doesn't exist.

- [ ] **Step 3: Write the shared AdminNav**

Create `web/src/components/AdminNav.tsx`:
```tsx
import { Link } from 'react-router-dom'
import { TopBar } from './TopBar'

// AdminNav renders the top bar plus the admin section sub-navigation. `active`
// is one of 'products' | 'plans' | 'approvals'.
export function AdminNav({ active }: { active: 'products' | 'plans' | 'approvals' }) {
  return (
    <>
      <TopBar search="" onSearch={() => {}} />
      <nav className="nav-tabs" style={{ padding: '12px 28px', gap: 14 }}>
        <Link className={active === 'products' ? 'active' : ''} to="/admin/products">Produits</Link>
        <Link className={active === 'plans' ? 'active' : ''} to="/admin/plans">Plans</Link>
        <Link className={active === 'approvals' ? 'active' : ''} to="/admin/approvals">Abonnements</Link>
      </nav>
    </>
  )
}
```

- [ ] **Step 4: Write the page**

Create `web/src/pages/AdminProductsPage.tsx`:
```tsx
import { useEffect, useState } from 'react'
import type { AdminProduct } from '../api/types'
import { adminGetProducts, adminCreateProduct, adminUpdateProduct, adminDeleteProduct } from '../api/client'
import { useAuth } from '../auth/AuthProvider'
import { AdminNav } from '../components/AdminNav'
import '../styles/catalog.css'

const empty: AdminProduct = { name: '', slug: '', category: '', version: '', contextPath: '', description: '', tags: [], icon: '', upstreamUrl: '', published: true }

const field: React.CSSProperties = { height: 38, padding: '0 12px', border: '1px solid var(--border-2)', borderRadius: 10, background: 'var(--bg)', color: 'var(--fg)' }

export function AdminProductsPage() {
  const { token } = useAuth()
  const [products, setProducts] = useState<AdminProduct[]>([])
  const [form, setForm] = useState<AdminProduct>(empty)
  const [editingId, setEditingId] = useState<number | null>(null)
  const [err, setErr] = useState('')

  function reload() {
    if (!token) return
    adminGetProducts(token).then(setProducts).catch(() => setErr('Impossible de charger les produits.'))
  }
  useEffect(reload, [token])

  function set<K extends keyof AdminProduct>(k: K, v: AdminProduct[K]) { setForm(f => ({ ...f, [k]: v })) }

  async function onSubmit() {
    if (!token) return
    setErr('')
    const payload: AdminProduct = { ...form, tags: form.tags }
    try {
      if (editingId == null) await adminCreateProduct(token, payload)
      else await adminUpdateProduct(token, editingId, payload)
      setForm(empty); setEditingId(null); reload()
    } catch (e) { setErr(e instanceof Error ? e.message : 'Échec de l’enregistrement.') }
  }

  function onEdit(p: AdminProduct) { setEditingId(p.id ?? null); setForm({ ...p }) }

  async function onDelete(p: AdminProduct) {
    if (!token || p.id == null) return
    setErr('')
    try { await adminDeleteProduct(token, p.id); reload() }
    catch (e) { setErr(e instanceof Error ? e.message : 'Échec de la suppression.') }
  }

  return (
    <>
      <AdminNav active="products" />
      <div className="content">
        <div className="chead"><div className="titlewrap"><h1>Produits</h1></div></div>
        {err && <p className="autherr" role="alert">{err}</p>}

        <div className="card" style={{ padding: 18, marginBottom: 22, display: 'grid', gap: 10, gridTemplateColumns: '1fr 1fr' }}>
          <label style={{ display: 'grid', gap: 4 }}>Nom<input aria-label="Nom" style={field} value={form.name} onChange={e => set('name', e.target.value)} /></label>
          <label style={{ display: 'grid', gap: 4 }}>Slug<input aria-label="Slug" style={field} value={form.slug} onChange={e => set('slug', e.target.value)} /></label>
          <label style={{ display: 'grid', gap: 4 }}>Catégorie<input aria-label="Catégorie" style={field} value={form.category} onChange={e => set('category', e.target.value)} /></label>
          <label style={{ display: 'grid', gap: 4 }}>Context path<input aria-label="Context path" style={field} value={form.contextPath} onChange={e => set('contextPath', e.target.value)} /></label>
          <label style={{ display: 'grid', gap: 4 }}>Upstream (host:port)<input aria-label="Upstream" style={field} value={form.upstreamUrl} onChange={e => set('upstreamUrl', e.target.value)} /></label>
          <label style={{ display: 'grid', gap: 4 }}>Version<input aria-label="Version" style={field} value={form.version} onChange={e => set('version', e.target.value)} placeholder="1.0.0" /></label>
          <label style={{ gridColumn: '1 / -1', display: 'flex', gap: 8, alignItems: 'center' }}>
            <input type="checkbox" aria-label="Publié" checked={form.published} onChange={e => set('published', e.target.checked)} /> Publié
          </label>
          <div style={{ gridColumn: '1 / -1', display: 'flex', gap: 10 }}>
            <button className="subbtn" onClick={onSubmit}>{editingId == null ? 'Créer le produit' : 'Enregistrer'}</button>
            {editingId != null && <button className="subbtn ghost" onClick={() => { setForm(empty); setEditingId(null) }}>Annuler</button>}
          </div>
        </div>

        {products.length === 0 && <p className="rescount">Aucun produit.</p>}
        {products.map(p => (
          <div key={p.id} className="cfoot" style={{ justifyContent: 'space-between', padding: '10px 0', borderBottom: '1px solid var(--border)' }}>
            <span>
              <b>{p.name}</b> <span className="ctx">{p.contextPath}</span>
              {' · '}<span className="pill">{p.upstreamUrl || 'pas d’upstream'}</span>
              {' · '}<span className="pill">{p.published ? 'publié' : 'masqué'}</span>
            </span>
            <span style={{ display: 'flex', gap: 8 }}>
              <button className="subbtn ghost" onClick={() => onEdit(p)} aria-label={`Modifier ${p.name}`}>Modifier</button>
              <button className="subbtn ghost" onClick={() => onDelete(p)} aria-label={`Supprimer ${p.name}`}>Supprimer</button>
            </span>
          </div>
        ))}
      </div>
    </>
  )
}
```

- [ ] **Step 5: Run tests + typecheck**

Run: `npx vitest run src/pages/AdminProductsPage.test.tsx` (PASS).
Run: `npx tsc --noEmit` (clean).

- [ ] **Step 6: Commit**

```bash
git add web/src/components/AdminNav.tsx web/src/pages/AdminProductsPage.tsx web/src/pages/AdminProductsPage.test.tsx
git commit -m "feat(web): admin products page (CRUD + publish + upstream, 409 handling)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: AdminPlansPage

**Files:**
- Create: `web/src/pages/AdminPlansPage.tsx`
- Create: `web/src/pages/AdminPlansPage.test.tsx`

- [ ] **Step 1: Write the failing test**

Create `web/src/pages/AdminPlansPage.test.tsx`:
```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { AdminPlansPage } from './AdminPlansPage'
import { ThemeProvider } from '../theme/ThemeProvider'
import { AuthProvider } from '../auth/AuthProvider'
import * as api from '../api/client'
import type { Plan } from '../api/types'

const plans: Plan[] = [{ id: 1, name: 'Silver', rateLimit: 300, windowSeconds: 60 }]

beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('token', 'tok')
  localStorage.setItem('user', JSON.stringify({ id: 1, email: 'admin@portal.local', name: '', role: 'admin' }))
  vi.restoreAllMocks()
})

function renderPage() {
  return render(<MemoryRouter><ThemeProvider><AuthProvider><AdminPlansPage /></AuthProvider></ThemeProvider></MemoryRouter>)
}

describe('AdminPlansPage', () => {
  it('lists plans', async () => {
    vi.spyOn(api, 'adminGetPlans').mockResolvedValue(plans)
    renderPage()
    await waitFor(() => expect(screen.getByText('Silver')).toBeInTheDocument())
  })

  it('creates a plan from the form', async () => {
    vi.spyOn(api, 'adminGetPlans').mockResolvedValue([])
    const create = vi.spyOn(api, 'adminCreatePlan').mockResolvedValue({ id: 5, name: 'Gold', rateLimit: 1000, windowSeconds: 60 })
    renderPage()
    await waitFor(() => expect(screen.getByLabelText('Nom du plan')).toBeInTheDocument())
    await userEvent.type(screen.getByLabelText('Nom du plan'), 'Gold')
    await userEvent.clear(screen.getByLabelText('Limite (requêtes)')); await userEvent.type(screen.getByLabelText('Limite (requêtes)'), '1000')
    await userEvent.clear(screen.getByLabelText('Fenêtre (secondes)')); await userEvent.type(screen.getByLabelText('Fenêtre (secondes)'), '60')
    await userEvent.click(screen.getByRole('button', { name: 'Créer le plan' }))
    await waitFor(() => expect(create).toHaveBeenCalled())
    expect(create.mock.calls[0][1]).toMatchObject({ name: 'Gold', rateLimit: 1000, windowSeconds: 60 })
  })

  it('shows the 409 message when a delete is blocked', async () => {
    vi.spyOn(api, 'adminGetPlans').mockResolvedValue(plans)
    vi.spyOn(api, 'adminDeletePlan').mockRejectedValue(new Error('plan is referenced by subscriptions'))
    renderPage()
    await waitFor(() => expect(screen.getByText('Silver')).toBeInTheDocument())
    await userEvent.click(screen.getByRole('button', { name: 'Supprimer Silver' }))
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent(/referenced by subscriptions/i))
  })
})
```

- [ ] **Step 2: Run to verify RED**

Run: `npx vitest run src/pages/AdminPlansPage.test.tsx`
Expected: failure — page doesn't exist.

- [ ] **Step 3: Write the page**

Create `web/src/pages/AdminPlansPage.tsx`:
```tsx
import { useEffect, useState } from 'react'
import type { Plan } from '../api/types'
import { adminGetPlans, adminCreatePlan, adminUpdatePlan, adminDeletePlan } from '../api/client'
import { useAuth } from '../auth/AuthProvider'
import { AdminNav } from '../components/AdminNav'
import '../styles/catalog.css'

const emptyPlan: Plan = { id: 0, name: '', rateLimit: 100, windowSeconds: 60 }
const field: React.CSSProperties = { height: 38, padding: '0 12px', border: '1px solid var(--border-2)', borderRadius: 10, background: 'var(--bg)', color: 'var(--fg)' }

export function AdminPlansPage() {
  const { token } = useAuth()
  const [plans, setPlans] = useState<Plan[]>([])
  const [form, setForm] = useState<Plan>(emptyPlan)
  const [editingId, setEditingId] = useState<number | null>(null)
  const [err, setErr] = useState('')

  function reload() {
    if (!token) return
    adminGetPlans(token).then(setPlans).catch(() => setErr('Impossible de charger les plans.'))
  }
  useEffect(reload, [token])

  async function onSubmit() {
    if (!token) return
    setErr('')
    try {
      if (editingId == null) await adminCreatePlan(token, form)
      else await adminUpdatePlan(token, editingId, form)
      setForm(emptyPlan); setEditingId(null); reload()
    } catch (e) { setErr(e instanceof Error ? e.message : 'Échec de l’enregistrement.') }
  }

  async function onDelete(p: Plan) {
    if (!token) return
    setErr('')
    try { await adminDeletePlan(token, p.id); reload() }
    catch (e) { setErr(e instanceof Error ? e.message : 'Échec de la suppression.') }
  }

  return (
    <>
      <AdminNav active="plans" />
      <div className="content">
        <div className="chead"><div className="titlewrap"><h1>Plans</h1></div></div>
        {err && <p className="autherr" role="alert">{err}</p>}

        <div className="card" style={{ padding: 18, marginBottom: 22, display: 'grid', gap: 10, gridTemplateColumns: '1fr 1fr 1fr' }}>
          <label style={{ display: 'grid', gap: 4 }}>Nom du plan<input aria-label="Nom du plan" style={field} value={form.name} onChange={e => setForm(f => ({ ...f, name: e.target.value }))} /></label>
          <label style={{ display: 'grid', gap: 4 }}>Limite (requêtes)<input aria-label="Limite (requêtes)" type="number" style={field} value={form.rateLimit} onChange={e => setForm(f => ({ ...f, rateLimit: Number(e.target.value) }))} /></label>
          <label style={{ display: 'grid', gap: 4 }}>Fenêtre (secondes)<input aria-label="Fenêtre (secondes)" type="number" style={field} value={form.windowSeconds} onChange={e => setForm(f => ({ ...f, windowSeconds: Number(e.target.value) }))} /></label>
          <div style={{ gridColumn: '1 / -1', display: 'flex', gap: 10 }}>
            <button className="subbtn" onClick={onSubmit}>{editingId == null ? 'Créer le plan' : 'Enregistrer'}</button>
            {editingId != null && <button className="subbtn ghost" onClick={() => { setForm(emptyPlan); setEditingId(null) }}>Annuler</button>}
          </div>
        </div>

        {plans.length === 0 && <p className="rescount">Aucun plan.</p>}
        {plans.map(p => (
          <div key={p.id} className="cfoot" style={{ justifyContent: 'space-between', padding: '10px 0', borderBottom: '1px solid var(--border)' }}>
            <span><b>{p.name}</b> · <span className="pill">{p.rateLimit} req / {p.windowSeconds}s</span></span>
            <span style={{ display: 'flex', gap: 8 }}>
              <button className="subbtn ghost" onClick={() => { setEditingId(p.id); setForm({ ...p }) }} aria-label={`Modifier ${p.name}`}>Modifier</button>
              <button className="subbtn ghost" onClick={() => onDelete(p)} aria-label={`Supprimer ${p.name}`}>Supprimer</button>
            </span>
          </div>
        ))}
      </div>
    </>
  )
}
```

- [ ] **Step 4: Run tests + typecheck**

Run: `npx vitest run src/pages/AdminPlansPage.test.tsx` (PASS).
Run: `npx tsc --noEmit` (clean).

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/AdminPlansPage.tsx web/src/pages/AdminPlansPage.test.tsx
git commit -m "feat(web): admin plans page (CRUD, 409 handling)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: AdminApprovalsPage

**Files:**
- Create: `web/src/pages/AdminApprovalsPage.tsx`
- Create: `web/src/pages/AdminApprovalsPage.test.tsx`

- [ ] **Step 1: Write the failing test**

Create `web/src/pages/AdminApprovalsPage.test.tsx`:
```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { AdminApprovalsPage } from './AdminApprovalsPage'
import { ThemeProvider } from '../theme/ThemeProvider'
import { AuthProvider } from '../auth/AuthProvider'
import * as api from '../api/client'
import type { AdminSubscription } from '../api/types'

const pending: AdminSubscription[] = [
  { id: 7, applicationName: 'My App', ownerEmail: 'dev@x.com', productName: 'PizzaShackAPI', version: '1.0.0', planName: 'Silver', status: 'pending', createdAt: '2026-05-30T10:00:00Z' },
]

beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('token', 'tok')
  localStorage.setItem('user', JSON.stringify({ id: 1, email: 'admin@portal.local', name: '', role: 'admin' }))
  vi.restoreAllMocks()
})

function renderPage() {
  return render(<MemoryRouter><ThemeProvider><AuthProvider><AdminApprovalsPage /></AuthProvider></ThemeProvider></MemoryRouter>)
}

describe('AdminApprovalsPage', () => {
  it('lists pending subscriptions with app, product, plan and requester', async () => {
    vi.spyOn(api, 'adminGetSubscriptions').mockResolvedValue(pending)
    renderPage()
    await waitFor(() => expect(screen.getByText('PizzaShackAPI')).toBeInTheDocument())
    expect(screen.getByText('My App')).toBeInTheDocument()
    expect(screen.getByText('dev@x.com')).toBeInTheDocument()
    expect(api.adminGetSubscriptions).toHaveBeenCalledWith('tok', 'pending')
  })

  it('approves a subscription and refreshes', async () => {
    const get = vi.spyOn(api, 'adminGetSubscriptions').mockResolvedValue(pending)
    const approve = vi.spyOn(api, 'adminApproveSubscription').mockResolvedValue(undefined)
    renderPage()
    await waitFor(() => expect(screen.getByText('PizzaShackAPI')).toBeInTheDocument())
    get.mockResolvedValue([])
    await userEvent.click(screen.getByRole('button', { name: 'Approuver' }))
    await waitFor(() => expect(approve).toHaveBeenCalledWith('tok', 7))
    await waitFor(() => expect(screen.getByText(/Aucun abonnement en attente/i)).toBeInTheDocument())
  })

  it('rejects a subscription', async () => {
    vi.spyOn(api, 'adminGetSubscriptions').mockResolvedValue(pending)
    const reject = vi.spyOn(api, 'adminRejectSubscription').mockResolvedValue(undefined)
    renderPage()
    await waitFor(() => expect(screen.getByText('PizzaShackAPI')).toBeInTheDocument())
    await userEvent.click(screen.getByRole('button', { name: 'Rejeter' }))
    await waitFor(() => expect(reject).toHaveBeenCalledWith('tok', 7))
  })
})
```

- [ ] **Step 2: Run to verify RED**

Run: `npx vitest run src/pages/AdminApprovalsPage.test.tsx`
Expected: failure — page doesn't exist.

- [ ] **Step 3: Write the page**

Create `web/src/pages/AdminApprovalsPage.tsx`:
```tsx
import { useEffect, useState } from 'react'
import type { AdminSubscription } from '../api/types'
import { adminGetSubscriptions, adminApproveSubscription, adminRejectSubscription } from '../api/client'
import { useAuth } from '../auth/AuthProvider'
import { AdminNav } from '../components/AdminNav'
import '../styles/catalog.css'

export function AdminApprovalsPage() {
  const { token } = useAuth()
  const [subs, setSubs] = useState<AdminSubscription[]>([])
  const [err, setErr] = useState('')

  function reload() {
    if (!token) return
    adminGetSubscriptions(token, 'pending').then(setSubs).catch(() => setErr('Impossible de charger les abonnements.'))
  }
  useEffect(reload, [token])

  async function act(id: number, fn: (t: string, i: number) => Promise<void>) {
    if (!token) return
    setErr('')
    try { await fn(token, id); reload() }
    catch (e) { setErr(e instanceof Error ? e.message : 'Échec de l’opération.') }
  }

  return (
    <>
      <AdminNav active="approvals" />
      <div className="content">
        <div className="chead"><div className="titlewrap"><h1>Abonnements en attente</h1></div></div>
        {err && <p className="autherr" role="alert">{err}</p>}
        {subs.length === 0 && <p className="rescount">Aucun abonnement en attente.</p>}
        {subs.map(s => (
          <div key={s.id} className="cfoot" style={{ justifyContent: 'space-between', padding: '12px 0', borderBottom: '1px solid var(--border)' }}>
            <span>
              <b>{s.productName}</b> <span className="ctx">{s.version}</span> · {s.planName}
              {' — '}<span>{s.applicationName}</span> <span className="pill">{s.ownerEmail}</span>
            </span>
            <span style={{ display: 'flex', gap: 8 }}>
              <button className="subbtn" onClick={() => act(s.id, adminApproveSubscription)}>Approuver</button>
              <button className="subbtn ghost" onClick={() => act(s.id, adminRejectSubscription)}>Rejeter</button>
            </span>
          </div>
        ))}
      </div>
    </>
  )
}
```

- [ ] **Step 4: Run tests + typecheck**

Run: `npx vitest run src/pages/AdminApprovalsPage.test.tsx` (PASS).
Run: `npx tsc --noEmit` (clean).

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/AdminApprovalsPage.tsx web/src/pages/AdminApprovalsPage.test.tsx
git commit -m "feat(web): admin approvals page (pending queue, approve/reject)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 6: Wire admin routes + show subscription status on Applications

**Files:**
- Modify: `web/src/App.tsx`
- Modify: `web/src/pages/ApplicationsPage.tsx`
- Modify: `web/src/pages/ApplicationsPage.test.tsx`

- [ ] **Step 1: Write the failing test (status badge on Applications)**

In `web/src/pages/ApplicationsPage.test.tsx`, the existing test's `getApplicationDetail` mock returns a subscription without `status`. Update that mock subscription to include `status: 'pending'` and assert the status text shows:

Replace the `getApplicationDetail` mock and add an assertion:
```tsx
    vi.spyOn(api, 'getApplicationDetail').mockResolvedValue({
      apiKey: 'KEY-9', consumerUsername: 'app_9',
      subscriptions: [{ productId: 3, productName: 'PizzaShackAPI', version: '1.0.0', contextPath: '/pizzashack', planId: 2, planName: 'Silver', status: 'pending' }],
    })
```
And after the existing assertions in that test, add:
```tsx
    expect(screen.getByText(/En attente/i)).toBeInTheDocument()
```

- [ ] **Step 2: Run to verify RED**

Run: `npx vitest run src/pages/ApplicationsPage.test.tsx`
Expected: failure — no "En attente" text rendered yet (and a TS error if `status` is required on the mock — Task 1 made it required, so also the other places constructing `SubscriptionView` in tests must include it; the only one is here).

- [ ] **Step 3: Show status in `ApplicationsPage.tsx`**

In `web/src/pages/ApplicationsPage.tsx`, change the subscription row to render a localized status label. Replace the subscriptions `.map(...)` block with:
```tsx
            {detail.subscriptions.map(s => (
              <div key={s.productId} className="cfoot" style={{ justifyContent: 'space-between' }}>
                <span><span>{s.productName}</span> <span className="ctx">{s.contextPath}</span> · {s.planName} · <span className="pill">{statusLabel(s.status)}</span></span>
                <button className="subbtn ghost" onClick={() => onUnsub(s.productId)}>Se désabonner</button>
              </div>
            ))}
```
And add this helper above the `return` (or at module scope):
```tsx
function statusLabel(status: string): string {
  switch (status) {
    case 'active': return 'Actif'
    case 'pending': return 'En attente'
    case 'rejected': return 'Rejeté'
    default: return status
  }
}
```

- [ ] **Step 4: Add admin routes in `App.tsx`**

Replace `web/src/App.tsx` with:
```tsx
import { Routes, Route } from 'react-router-dom'
import { CatalogPage } from './pages/CatalogPage'
import { LoginPage } from './pages/LoginPage'
import { RegisterPage } from './pages/RegisterPage'
import { ApplicationsPage } from './pages/ApplicationsPage'
import { AdminGuard } from './admin/AdminGuard'
import { AdminProductsPage } from './pages/AdminProductsPage'
import { AdminPlansPage } from './pages/AdminPlansPage'
import { AdminApprovalsPage } from './pages/AdminApprovalsPage'

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<CatalogPage />} />
      <Route path="/login" element={<LoginPage />} />
      <Route path="/register" element={<RegisterPage />} />
      <Route path="/applications" element={<ApplicationsPage />} />
      <Route path="/admin/products" element={<AdminGuard><AdminProductsPage /></AdminGuard>} />
      <Route path="/admin/plans" element={<AdminGuard><AdminPlansPage /></AdminGuard>} />
      <Route path="/admin/approvals" element={<AdminGuard><AdminApprovalsPage /></AdminGuard>} />
    </Routes>
  )
}
```

- [ ] **Step 5: Run the whole suite + typecheck + build**

Run: `npx vitest run` (ALL tests green).
Run: `npx tsc --noEmit` (clean).
Run: `npm run build` (Vite production build succeeds).

- [ ] **Step 6: Commit**

```bash
git add web/src/App.tsx web/src/pages/ApplicationsPage.tsx web/src/pages/ApplicationsPage.test.tsx
git commit -m "feat(web): mount admin routes under AdminGuard; show subscription status

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 7: Live smoke (optional, requires running stack + dev server)

Manual end-to-end browser check; skip if the stack/dev server is not running (vitest covers the logic). With the backend up (`make up && make run`) and `npm run dev` in `web/`:

- [ ] Log in as the admin (`admin@portal.local`). Confirm an "Admin" link appears in the top nav; a developer login shows no such link, and visiting `/admin/products` as a developer redirects to the catalog.
- [ ] Products: create a product with `upstreamUrl=echo:8080`, toggle published, edit it, attempt to delete one with active subscriptions (expect the inline 409 message).
- [ ] Plans: create/edit/delete a plan; deleting a plan in use shows the 409 message.
- [ ] Approvals: as a developer subscribe to a product (it shows "En attente" on the Applications page); as admin approve it from the queue; confirm the developer's app detail flips to "Actif" and the gateway call with the key now succeeds.

---

## Self-review notes (already applied)

- **Spec coverage:** role-gated `/admin` (AdminGuard, Task 2); product editor incl. upstream + publish + 409 (Task 3); plan editor + 409 (Task 4); approvals queue approve/reject (Task 5); TopBar admin link for admins (Task 2); subscription status on Applications (Task 6). Reuses Atlas tokens/light-dark via existing CSS.
- **Type consistency:** `AdminProduct` (id optional for create) matches the backend admin Product JSON; `AdminSubscription` matches `AdminSubscriptionView`; `Plan` reused as-is; `SubscriptionView.status` added and the one test constructing it updated. New client fns: `adminGetProducts/adminCreateProduct/adminUpdateProduct/adminDeleteProduct`, `adminGetPlans/adminCreatePlan/adminUpdatePlan/adminDeletePlan`, `adminGetSubscriptions/adminApproveSubscription/adminRejectSubscription` — names used identically across client, tests, and pages.
- **Error handling:** every page surfaces thrown messages via an `autherr` role="alert" paragraph (so backend 400/409 text — "product has active subscriptions", "plan is referenced by subscriptions", validation messages — is shown verbatim). 204 responses handled by `sendAuthed` (no JSON parse on success).
- **No placeholders:** every code step is complete and compilable; exact `npx vitest run`/`tsc`/`build` commands with expected results.
