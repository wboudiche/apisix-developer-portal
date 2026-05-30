# APISIX Developer Portal — Frontend Implementation Plan (Plan 2 of 4)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the React frontend for the portal — port the validated "Atlas" catalog UI into a real React app, wire it to the Plan-1 backend (`/api/products`, `/api/auth/*`), with light/dark theming and local-account login/register.

**Architecture:** Vite + React + TypeScript SPA living in `web/`. A thin typed `fetch` API client talks to the Go backend (dev: Vite proxies `/api` → `http://localhost:8080`). React Router for pages (Catalog, Login, Register). Auth state (JWT in `localStorage`) via a small context. The Atlas design tokens/CSS are ported from the existing `/index.html` mockup into global CSS + focused components. Component tests use Vitest + React Testing Library with `fetch` mocked.

**Tech Stack:** Node 20+/npm, Vite, React 18, TypeScript, react-router-dom v6, Vitest, @testing-library/react, jsdom. Fonts: Bricolage Grotesque + IBM Plex Sans + JetBrains Mono (as in Atlas).

This is Plan 2 of 4 (Foundation ✓ → **Frontend** → Core subscribe loop → Admin). Backend must be running (`make up && go run ./cmd/portal`) for live use; tests mock the network and need no backend. Reference design: `/index.html`. Spec: `docs/superpowers/specs/2026-05-29-apisix-developer-portal-design.md`.

---

## File structure (created by this plan)

```
web/
├── package.json, vite.config.ts, tsconfig.json, index.html
├── src/
│   ├── main.tsx                 # React root + Router + ThemeProvider + AuthProvider
│   ├── App.tsx                  # routes
│   ├── styles/tokens.css        # Atlas :root tokens (light + dark) — ported from /index.html
│   ├── styles/base.css          # resets, fonts, element base
│   ├── api/types.ts             # Product, User, AuthResponse
│   ├── api/client.ts            # getProducts(query), register, login
│   ├── api/client.test.ts
│   ├── theme/ThemeProvider.tsx  # light/dark via data-theme + localStorage
│   ├── auth/AuthProvider.tsx    # token storage, login/register/logout, useAuth()
│   ├── auth/AuthProvider.test.tsx
│   ├── components/TopBar.tsx
│   ├── components/CategoryRail.tsx
│   ├── components/ApiCard.tsx
│   ├── pages/CatalogPage.tsx
│   ├── pages/CatalogPage.test.tsx
│   ├── pages/LoginPage.tsx
│   ├── pages/RegisterPage.tsx
│   └── pages/AuthPages.test.tsx
└── (vitest config inside vite.config.ts)
```

Each component has one responsibility. The API client is the only place that knows URLs/shapes. Theme and auth are isolated providers consumed via hooks.

---

## Task 1: Scaffold the Vite React+TS app

**Files:** create `web/` via Vite, add deps, configure Vitest + dev proxy.

- [ ] **Step 1: Scaffold**

Run:
```bash
cd /home/walidboudiche/working/apisix-developper-portal
npm create vite@latest web -- --template react-ts
cd web
npm install
npm install react-router-dom
npm install -D vitest @testing-library/react @testing-library/jest-dom @testing-library/user-event jsdom
```
Expected: `web/` created with a React+TS template; deps installed.

- [ ] **Step 2: Configure `web/vite.config.ts`** (dev proxy + vitest)

```ts
/// <reference types="vitest" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: { '/api': 'http://localhost:8080' },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: './src/setupTests.ts',
  },
})
```

- [ ] **Step 3: Create `web/src/setupTests.ts`**

```ts
import '@testing-library/jest-dom'
```

- [ ] **Step 4: Add test script** — in `web/package.json` `scripts`, add `"test": "vitest run"` and `"test:watch": "vitest"`.

- [ ] **Step 5: Verify build & empty test run**

Run: `cd web && npm run build && npx vitest run --passWithNoTests`
Expected: build succeeds; vitest reports no tests (exit 0).

- [ ] **Step 6: Commit**

```bash
cd /home/walidboudiche/working/apisix-developper-portal
git add web
git commit -m "feat(web): scaffold Vite React+TS app with vitest and /api dev proxy"
```

---

## Task 2: Design tokens, fonts, base styles (port from Atlas)

**Files:** `web/src/styles/tokens.css`, `web/src/styles/base.css`; import them in `web/src/main.tsx` (created in Task 7 — for now import in `web/src/App.tsx` placeholder). Delete the Vite template CSS (`web/src/App.css`, `web/src/index.css`) to avoid conflicts.

