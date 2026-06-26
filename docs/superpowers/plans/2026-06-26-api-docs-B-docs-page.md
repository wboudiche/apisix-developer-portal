# Interactive API Docs — Plan B: Docs Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a public product detail page at `/apis/:slug` that shows the product header (name, category, version, rating, tags, description), a "S'abonner" action, and renders the stored OpenAPI spec as interactive docs with Scalar. Try-it stays read-only here (Plan C wires the live calls).

**Architecture:** A new `ProductDetailPage` fetches the product (`GET /api/products/{slug}`) and its spec (`GET /api/products/{slug}/spec`, added in Plan A) and renders a `ScalarDocs` wrapper around `@scalar/api-reference-react`. The wrapper is lazy-loaded so Scalar's bundle stays out of the catalog/admin chunks. Catalog cards link to the page. Unit tests mock the Scalar component (it does not render meaningfully in jsdom and its exact prop shape shouldn't couple our tests); real rendering is confirmed by the live check.

**Tech Stack:** React 19 + TS, Vite, vitest, react-router, `@scalar/api-reference-react` (new dep).

## Global Constraints

- pnpm project under `web/`. Run tests with `pnpm exec vitest run <path>`, typecheck `pnpm exec tsc --noEmit`, build `pnpm build`.
- Route is `/apis/:slug`. Docs are PUBLIC (no auth to read).
- Spec is fetched as raw text from `GET /api/products/{slug}/spec`; `404` → the product has no docs → show a placeholder, not an error.
- Scalar is lazy-loaded (`React.lazy` + `Suspense`) so it is not in the main bundle.
- French copy; reuse existing Atlas tokens and the catalog header patterns (`ApiCard` shows the icon/stars/category — mirror them).
- Reuse `SubscribeModal` (`web/src/components/SubscribeModal.tsx`, props `{ product: Product; onClose }`) for the subscribe action, login-gated like the catalog.

---

## Task B1: Client functions `getProduct` + `getProductSpec`

**Files:**
- Modify: `web/src/api/client.ts`
- Test: `web/src/api/client.product.test.ts` (new)

**Interfaces:**
- Consumes: existing `parse<T>`, `Product` type, `ApiError`.
- Produces:
  - `getProduct(slug: string): Promise<Product>` — `GET /api/products/{slug}`.
  - `getProductSpec(slug: string): Promise<string | null>` — `GET /api/products/{slug}/spec`; returns the raw spec text, or `null` on `404` (no docs).

- [ ] **Step 1: Write the failing test**

Create `web/src/api/client.product.test.ts`:
```ts
import { it, expect, vi, afterEach } from 'vitest'
import { getProduct, getProductSpec } from './client'

afterEach(() => vi.restoreAllMocks())

it('getProduct fetches a single product by slug', async () => {
  const product = { id: 1, name: 'Orders', slug: 'orders', category: 'Data', version: '1.0.0', contextPath: '/orders', description: '', tags: [], icon: '', rating: 4 }
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(
    new Response(JSON.stringify(product), { status: 200, headers: { 'Content-Type': 'application/json' } }),
  )
  const out = await getProduct('orders')
  expect(out.name).toBe('Orders')
})

it('getProductSpec returns the raw spec text', async () => {
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(
    new Response('{"openapi":"3.0.0"}', { status: 200, headers: { 'Content-Type': 'application/json' } }),
  )
  expect(await getProductSpec('orders')).toBe('{"openapi":"3.0.0"}')
})

it('getProductSpec returns null on 404 (no docs)', async () => {
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('{"error":"spec not found"}', { status: 404 }))
  expect(await getProductSpec('orders')).toBeNull()
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd web && pnpm exec vitest run src/api/client.product.test.ts`
Expected: FAIL — `getProduct`/`getProductSpec` not exported.

- [ ] **Step 3: Implement the client functions**

In `web/src/api/client.ts`, in the public catalog section (near `getProducts`):
```ts
export async function getProduct(slug: string): Promise<Product> {
  const url = `/api/products/${encodeURIComponent(slug)}`
  return parse<Product>(await fetch(url), url)
}

export async function getProductSpec(slug: string): Promise<string | null> {
  const res = await fetch(`/api/products/${encodeURIComponent(slug)}/spec`)
  if (res.status === 404) return null
  if (!res.ok) throw new ApiError(`spec fetch failed (${res.status})`, res.status)
  return res.text()
}
```
(Ensure `ApiError` and `Product` are already imported in client.ts — they are.)

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd web && pnpm exec vitest run src/api/client.product.test.ts`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/api/client.ts web/src/api/client.product.test.ts
git commit -m "feat(web): getProduct + getProductSpec client fns"
```