- [ ] **Step 1: Create `web/src/styles/tokens.css`**

Port the Atlas palette from `/index.html` (its `:root{…}` block, lines ~11-40) into a `:root` (light) set, and add a dark override under `:root[data-theme="dark"]`. Use exactly the Atlas light values for `:root`. Add this dark block:

```css
/* tokens.css */
:root{
  /* === LIGHT (ported verbatim from /index.html :root) === */
  --bg: oklch(98.5% 0.004 255);
  --surface: oklch(100% 0 0);
  --ink: oklch(27% 0.105 27);
  --ink-soft: oklch(34% 0.115 27);
  --ink-line: oklch(45% 0.10 27);
  --fg: oklch(25% 0.02 262);
  --muted: oklch(53% 0.015 262);
  --faint: oklch(67% 0.012 262);
  --border: oklch(91% 0.006 262);
  --border-2: oklch(86% 0.008 262);
  --accent: oklch(55% 0.205 27);
  --accent-d: oklch(47% 0.19 27);
  --accent-soft: oklch(95% 0.04 27);
  --warn: oklch(74% 0.15 78);
  --c-admin: oklch(56% 0.035 262);
  --c-finance: oklch(54% 0.09 158);
  --c-marketing: oklch(64% 0.135 55);
  --c-eng: oklch(55% 0.11 288);
  --r: 14px;
  --shadow: 0 1px 2px oklch(24% 0.03 262 / .05), 0 8px 24px oklch(24% 0.03 262 / .06);
  --shadow-h: 0 2px 4px oklch(24% 0.03 262 / .07), 0 16px 40px oklch(24% 0.03 262 / .12);
  --topbar-h: 62px;
  --rail-w: 270px;
  --font-display:'Bricolage Grotesque',-apple-system,system-ui,sans-serif;
  --font-body:'IBM Plex Sans',-apple-system,system-ui,sans-serif;
  --font-mono:'JetBrains Mono',ui-monospace,Menlo,monospace;
}
:root[data-theme="dark"]{
  --bg: oklch(20% 0.012 262);
  --surface: oklch(25% 0.013 262);
  --ink: oklch(22% 0.02 262);
  --ink-soft: oklch(30% 0.02 262);
  --ink-line: oklch(38% 0.02 262);
  --fg: oklch(93% 0.012 262);
  --muted: oklch(72% 0.015 262);
  --faint: oklch(58% 0.013 262);
  --border: oklch(33% 0.012 262);
  --border-2: oklch(40% 0.014 262);
  --accent: oklch(64% 0.2 27);
  --accent-d: oklch(56% 0.19 27);
  --accent-soft: oklch(33% 0.07 27);
  --shadow: 0 1px 2px oklch(0% 0 0 / .3), 0 8px 24px oklch(0% 0 0 / .4);
  --shadow-h: 0 2px 4px oklch(0% 0 0 / .4), 0 16px 40px oklch(0% 0 0 / .5);
}
```

- [ ] **Step 2: Create `web/src/styles/base.css`**

```css
@import url('https://fonts.googleapis.com/css2?family=Bricolage+Grotesque:opsz,wght@12..96,500;12..96,600;12..96,700&family=IBM+Plex+Sans:wght@400;500;600&family=JetBrains+Mono:wght@400;500;600&display=swap');

*{box-sizing:border-box;margin:0;padding:0}
html{scroll-behavior:smooth}
body{
  font-family:var(--font-body);
  background:var(--bg);
  color:var(--fg);
  -webkit-font-smoothing:antialiased;
  line-height:1.5;
  transition:background-color .2s, color .2s;
}
::selection{background:var(--accent-soft)}
button{font-family:inherit;cursor:pointer}
a{color:inherit;text-decoration:none}
```

- [ ] **Step 3: Remove template CSS**

Run: `cd web && rm -f src/App.css src/index.css` and remove their imports from `src/main.tsx`/`src/App.tsx` (the Vite template imports `./index.css` in main.tsx and `./App.css` in App.tsx — delete those import lines). Then in `src/App.tsx` temporarily add `import './styles/tokens.css'; import './styles/base.css'` so styles load until main.tsx is finalized in Task 7.

- [ ] **Step 4: Verify**

Run: `cd web && npm run build`
Expected: builds with no missing-import errors.

- [ ] **Step 5: Commit**

```bash
cd /home/walidboudiche/working/apisix-developper-portal
git add web/src/styles web/src/App.tsx web/src/main.tsx
git rm --cached web/src/App.css web/src/index.css 2>/dev/null || true
git commit -m "feat(web): port Atlas design tokens (light+dark) and base styles"
```

---

## Task 3: Typed API client (TDD, fetch mocked)

**Files:** `web/src/api/types.ts`, `web/src/api/client.ts`, `web/src/api/client.test.ts`

- [ ] **Step 1: Create `web/src/api/types.ts`**

```ts
export interface Product {
  id: number
  name: string
  slug: string
  category: string
  version: string
  contextPath: string
  description: string
  tags: string[]
  icon: string
  rating: number
}

export interface User {
  id: number
  email: string
  name: string
  role: string
}

export interface AuthResponse {
  user: User
  token: string
}

export interface ProductQuery {
  search?: string
  category?: string
  tag?: string
  sort?: 'alpha' | 'rating'
}
```

- [ ] **Step 2: Write failing test `web/src/api/client.test.ts`**

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { getProducts, login, register } from './client'

beforeEach(() => { vi.restoreAllMocks() })

function mockFetch(status: number, body: unknown) {
  globalThis.fetch = vi.fn().mockResolvedValue({
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
  }) as unknown as typeof fetch
}

describe('getProducts', () => {
  it('GETs /api/products and returns the array', async () => {
    mockFetch(200, [{ id: 1, name: 'Orders', slug: 'orders', tags: [] }])
    const out = await getProducts({})
    expect(out).toHaveLength(1)
    expect((globalThis.fetch as any).mock.calls[0][0]).toBe('/api/products')
  })

  it('encodes query params', async () => {
    mockFetch(200, [])
    await getProducts({ search: 'pi zza', category: 'Finance', sort: 'alpha' })
    const url = (globalThis.fetch as any).mock.calls[0][0] as string
    expect(url).toContain('/api/products?')
    expect(url).toContain('search=pi+zza')
    expect(url).toContain('category=Finance')
    expect(url).toContain('sort=alpha')
  })
})

describe('login/register', () => {
  it('login POSTs credentials and returns AuthResponse', async () => {
    mockFetch(200, { user: { id: 1, email: 'a@b.c', name: '', role: 'developer' }, token: 'jwt' })
    const res = await login('a@b.c', 'pw12345678')
    expect(res.token).toBe('jwt')
    const [url, opts] = (globalThis.fetch as any).mock.calls[0]
    expect(url).toBe('/api/auth/login')
    expect(opts.method).toBe('POST')
  })

  it('register throws on non-2xx with the server error message', async () => {
    mockFetch(409, { error: 'email already registered' })
    await expect(register('a@b.c', 'pw12345678', 'A')).rejects.toThrow('email already registered')
  })
})
```

- [ ] **Step 3: Run → fails** (`cd web && npx vitest run src/api/client.test.ts`) — module/exports undefined.

- [ ] **Step 4: Implement `web/src/api/client.ts`**

```ts
import type { Product, AuthResponse, ProductQuery } from './types'

async function parse<T>(res: Response): Promise<T> {
  const body = await res.json().catch(() => ({}))
  if (!res.ok) {
    const msg = (body as { error?: string }).error || `request failed (${res.status})`
    throw new Error(msg)
  }
  return body as T
}

export async function getProducts(q: ProductQuery): Promise<Product[]> {
  const params = new URLSearchParams()
  if (q.search) params.set('search', q.search)
  if (q.category) params.set('category', q.category)
  if (q.tag) params.set('tag', q.tag)
  if (q.sort) params.set('sort', q.sort)
  const qs = params.toString()
  const res = await fetch(qs ? `/api/products?${qs}` : '/api/products')
  return parse<Product[]>(res)
}

function postJSON(url: string, body: unknown): Promise<Response> {
  return fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
}

export async function login(email: string, password: string): Promise<AuthResponse> {
  return parse<AuthResponse>(await postJSON('/api/auth/login', { email, password }))
}

export async function register(email: string, password: string, name: string): Promise<AuthResponse> {
  return parse<AuthResponse>(await postJSON('/api/auth/register', { email, password, name }))
}
```

- [ ] **Step 5: Run → passes** (`cd web && npx vitest run src/api/client.test.ts` → all pass).

- [ ] **Step 6: Commit**

```bash
git add web/src/api
git commit -m "feat(web): typed API client for products + auth (TDD)"
```

---

## Task 4: Theme + Auth providers (TDD for auth)

**Files:** `web/src/theme/ThemeProvider.tsx`, `web/src/auth/AuthProvider.tsx`, `web/src/auth/AuthProvider.test.tsx`

- [ ] **Step 1: Create `web/src/theme/ThemeProvider.tsx`**

```tsx
import { createContext, useContext, useEffect, useState, type ReactNode } from 'react'