---

## Task B2: Scalar dependency + `ScalarDocs` wrapper

**Files:**
- Modify: `web/package.json`, `web/pnpm-lock.yaml` (via pnpm add)
- Create: `web/src/components/ScalarDocs.tsx`
- Test: `web/src/components/ScalarDocs.test.tsx`

**Interfaces:**
- Produces: `ScalarDocs({ spec }: { spec: string }): JSX.Element` — renders `@scalar/api-reference-react`'s `ApiReferenceReact` with the spec as inline content, themed to the portal.

- [ ] **Step 1: Add the dependency**

Run:
```bash
cd web && pnpm add @scalar/api-reference-react
```
Expected: added to `package.json` dependencies; lockfile updated; `pnpm build` still resolvable.

- [ ] **Step 2: Write the failing test (mock the Scalar module)**

Create `web/src/components/ScalarDocs.test.tsx`:
```tsx
import { it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ScalarDocs } from './ScalarDocs'

// Scalar renders nothing meaningful in jsdom and pulls a large bundle; mock it
// and assert ScalarDocs hands it our spec content.
vi.mock('@scalar/api-reference-react', () => ({
  ApiReferenceReact: ({ configuration }: { configuration: { content: string } }) => (
    <div data-testid="scalar" data-content={configuration.content} />
  ),
}))

it('passes the spec to ApiReferenceReact as content', () => {
  render(<ScalarDocs spec='{"openapi":"3.0.0"}' />)
  expect(screen.getByTestId('scalar')).toHaveAttribute('data-content', '{"openapi":"3.0.0"}')
})
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `cd web && pnpm exec vitest run src/components/ScalarDocs.test.tsx`
Expected: FAIL — cannot find `./ScalarDocs`.

- [ ] **Step 4: Implement the wrapper**

Create `web/src/components/ScalarDocs.tsx`:
```tsx
import '@scalar/api-reference-react/style.css'
import { ApiReferenceReact } from '@scalar/api-reference-react'