type Theme = 'light' | 'dark'
const ThemeCtx = createContext<{ theme: Theme; toggle: () => void }>({ theme: 'light', toggle: () => {} })

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setTheme] = useState<Theme>(() => (localStorage.getItem('theme') as Theme) || 'light')
  useEffect(() => {
    document.documentElement.setAttribute('data-theme', theme)
    localStorage.setItem('theme', theme)
  }, [theme])
  const toggle = () => setTheme(t => (t === 'light' ? 'dark' : 'light'))
  return <ThemeCtx.Provider value={{ theme, toggle }}>{children}</ThemeCtx.Provider>
}

export const useTheme = () => useContext(ThemeCtx)
```

- [ ] **Step 2: Write failing test `web/src/auth/AuthProvider.test.tsx`**

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AuthProvider, useAuth } from './AuthProvider'
import * as client from '../api/client'

beforeEach(() => { localStorage.clear(); vi.restoreAllMocks() })

function Probe() {
  const { user, login, logout } = useAuth()
  return (
    <div>
      <span data-testid="who">{user ? user.email : 'anon'}</span>
      <button onClick={() => login('a@b.c', 'pw12345678')}>login</button>
      <button onClick={logout}>logout</button>
    </div>
  )
}

describe('AuthProvider', () => {
  it('logs in, exposes user, persists token, then logs out', async () => {
    vi.spyOn(client, 'login').mockResolvedValue({
      user: { id: 1, email: 'a@b.c', name: '', role: 'developer' }, token: 'jwt-123',
    })
    render(<AuthProvider><Probe /></AuthProvider>)
    expect(screen.getByTestId('who').textContent).toBe('anon')

    await userEvent.click(screen.getByText('login'))
    await waitFor(() => expect(screen.getByTestId('who').textContent).toBe('a@b.c'))
    expect(localStorage.getItem('token')).toBe('jwt-123')

    await userEvent.click(screen.getByText('logout'))
    await waitFor(() => expect(screen.getByTestId('who').textContent).toBe('anon'))
    expect(localStorage.getItem('token')).toBeNull()
  })
})
```

- [ ] **Step 3: Run → fails.**

- [ ] **Step 4: Implement `web/src/auth/AuthProvider.tsx`**

```tsx
import { createContext, useContext, useState, type ReactNode } from 'react'
import type { User } from '../api/types'
import * as api from '../api/client'

interface AuthState {
  user: User | null
  token: string | null
  login: (email: string, password: string) => Promise<void>
  register: (email: string, password: string, name: string) => Promise<void>
  logout: () => void
}

const AuthCtx = createContext<AuthState | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [token, setToken] = useState<string | null>(() => localStorage.getItem('token'))
  const [user, setUser] = useState<User | null>(() => {
    const raw = localStorage.getItem('user')
    return raw ? (JSON.parse(raw) as User) : null
  })

  function apply(res: { user: User; token: string }) {
    setUser(res.user); setToken(res.token)
    localStorage.setItem('token', res.token)
    localStorage.setItem('user', JSON.stringify(res.user))
  }

  const login = async (email: string, password: string) => apply(await api.login(email, password))
  const register = async (email: string, password: string, name: string) => apply(await api.register(email, password, name))
  const logout = () => {
    setUser(null); setToken(null)
    localStorage.removeItem('token'); localStorage.removeItem('user')
  }

  return <AuthCtx.Provider value={{ user, token, login, register, logout }}>{children}</AuthCtx.Provider>
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthCtx)
  if (!ctx) throw new Error('useAuth must be used within AuthProvider')
  return ctx
}
```

- [ ] **Step 5: Run → passes.**

- [ ] **Step 6: Commit**

```bash
git add web/src/theme web/src/auth
git commit -m "feat(web): theme (light/dark) and auth providers (TDD)"
```

---

## Task 5: Catalog UI — TopBar, CategoryRail, ApiCard, CatalogPage (TDD)

**Files:** `web/src/components/TopBar.tsx`, `web/src/components/CategoryRail.tsx`, `web/src/components/ApiCard.tsx`, `web/src/pages/CatalogPage.tsx`, `web/src/pages/CatalogPage.test.tsx`. Port markup/CSS from `/index.html` (topbar, rail, cards). Co-locate component CSS as a single `web/src/styles/catalog.css` imported by CatalogPage (port the relevant `.topbar/.rail/.cat/.card/...` rules from `/index.html`, which already use the tokens from Task 2).

- [ ] **Step 1: Create `web/src/styles/catalog.css`** — copy the component CSS rules for `.topbar`, `.brand`, `.nav-tabs`, `.search`, `.rail`, `.cat`, `.tags/.tag`, `.content`, `.chead`, `.grid`, `.card`, `.thumb`, `.cbody`, `.cfoot`, `.subbtn`, the responsive blocks, etc. from `/index.html`'s `<style>` (lines ~55-313). They already reference the Task-2 tokens, so they work as-is. (Engineer: open `/index.html` and copy those rule blocks verbatim.)

- [ ] **Step 2: Create `web/src/components/ApiCard.tsx`**

```tsx
import type { Product } from '../api/types'

const CAT_DOT: Record<string, string> = {
  Administration: 'var(--c-admin)', Finance: 'var(--c-finance)',
  Marketing: 'var(--c-marketing)', Engineering: 'var(--c-eng)',
}

export function ApiCard({ p }: { p: Product }) {
  return (
    <article className="card in" data-testid="api-card">
      <div className="thumb">
        <span className="catbadge">{p.category}</span>
        <span className="ico" style={{ background: `linear-gradient(150deg, ${CAT_DOT[p.category] ?? 'var(--accent)'}, var(--accent-d))` }}>
          {p.icon.slice(0, 2).toUpperCase()}
        </span>
      </div>
      <div className="cbody">
        <div className="crow1"><span className="cname">{p.name}</span></div>
        <p className="cdesc">{p.description}</p>
        <div className="cmeta">
          <span className="pill">v<b>{p.version}</b></span>
          <span className="pill ctx">{p.contextPath}</span>
        </div>
        <div className="cfoot">
          <div className="ctags">{p.tags.slice(0, 2).map(t => <span key={t} className="ctag">{t}</span>)}</div>
          <button className="subbtn">S'abonner</button>
        </div>
      </div>
    </article>
  )
}
```

- [ ] **Step 3: Create `web/src/components/TopBar.tsx`**

```tsx
import { useTheme } from '../theme/ThemeProvider'
import { useAuth } from '../auth/AuthProvider'
import { Link } from 'react-router-dom'

export function TopBar({ search, onSearch }: { search: string; onSearch: (v: string) => void }) {
  const { theme, toggle } = useTheme()
  const { user, logout } = useAuth()
  return (
    <header className="topbar">
      <Link className="brand" to="/"><span className="name">Atlas</span></Link>
      <nav className="nav-tabs"><Link className="active" to="/">APIs</Link></nav>
      <div className="search">
        <input value={search} onChange={e => onSearch(e.target.value)} placeholder="Rechercher une API, un tag…" aria-label="Rechercher" />
      </div>
      <button className="icon-btn" onClick={toggle} aria-label="Basculer le thème">{theme === 'dark' ? '☀' : '☾'}</button>
      {user
        ? <button className="icon-btn" onClick={logout}>{user.email.slice(0, 2).toUpperCase()}</button>
        : <Link className="icon-btn" to="/login">Connexion</Link>}
    </header>
  )
}
```

- [ ] **Step 4: Create `web/src/components/CategoryRail.tsx`**

```tsx
export function CategoryRail({
  categories, active, onPick,
}: { categories: { name: string; count: number }[]; active: string | null; onPick: (c: string | null) => void }) {
  return (
    <aside className="rail">
      <div className="rail-head"><h2>Catégories d'API</h2></div>
      <nav>
        <button className={`cat ${active === null ? 'active' : ''}`} onClick={() => onPick(null)}>
          <span className="clabel">Toutes les catégories</span>
          <span className="cnt">{categories.reduce((n, c) => n + c.count, 0)}</span>
        </button>
        {categories.map(c => (
          <button key={c.name} className={`cat ${active === c.name ? 'active' : ''}`} onClick={() => onPick(c.name)}>
            <span className="clabel">{c.name}</span><span className="cnt">{c.count}</span>
          </button>
        ))}
      </nav>
    </aside>
  )
}
```