// Wraps Scalar's React renderer. `spec` is the raw OpenAPI text (JSON or YAML);
// Scalar accepts either as inline `content`. Themed to the portal's crimson via
// CSS variables on the wrapper (Scalar reads --scalar-* vars).
export function ScalarDocs({ spec }: { spec: string }) {
  return (
    <div className="scalar-wrap">
      <ApiReferenceReact
        configuration={{
          content: spec,
          hideClientButton: true,
          hideDownloadButton: false,
          theme: 'default',
        }}
      />
    </div>
  )
}
```
NOTE for the implementer: the installed `@scalar/api-reference-react` version determines the exact `configuration` shape. If `content` is not accepted in the installed version, use `{ spec: { content: spec } }` instead — confirm against the package's types (`node_modules/@scalar/api-reference-react`). Keep the mock in the test matching whatever prop you settle on. The stylesheet import path may also be `@scalar/api-reference-react/style.css` or `.../dist/style.css` — use the one that resolves.

- [ ] **Step 5: Run the test to verify it passes**

Run: `cd web && pnpm exec vitest run src/components/ScalarDocs.test.tsx && pnpm exec tsc --noEmit`
Expected: PASS + no type errors.

- [ ] **Step 6: Commit**

```bash
git add web/package.json web/pnpm-lock.yaml web/src/components/ScalarDocs.tsx web/src/components/ScalarDocs.test.tsx
git commit -m "feat(web): ScalarDocs wrapper around @scalar/api-reference-react"
```

---

## Task B3: Product detail page `/apis/:slug`

**Files:**
- Create: `web/src/pages/ProductDetailPage.tsx`
- Create: `web/src/styles/productdetail.css`
- Modify: `web/src/App.tsx` (route)
- Test: `web/src/pages/ProductDetailPage.test.tsx`

**Interfaces:**
- Consumes: `getProduct`, `getProductSpec` (B1); `ScalarDocs` (B2, lazy); `SubscribeModal`; `TopBar`; `useAuth`; `apiIcons` helpers (`ApiIcon`, `categoryTint`) used by `ApiCard`.
- Produces: route element `<ProductDetailPage />` at `/apis/:slug`.

- [ ] **Step 1: Write the failing tests**

Create `web/src/pages/ProductDetailPage.test.tsx`:
```tsx
import { it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { ProductDetailPage } from './ProductDetailPage'
import { AuthProvider } from '../auth/AuthProvider'
import { ThemeProvider } from '../theme/ThemeProvider'
import * as api from '../api/client'
import type { Product } from '../api/types'

// Mock Scalar so the page test doesn't pull the real renderer.
vi.mock('@scalar/api-reference-react', () => ({
  ApiReferenceReact: ({ configuration }: { configuration: { content: string } }) => (
    <div data-testid="scalar" data-content={configuration.content} />
  ),
}))

const product: Product = {
  id: 1, name: 'Orders API', slug: 'orders', category: 'Data', version: '2.1.0',
  contextPath: '/orders', description: 'Gère les commandes.', tags: ['data'], icon: '', rating: 4,
}

beforeEach(() => {
  localStorage.clear()
  vi.spyOn(api, 'getProduct').mockResolvedValue(product)
})
afterEach(() => vi.restoreAllMocks())

function renderAt(slug: string) {
  return render(
    <MemoryRouter initialEntries={[`/apis/${slug}`]}>
      <ThemeProvider><AuthProvider>
        <Routes><Route path="/apis/:slug" element={<ProductDetailPage />} /></Routes>
      </AuthProvider></ThemeProvider>
    </MemoryRouter>
  )
}

it('renders the product header and the Scalar docs when a spec exists', async () => {
  vi.spyOn(api, 'getProductSpec').mockResolvedValue('{"openapi":"3.0.0"}')
  renderAt('orders')
  expect(await screen.findByRole('heading', { name: /Orders API/ })).toBeInTheDocument()
  expect(screen.getByText('Gère les commandes.')).toBeInTheDocument()
  await waitFor(() => expect(screen.getByTestId('scalar')).toHaveAttribute('data-content', '{"openapi":"3.0.0"}'))
})

it('shows a placeholder when the product has no spec', async () => {
  vi.spyOn(api, 'getProductSpec').mockResolvedValue(null)
  renderAt('orders')
  expect(await screen.findByText(/Documentation bientôt disponible/i)).toBeInTheDocument()
  expect(screen.queryByTestId('scalar')).not.toBeInTheDocument()
})
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd web && pnpm exec vitest run src/pages/ProductDetailPage.test.tsx`
Expected: FAIL — cannot find `./ProductDetailPage`.

- [ ] **Step 3: Implement the page**

Create `web/src/pages/ProductDetailPage.tsx`:
```tsx
import { Suspense, lazy, useEffect, useState } from 'react'
import { Link, useParams, useNavigate } from 'react-router-dom'
import { getProduct, getProductSpec } from '../api/client'
import type { Product } from '../api/types'
import { useAuth } from '../auth/AuthProvider'
import { TopBar } from '../components/TopBar'
import { SubscribeModal } from '../components/SubscribeModal'
import { ApiIcon, categoryDotColor } from '../components/apiIcons'
import '../styles/productdetail.css'

const ScalarDocs = lazy(() => import('../components/ScalarDocs').then(m => ({ default: m.ScalarDocs })))

export function ProductDetailPage() {
  const { slug = '' } = useParams()
  const { user } = useAuth()
  const nav = useNavigate()
  const [product, setProduct] = useState<Product | null>(null)
  const [spec, setSpec] = useState<string | null>(null)
  const [loaded, setLoaded] = useState(false)
  const [err, setErr] = useState('')
  const [subOpen, setSubOpen] = useState(false)

  useEffect(() => {
    let alive = true
    setLoaded(false)
    Promise.all([getProduct(slug), getProductSpec(slug).catch(() => null)])
      .then(([p, s]) => { if (alive) { setProduct(p); setSpec(s) } })
      .catch(() => { if (alive) setErr('Produit introuvable.') })
      .finally(() => { if (alive) setLoaded(true) })
    return () => { alive = false }
  }, [slug])

  return (
    <>
      <TopBar search="" onSearch={() => {}} />
      <div className="apidetail">
        <div className="crumbs"><Link to="/">Catalogue</Link> <span>/</span> <b>{product?.name ?? slug}</b></div>
        {err && <p className="autherr" role="alert">{err}</p>}
        {product && (
          <>
            <header className="apihead">
              <span className="glyph" style={{ background: categoryDotColor(product.category) }}><ApiIcon name={product.icon} /></span>
              <div className="htext">
                <h1>{product.name}</h1>
                <p className="sub"><span className="cat">{product.category}</span> · v{product.version} · ★ {product.rating}</p>
                {product.description && <p className="desc">{product.description}</p>}
                <div className="tags">{product.tags.map(t => <span key={t} className="tag">{t}</span>)}</div>
              </div>
              <button className="btn btn-primary" onClick={() => user ? setSubOpen(true) : nav('/login')}>S'abonner</button>
            </header>

            {loaded && spec && (
              <Suspense fallback={<p className="docs-loading">Chargement de la documentation…</p>}>
                <ScalarDocs spec={spec} />
              </Suspense>
            )}
            {loaded && !spec && (
              <div className="docs-empty"><h3>Documentation bientôt disponible</h3>
                <p>Aucune spécification OpenAPI n'est encore attachée à cette API.</p></div>
            )}
          </>
        )}
      </div>
      {subOpen && product && <SubscribeModal product={product} onClose={() => setSubOpen(false)} />}
    </>
  )
}
```
NOTE: `apiIcons.tsx` exports `ApiIcon`, `categoryTint` (returns a CSS *style object* — used by `ApiCard` as `style={categoryTint(p.category)}`), and `categoryDotColor` (returns a *color string*). The header glyph uses `categoryDotColor` for its background. The subscribe gate mirrors `CatalogPage.handleSubscribe`: `if (!user) nav('/login')` else open the modal (`SubscribeModal` does not redirect on its own).

- [ ] **Step 4: Add minimal styles**

Create `web/src/styles/productdetail.css`:
```css
.apidetail{max-width:1120px;margin:0 auto;padding:26px 28px 80px}
.apidetail .crumbs{display:flex;align-items:center;gap:8px;font-size:13px;color:var(--muted);margin-bottom:18px}
.apidetail .crumbs a:hover{color:var(--fg)}
.apidetail .apihead{display:flex;align-items:flex-start;gap:18px;flex-wrap:wrap;margin-bottom:24px}
.apidetail .apihead .glyph{width:54px;height:54px;border-radius:13px;flex:none;display:grid;place-items:center;color:#fff}
.apidetail .apihead .htext{min-width:0;flex:1}
.apidetail .apihead h1{font-family:var(--font-display);font-size:28px;font-weight:800;letter-spacing:-.02em}
.apidetail .apihead .sub{font-size:13px;color:var(--muted);margin-top:4px}
.apidetail .apihead .cat{color:var(--accent);font-weight:600}
.apidetail .apihead .desc{font-size:14px;color:var(--ink-soft,var(--fg));margin-top:10px;max-width:70ch}
.apidetail .apihead .tags{display:flex;gap:6px;flex-wrap:wrap;margin-top:10px}
.apidetail .apihead .tag{font-size:11px;padding:2px 8px;border-radius:999px;background:var(--accent-soft);color:var(--accent)}
.apidetail .docs-empty{border:1px dashed var(--border-2);border-radius:14px;padding:40px;text-align:center;color:var(--muted)}
.apidetail .docs-empty h3{font-family:var(--font-display);font-size:18px;color:var(--fg);margin-bottom:6px}
.apidetail .docs-loading{color:var(--muted);padding:24px}
.scalar-wrap{border:1px solid var(--border-2);border-radius:14px;overflow:hidden}
```

- [ ] **Step 5: Register the route**

In `web/src/App.tsx`, add the import and route:
```tsx
import { ProductDetailPage } from './pages/ProductDetailPage'
```
```tsx
      <Route path="/apis/:slug" element={<ProductDetailPage />} />
```
(Place it after the `/` catalog route.)

- [ ] **Step 6: Run the tests + full gate**

Run: `cd web && pnpm exec vitest run src/pages/ProductDetailPage.test.tsx && pnpm exec vitest run --exclude 'e2e/**' && pnpm exec tsc --noEmit && pnpm build`
Expected: page tests pass; full suite green; tsc clean; build succeeds (Scalar in its own lazy chunk).

- [ ] **Step 7: Commit**

```bash
git add web/src/pages/ProductDetailPage.tsx web/src/pages/ProductDetailPage.test.tsx web/src/styles/productdetail.css web/src/App.tsx
git commit -m "feat(web): product detail page at /apis/:slug with Scalar docs"
```

---

## Task B4: Catalog cards link to the detail page

**Files:**
- Modify: `web/src/components/ApiCard.tsx`
- Test: `web/src/components/ApiCard.test.tsx`

**Interfaces:**
- Consumes: `Product`, react-router `Link`.
- Produces: the card's title/icon links to `/apis/:slug`; the "S'abonner" button still works (its click must not navigate).

- [ ] **Step 1: Write the failing test**

Add to `web/src/components/ApiCard.test.tsx` (wrap renders in `MemoryRouter` if not already — check the file and add the import):
```tsx
  it('links the card title to the product detail page', () => {
    const p = { id: 1, name: 'Orders API', slug: 'orders', category: 'Data', version: '1.0.0', contextPath: '/orders', description: '', tags: [], icon: '', rating: 4 }
    render(<MemoryRouter><ApiCard p={p} onSubscribe={() => {}} /></MemoryRouter>)
    expect(screen.getByRole('link', { name: /Orders API/ })).toHaveAttribute('href', '/apis/orders')
  })
```
(If the file's existing tests don't use `MemoryRouter`, wrap them too — `ApiCard` will now render a `Link`.)

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd web && pnpm exec vitest run src/components/ApiCard.test.tsx`
Expected: FAIL — no link / `useHref` outside a Router (if existing tests lack a Router).

- [ ] **Step 3: Wrap the card heading in a Link**

In `web/src/components/ApiCard.tsx`, add `import { Link } from 'react-router-dom'` and wrap the product name (and optionally the icon) in:
```tsx
<Link className="api-card-title" to={`/apis/${p.slug}`}>{p.name}</Link>
```
Keep the existing "S'abonner" button as-is; it already calls `onSubscribe(p)` and does not navigate. Do not make the whole card a link (the subscribe button must remain independently clickable).

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd web && pnpm exec vitest run src/components/ApiCard.test.tsx`
Expected: PASS.

- [ ] **Step 5: Full frontend gate + commit**

Run: `cd web && pnpm exec vitest run --exclude 'e2e/**' && pnpm exec tsc --noEmit && pnpm build`
Expected: green.
```bash
git add web/src/components/ApiCard.tsx web/src/components/ApiCard.test.tsx
git commit -m "feat(web): catalog cards link to the product detail page"
```

---

## Task B5: Live verification

- [ ] **Step 1: Ensure a published product with a spec exists**

The dev DB has a `Docs Live` product (slug `docs-live-*`) with a real Petstore spec from Plan A's live check. If not, import one via admin and publish it.

- [ ] **Step 2: Open the detail page in a browser**

With the portal (`:8090`) and Vite (`:5173`) running, navigate to `http://localhost:5173/apis/<slug>` and **look at the screenshot**: the header (name/category/version/rating/description), then the Scalar docs with the spec's operations rendered. Click a catalog card's title and confirm it navigates here.

- [ ] **Step 3: No-spec placeholder**

Open `/apis/<slug>` for a product without a spec (e.g. a seeded one) and confirm the "Documentation bientôt disponible" placeholder shows instead of Scalar.

---

## Self-Review notes

- **Spec coverage (Plan B scope):** new `/apis/:slug` route ✅ (B3); header with name/category/version/rating/tags/description + "S'abonner" reusing SubscribeModal ✅ (B3); Scalar render from `/api/products/{slug}/spec` ✅ (B2+B3); no-spec placeholder ✅ (B3); docs public (no auth) ✅ (page never requires token to read); catalog cards link in ✅ (B4); Scalar lazy-loaded ✅ (B3 `lazy()` + Suspense).
- **Type consistency:** `getProduct`/`getProductSpec`/`ScalarDocs({spec})`/`ProductDetailPage` names consistent across tasks; `ScalarDocs` mock shape matches between B2 and B3 tests.
- **Deferred to Plan C:** Try-it is read-only here; Plan C adds the `POST /api/try/{slug}` proxy and wires Scalar's send action + the subscriber/app-key resolution. The single import→docs→subscribe→try-it e2e lands in Plan C.
- **Implementer notes:** confirm `@scalar/api-reference-react` config prop (`content` vs `spec.content`) and stylesheet path against the installed version, keeping the test mock in sync; mirror `ApiCard`'s exact `apiIcons` imports for the header glyph.