- [ ] **Step 5: Write failing test `web/src/pages/CatalogPage.test.tsx`**

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { CatalogPage } from './CatalogPage'
import { ThemeProvider } from '../theme/ThemeProvider'
import { AuthProvider } from '../auth/AuthProvider'
import * as api from '../api/client'
import type { Product } from '../api/types'

const sample: Product[] = [
  { id: 1, name: 'PizzaShackAPI', slug: 'pizzashackapi', category: 'Engineering', version: '1.0.0', contextPath: '/pizzashack', description: 'demo', tags: ['pizza'], icon: 'pi', rating: 4.5 },
  { id: 2, name: 'CurrencyConverterAPI', slug: 'currencyconverterapi', category: 'Finance', version: '1.0.0', contextPath: '/currencyconv', description: 'fx', tags: ['devises'], icon: 'cu', rating: 5 },
]

function renderPage() {
  return render(
    <MemoryRouter><ThemeProvider><AuthProvider><CatalogPage /></AuthProvider></ThemeProvider></MemoryRouter>
  )
}

beforeEach(() => { localStorage.clear(); vi.restoreAllMocks() })

describe('CatalogPage', () => {
  it('loads and renders all products from the API', async () => {
    const spy = vi.spyOn(api, 'getProducts').mockResolvedValue(sample)
    renderPage()
    await waitFor(() => expect(screen.getAllByTestId('api-card')).toHaveLength(2))
    expect(spy).toHaveBeenCalledWith({}) // initial unfiltered load
    expect(screen.getByText('PizzaShackAPI')).toBeInTheDocument()
  })

  it('re-queries the API when the user types a search', async () => {
    const spy = vi.spyOn(api, 'getProducts').mockResolvedValue(sample)
    renderPage()
    await waitFor(() => expect(screen.getAllByTestId('api-card')).toHaveLength(2))
    await userEvent.type(screen.getByLabelText('Rechercher'), 'pizza')
    await waitFor(() => expect(spy).toHaveBeenCalledWith(expect.objectContaining({ search: 'pizza' })))
  })
})
```

- [ ] **Step 6: Run → fails** (`CatalogPage` undefined).

- [ ] **Step 7: Implement `web/src/pages/CatalogPage.tsx`**

```tsx
import { useEffect, useMemo, useState } from 'react'
import { getProducts } from '../api/client'
import type { Product } from '../api/types'
import { TopBar } from '../components/TopBar'
import { CategoryRail } from '../components/CategoryRail'
import { ApiCard } from '../components/ApiCard'
import '../styles/catalog.css'

export function CatalogPage() {
  const [products, setProducts] = useState<Product[]>([])
  const [search, setSearch] = useState('')
  const [category, setCategory] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let alive = true
    setLoading(true)
    getProducts({ search: search || undefined, category: category || undefined })
      .then(p => { if (alive) setProducts(p) })
      .finally(() => { if (alive) setLoading(false) })
    return () => { alive = false }
  }, [search, category])

  // category counts come from the unfiltered list; recompute from current load when unfiltered
  const categories = useMemo(() => {
    const counts: Record<string, number> = {}
    products.forEach(p => { counts[p.category] = (counts[p.category] ?? 0) + 1 })
    return Object.entries(counts).map(([name, count]) => ({ name, count }))
  }, [products])

  return (
    <>
      <TopBar search={search} onSearch={setSearch} />
      <div className="layout">
        <CategoryRail categories={categories} active={category} onPick={setCategory} />
        <main className="content">
          <div className="chead"><div className="titlewrap"><h1>Catalogue d'API</h1>
            <p className="rescount"><b>{products.length}</b> API{products.length > 1 ? 's' : ''}</p></div></div>
          <div className="grid">
            {products.map(p => <ApiCard key={p.id} p={p} />)}
          </div>
          {!loading && products.length === 0 && <p className="rescount">Aucune API ne correspond.</p>}
        </main>
      </div>
    </>
  )
}
```

- [ ] **Step 8: Run → passes** (`cd web && npx vitest run src/pages/CatalogPage.test.tsx`).

- [ ] **Step 9: Commit**

```bash
git add web/src/components web/src/pages/CatalogPage.tsx web/src/pages/CatalogPage.test.tsx web/src/styles/catalog.css
git commit -m "feat(web): catalog page with TopBar/Rail/Card wired to API (TDD)"
```

---

## Task 6: Login & Register pages (TDD)

**Files:** `web/src/pages/LoginPage.tsx`, `web/src/pages/RegisterPage.tsx`, `web/src/pages/AuthPages.test.tsx`

- [ ] **Step 1: Write failing test `web/src/pages/AuthPages.test.tsx`**

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { LoginPage } from './LoginPage'
import { AuthProvider } from '../auth/AuthProvider'
import * as api from '../api/client'

beforeEach(() => { localStorage.clear(); vi.restoreAllMocks() })

function renderLogin() {
  return render(
    <MemoryRouter initialEntries={['/login']}>
      <AuthProvider>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/" element={<div>CATALOG HOME</div>} />
        </Routes>
      </AuthProvider>
    </MemoryRouter>
  )
}

describe('LoginPage', () => {
  it('submits credentials and navigates home on success', async () => {
    vi.spyOn(api, 'login').mockResolvedValue({ user: { id: 1, email: 'a@b.c', name: '', role: 'developer' }, token: 'jwt' })
    renderLogin()
    await userEvent.type(screen.getByLabelText('Email'), 'a@b.c')
    await userEvent.type(screen.getByLabelText('Mot de passe'), 'pw12345678')
    await userEvent.click(screen.getByRole('button', { name: /connexion/i }))
    await waitFor(() => expect(screen.getByText('CATALOG HOME')).toBeInTheDocument())
  })

  it('shows the server error on failure', async () => {
    vi.spyOn(api, 'login').mockRejectedValue(new Error('invalid credentials'))
    renderLogin()
    await userEvent.type(screen.getByLabelText('Email'), 'a@b.c')
    await userEvent.type(screen.getByLabelText('Mot de passe'), 'wrongpass')
    await userEvent.click(screen.getByRole('button', { name: /connexion/i }))
    await waitFor(() => expect(screen.getByText('invalid credentials')).toBeInTheDocument())
  })
})
```

- [ ] **Step 2: Run → fails.**

- [ ] **Step 3: Implement `web/src/pages/LoginPage.tsx`**

```tsx
import { useState, type FormEvent } from 'react'
import { useNavigate, Link } from 'react-router-dom'
import { useAuth } from '../auth/AuthProvider'

export function LoginPage() {
  const { login } = useAuth()
  const nav = useNavigate()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [err, setErr] = useState('')

  async function onSubmit(e: FormEvent) {
    e.preventDefault(); setErr('')
    try { await login(email, password); nav('/') }
    catch (e) { setErr(e instanceof Error ? e.message : 'Échec de connexion') }
  }

  return (
    <form className="authcard" onSubmit={onSubmit}>
      <h1>Connexion</h1>
      <label>Email<input aria-label="Email" type="email" value={email} onChange={e => setEmail(e.target.value)} required /></label>
      <label>Mot de passe<input aria-label="Mot de passe" type="password" value={password} onChange={e => setPassword(e.target.value)} required /></label>
      {err && <p className="autherr" role="alert">{err}</p>}
      <button className="subbtn" type="submit">Connexion</button>
      <p>Pas de compte ? <Link to="/register">Créer un compte</Link></p>
    </form>
  )
}
```

- [ ] **Step 4: Implement `web/src/pages/RegisterPage.tsx`**

```tsx
import { useState, type FormEvent } from 'react'
import { useNavigate, Link } from 'react-router-dom'
import { useAuth } from '../auth/AuthProvider'

export function RegisterPage() {
  const { register } = useAuth()
  const nav = useNavigate()
  const [email, setEmail] = useState('')
  const [name, setName] = useState('')
  const [password, setPassword] = useState('')
  const [err, setErr] = useState('')

  async function onSubmit(e: FormEvent) {
    e.preventDefault(); setErr('')
    if (password.length < 8) { setErr('Mot de passe : 8 caractères minimum'); return }
    try { await register(email, password, name); nav('/') }
    catch (e) { setErr(e instanceof Error ? e.message : "Échec de l'inscription") }
  }

  return (
    <form className="authcard" onSubmit={onSubmit}>
      <h1>Créer un compte</h1>
      <label>Nom<input aria-label="Nom" value={name} onChange={e => setName(e.target.value)} /></label>
      <label>Email<input aria-label="Email" type="email" value={email} onChange={e => setEmail(e.target.value)} required /></label>
      <label>Mot de passe<input aria-label="Mot de passe" type="password" value={password} onChange={e => setPassword(e.target.value)} required /></label>
      {err && <p className="autherr" role="alert">{err}</p>}
      <button className="subbtn" type="submit">Créer le compte</button>
      <p>Déjà inscrit ? <Link to="/login">Se connecter</Link></p>
    </form>
  )
}
```

- [ ] **Step 5: Add minimal auth-form CSS** — append to `web/src/styles/base.css`:

```css
.authcard{max-width:380px;margin:64px auto;display:flex;flex-direction:column;gap:14px;background:var(--surface);border:1px solid var(--border);border-radius:var(--r);padding:28px;box-shadow:var(--shadow)}
.authcard h1{font-family:var(--font-display);font-size:24px}
.authcard label{display:flex;flex-direction:column;gap:6px;font-size:13px;color:var(--muted)}
.authcard input{height:40px;border:1px solid var(--border-2);border-radius:10px;background:var(--bg);padding:0 12px;font-size:14px;color:var(--fg)}
.authcard input:focus{outline:none;border-color:var(--accent);box-shadow:0 0 0 4px var(--accent-soft)}
.authcard .subbtn{justify-content:center;border:1px solid var(--accent);color:#fff;background:var(--accent);padding:10px;border-radius:10px;font-weight:600}
.autherr{color:var(--accent-d);font-size:13px}
```

- [ ] **Step 6: Run → passes** (`cd web && npx vitest run src/pages/AuthPages.test.tsx`).

- [ ] **Step 7: Commit**

```bash
git add web/src/pages/LoginPage.tsx web/src/pages/RegisterPage.tsx web/src/pages/AuthPages.test.tsx web/src/styles/base.css
git commit -m "feat(web): login & register pages wired to auth (TDD)"
```

---

## Task 7: App shell, routing, run & build

**Files:** `web/src/App.tsx`, `web/src/main.tsx`

- [ ] **Step 1: Implement `web/src/main.tsx`**

```tsx
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import './styles/tokens.css'
import './styles/base.css'
import { ThemeProvider } from './theme/ThemeProvider'
import { AuthProvider } from './auth/AuthProvider'
import App from './App'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <BrowserRouter>
      <ThemeProvider>
        <AuthProvider>
          <App />
        </AuthProvider>
      </ThemeProvider>
    </BrowserRouter>
  </StrictMode>,
)
```

- [ ] **Step 2: Implement `web/src/App.tsx`**

```tsx
import { Routes, Route } from 'react-router-dom'
import { CatalogPage } from './pages/CatalogPage'
import { LoginPage } from './pages/LoginPage'
import { RegisterPage } from './pages/RegisterPage'

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<CatalogPage />} />
      <Route path="/login" element={<LoginPage />} />
      <Route path="/register" element={<RegisterPage />} />
    </Routes>
  )
}
```

(Remove the leftover temporary token/base imports from the Task-2 placeholder in App.tsx — they now live in main.tsx.)

- [ ] **Step 3: Full test + build**

Run: `cd web && npm run test && npm run build`
Expected: all Vitest suites pass (api, auth, catalog, auth pages); production build succeeds.

- [ ] **Step 4: Manual run against the backend (smoke)**

Run the backend (`cd .. && make up && go run ./cmd/portal &` on :8080), then `cd web && npm run dev`. In the browser at the Vite URL: the catalog shows the 9 seeded APIs; typing in search filters; clicking a category filters; the theme toggle flips light/dark; `/register` then `/login` work and the topbar shows the user. (Report what you observed; then stop the dev server and backend.)

- [ ] **Step 5: Commit**

```bash
cd /home/walidboudiche/working/apisix-developper-portal
git add web/src/App.tsx web/src/main.tsx
git commit -m "feat(web): app shell + routing (catalog, login, register)"
```

---

## Self-review notes (author)

- **Spec coverage:** catalog browse/search/filter wired to real API (Tasks 3,5) ✓; light+dark theme (Tasks 2,4) ✓; local login/register against `/api/auth` (Tasks 4,6) ✓; Atlas visual design ported (Tasks 2,5) ✓. Subscribe action is a non-functional button here (the real subscribe/credentials flow is Plan 3, by design); sort/tag are supported by the client but the catalog UI surfaces search+category first (sort/tag controls can be added in Plan 3 alongside subscribe). View-toggle (grid/list) from Atlas is deferred as cosmetic.
- **No placeholders:** all code is complete; CSS porting steps reference exact line ranges in the in-repo `/index.html`.
- **Type consistency:** `Product`/`User`/`AuthResponse`/`ProductQuery` defined once in `api/types.ts` and reused; `getProducts/login/register` signatures match across client, providers, and pages; `useAuth()`/`useTheme()` hook shapes are consistent across consumers.
```
