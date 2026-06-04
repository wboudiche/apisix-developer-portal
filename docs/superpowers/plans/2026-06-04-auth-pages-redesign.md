# Auth Pages Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the plain login/register forms with the split-screen design from the user's `/login.html` blueprint (crimson APISIX vitrine left, form card right), pixel-faithful including placeholder controls, with live catalog stats.

**Architecture:** A shared `AuthShell` component owns the split-screen layout and fetches catalog stats; `LoginPage` and `RegisterPage` render blueprint-faithful form cards inside it. All blueprint CSS is ported once into `web/src/styles/auth.css`, mapped onto the existing design tokens (so dark mode works) and scoped under `.auth-shell` to avoid collisions with `catalog.css`.

**Tech Stack:** React 19 + TypeScript, react-router-dom v7, Vitest + React Testing Library (jsdom), plain CSS with oklch tokens.

**Spec:** `docs/superpowers/specs/2026-06-04-auth-pages-design.md`
**Blueprint (source of truth for visuals):** `/login.html` at repo root.

**CRITICAL trap:** `catalog.css` defines `.card{opacity:0;transform:translateY(14px)}` (entrance animation, only `.card.in` is visible) and CSS is bundled globally by Vite. The auth form card therefore must NOT use the blueprint's `.card` class name — this plan renames it `.acard`. Similarly the spinner keyframes are named `auth-spin` to avoid clashing with any `spin` keyframes. Do not "simplify" these names back.

---

### Task 1: Add `--danger` and `--success` tokens

The blueprint uses `--danger` (invalid fields) and `--success`; `tokens.css` doesn't define them.

**Files:**
- Modify: `web/src/styles/tokens.css`

- [ ] **Step 1: Add tokens to the light theme block**

In `web/src/styles/tokens.css`, inside the `:root{...}` block, immediately after the line `--accent-soft: oklch(95% 0.04 27);` add:

```css
  --success: oklch(58% 0.13 155);
  --danger: oklch(58% 0.20 25);
```

- [ ] **Step 2: Add tokens to the dark theme block**

Inside `:root[data-theme="dark"]{...}`, immediately after the line `--accent-soft: oklch(34% 0.075 27);` add:

```css
  --success: oklch(66% 0.13 155);
  --danger: oklch(66% 0.19 25);
```

(Lightened for contrast on dark backgrounds, same pattern as the dark `--accent`.)

- [ ] **Step 3: Verify the web build still compiles**

Run: `cd web && npx vitest run src/pages/AuthPages.test.tsx`
Expected: PASS (2 tests) — proves no CSS syntax error breaks the bundle.

- [ ] **Step 4: Commit**

```bash
git add web/src/styles/tokens.css
git commit -m "feat(web): add --danger/--success tokens for auth pages"
```

---

### Task 2: Create `auth.css` (full blueprint port, scoped)

**Files:**
- Create: `web/src/styles/auth.css`

- [ ] **Step 1: Confirm the collision list**

Run: `cd web && grep -nE '\.(acard|aside|a-brand|a-mark|a-body|a-eyebrow|a-stats|m-head|m-brand|field|wrap|pw-toggle|err|row-between|remember|forgot|submit|spin|enterprise|legal|auth-shell|auth-main)\b' src/styles/catalog.css src/styles/base.css`
Expected: no output (none of the auth class names are already taken). If any line matches, rename that auth class with an `a-` prefix in the steps below and keep going.

- [ ] **Step 2: Write `web/src/styles/auth.css`**

Create the file with exactly this content (port of `/login.html` styles: tokens substituted, all rules scoped under `.auth-shell`, `.card`→`.acard`, keyframes→`auth-spin`; the vitrine's literal dark-crimson colors are intentional — it stays dark in both themes):

```css
/* Auth pages — split-screen layout ported from /login.html (the user's
   blueprint). Every rule is scoped under .auth-shell so nothing leaks into
   catalog/base styles. The form card is .acard, NOT .card: catalog.css gives
   .card opacity:0 (entrance animation) and CSS is global. */

.auth-shell{
  min-height:100vh;
  display:grid;
  grid-template-columns:1.05fr 1fr;
}

/* ============ LEFT — vitrine rouge APISIX ============ */
.auth-shell .aside{
  position:relative;
  overflow:hidden;
  background:
    radial-gradient(120% 90% at 12% 8%, var(--ink-soft), transparent 60%),
    linear-gradient(155deg, var(--ink) 0%, oklch(22% 0.10 27) 58%, oklch(18% 0.085 25) 100%);
  color:oklch(96% 0.01 27);
  padding:52px 56px;
  display:flex;
  flex-direction:column;
}
.auth-shell .aside::before{
  content:"";position:absolute;inset:0;pointer-events:none;
  background-image:
    linear-gradient(oklch(100% 0 0 / .045) 1px, transparent 1px),
    linear-gradient(90deg, oklch(100% 0 0 / .045) 1px, transparent 1px);
  background-size:46px 46px;
  mask-image:radial-gradient(120% 100% at 30% 0%, #000 35%, transparent 80%);
}
.auth-shell .aside::after{
  content:"";position:absolute;width:520px;height:520px;border-radius:50%;
  right:-160px;bottom:-200px;pointer-events:none;
  background:radial-gradient(circle, oklch(70% 0.21 30 / .35), transparent 70%);
  filter:blur(8px);
}
.auth-shell .aside > *{position:relative;z-index:1}

.auth-shell .a-brand{display:flex;align-items:center;gap:13px}
.auth-shell .a-mark{
  width:44px;height:44px;border-radius:12px;flex:none;display:grid;place-items:center;
  background:#fff;color:var(--accent);
  box-shadow:0 5px 16px oklch(10% 0.06 25 / .4);
}
.auth-shell .a-mark svg{width:25px;height:25px}
.auth-shell .a-brand .name{font-family:var(--font-display);font-weight:700;font-size:19px;letter-spacing:-.01em;line-height:1}
.auth-shell .a-brand .sub{font-size:11px;color:oklch(86% 0.04 30);letter-spacing:.05em;text-transform:uppercase;margin-top:3px;display:block}

.auth-shell .a-body{margin-top:auto;margin-bottom:auto;max-width:430px}
.auth-shell .a-eyebrow{
  display:inline-flex;align-items:center;gap:8px;font-family:var(--font-mono);
  font-size:12px;letter-spacing:.04em;color:oklch(88% 0.05 32);
  border:1px solid oklch(100% 0 0 / .18);border-radius:30px;padding:6px 13px;margin-bottom:26px;
}
.auth-shell .a-eyebrow .dot{width:7px;height:7px;border-radius:50%;background:oklch(78% 0.17 150);box-shadow:0 0 0 4px oklch(78% 0.17 150 / .25)}
.auth-shell .a-body h1{
  font-family:var(--font-display);font-weight:700;font-size:clamp(30px,3.4vw,42px);
  line-height:1.08;letter-spacing:-.025em;color:#fff;
}
.auth-shell .a-body p{margin-top:18px;font-size:15.5px;line-height:1.65;color:oklch(89% 0.02 30);max-width:40ch}

.auth-shell .a-stats{display:flex;gap:30px;margin-top:auto;padding-top:34px}
.auth-shell .a-stats .s b{font-family:var(--font-display);font-size:26px;font-weight:700;color:#fff;letter-spacing:-.01em;display:block}
.auth-shell .a-stats .s span{font-size:12px;color:oklch(83% 0.02 30);letter-spacing:.02em}

/* ============ RIGHT — formulaire ============ */
.auth-shell .auth-main{display:grid;place-items:center;padding:48px 40px}
.auth-shell .acard{width:100%;max-width:404px}

.auth-shell .m-head{margin-bottom:28px}
.auth-shell .m-head h2{font-family:var(--font-display);font-size:27px;font-weight:700;letter-spacing:-.02em}
.auth-shell .m-head p{margin-top:7px;font-size:14.5px;color:var(--muted)}
.auth-shell .m-head p a{color:var(--accent);font-weight:600;text-decoration:none}
.auth-shell .m-head p a:hover{text-decoration:underline}

.auth-shell .field{margin-bottom:16px}
.auth-shell .field label{display:block;font-size:13px;font-weight:500;color:var(--fg);margin-bottom:7px}
.auth-shell .field .wrap{position:relative}
.auth-shell .field input{
  width:100%;height:46px;border:1px solid var(--border-2);border-radius:11px;
  background:var(--bg);padding:0 14px;font-size:14.5px;color:var(--fg);transition:.16s;
}
.auth-shell .field input::placeholder{color:var(--faint)}
.auth-shell .field input:hover{border-color:var(--border)}
.auth-shell .field input:focus{outline:none;border-color:var(--accent);box-shadow:0 0 0 4px var(--accent-soft);background:var(--surface)}
.auth-shell .field.invalid input{border-color:var(--danger)}
.auth-shell .field.invalid input:focus{box-shadow:0 0 0 4px oklch(58% 0.20 25 / .12)}
.auth-shell .field .pw-toggle{
  position:absolute;right:6px;top:50%;transform:translateY(-50%);
  width:34px;height:34px;border:0;background:transparent;border-radius:8px;
  display:grid;place-items:center;color:var(--faint);transition:.15s;
}
.auth-shell .field .pw-toggle:hover{color:var(--fg);background:var(--accent-soft)}
.auth-shell .field .pw-toggle svg{width:19px;height:19px}
.auth-shell .field .err{display:none;font-size:12.5px;color:var(--danger);margin-top:6px}
.auth-shell .field.invalid .err{display:block}

.auth-shell .row-between{display:flex;align-items:center;justify-content:space-between;margin:-2px 0 20px}
.auth-shell .remember{display:flex;align-items:center;gap:8px;font-size:13.5px;color:var(--muted);cursor:pointer;user-select:none}
.auth-shell .remember input{width:16px;height:16px;accent-color:var(--accent);cursor:pointer}
.auth-shell .forgot{font-size:13.5px;color:var(--accent);font-weight:600;text-decoration:none}
.auth-shell .forgot:hover{text-decoration:underline}

.auth-shell .submit{
  width:100%;height:48px;border:0;border-radius:11px;color:#fff;font-size:15px;font-weight:600;
  background:linear-gradient(150deg,var(--accent),var(--accent-d));
  box-shadow:0 6px 18px oklch(55% 0.205 27 / .32);transition:.16s;
  display:flex;align-items:center;justify-content:center;gap:10px;
}
.auth-shell .submit:hover{transform:translateY(-1px);box-shadow:0 10px 26px oklch(55% 0.205 27 / .4)}
.auth-shell .submit:active{transform:translateY(0)}
.auth-shell .submit[disabled]{opacity:.75;cursor:default;transform:none}
.auth-shell .submit .spin{width:17px;height:17px;border:2.5px solid oklch(100% 0 0 / .4);border-top-color:#fff;border-radius:50%;animation:auth-spin .7s linear infinite;display:none}
.auth-shell .submit.loading .spin{display:block}
.auth-shell .submit.loading .label{opacity:.85}
@keyframes auth-spin{to{transform:rotate(360deg)}}

.auth-shell .form-err{font-size:13px;color:var(--danger);margin:-12px 0 16px}

.auth-shell .enterprise{
  margin-top:22px;padding-top:20px;border-top:1px solid var(--border);
  display:flex;align-items:center;justify-content:center;gap:8px;font-size:13.5px;color:var(--muted);
}
.auth-shell .enterprise a{color:var(--fg);font-weight:600;text-decoration:none;display:inline-flex;align-items:center;gap:6px}
.auth-shell .enterprise a svg{width:15px;height:15px;color:var(--accent)}
.auth-shell .enterprise a:hover{color:var(--accent)}

.auth-shell .legal{margin-top:30px;text-align:center;font-size:12px;color:var(--faint);line-height:1.6}
.auth-shell .legal a{color:var(--muted);text-decoration:underline;text-underline-offset:2px}

/* mobile brand (hidden on desktop) */
.auth-shell .m-brand{display:none;align-items:center;gap:11px;margin-bottom:30px}
.auth-shell .m-brand .a-mark{background:#fff;border:1px solid var(--border);color:var(--accent);box-shadow:0 4px 12px oklch(55% 0.205 27 / .18)}
.auth-shell .m-brand .name{font-family:var(--font-display);font-weight:700;font-size:18px;letter-spacing:-.01em;line-height:1;color:var(--fg)}
.auth-shell .m-brand .sub{font-size:11px;color:var(--muted);letter-spacing:.05em;text-transform:uppercase;margin-top:3px;display:block}

/* ============ responsive ============ */
@media (max-width:860px){
  .auth-shell{grid-template-columns:1fr}
  .auth-shell .aside{display:none}
  .auth-shell .m-brand{display:flex}
  .auth-shell .auth-main{padding:40px 24px;align-items:start;min-height:100%}
  .auth-shell .acard{margin-top:6px}
}
@media (max-width:420px){
  .auth-shell .auth-main{padding:32px 20px}
  .auth-shell .m-head h2{font-size:24px}
}
```

Differences from the blueprint, all intentional (do not "fix"):
- `.card` → `.acard`, `.main` → `.auth-main`, `spin` keyframes → `auth-spin` (collision-proofing).
- `min-height:100vh` instead of `html,body{height:100%}` (the React app can't restyle `html`).
- `--danger`/`--success`/`--accent`/`--bg`/etc. come from `tokens.css` (light values identical to the blueprint; dark theme adapts automatically).
- `.pw-toggle:hover` uses `var(--accent-soft)` instead of the literal light gray (dark-mode safe).
- Added `.form-err` (global server-error line, used by both pages) and `display:block` on `.sub` spans (the blueprint relied on default spans stacking via flex column; explicit is safer).

- [ ] **Step 3: Commit**

```bash
git add web/src/styles/auth.css
git commit -m "feat(web): port login.html blueprint styles to scoped auth.css"
```

---

### Task 3: `AuthShell` component (vitrine + live stats)

**Files:**
- Create: `web/src/components/AuthShell.tsx`
- Test: `web/src/components/AuthShell.test.tsx`

- [ ] **Step 1: Write the failing tests**

Create `web/src/components/AuthShell.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { AuthShell } from './AuthShell'
import * as api from '../api/client'
import type { Product } from '../api/types'

const product = (id: number, category: string): Product => ({
  id, name: `P${id}`, slug: `p${id}`, category, version: '1.0.0',
  contextPath: `/p${id}`, description: '', tags: [], icon: 'globe', rating: 4,
})

beforeEach(() => vi.restoreAllMocks())

describe('AuthShell', () => {
  it('renders children and the vitrine content', async () => {
    vi.spyOn(api, 'getProducts').mockResolvedValue([])
    render(<AuthShell><p>FORM HERE</p></AuthShell>)
    expect(screen.getByText('FORM HERE')).toBeInTheDocument()
    expect(screen.getByText('Vos API, un seul portail.')).toBeInTheDocument()
    expect(await screen.findByText('disponibilité')).toBeInTheDocument()
  })

  it('shows live API and category counts from the catalog', async () => {
    vi.spyOn(api, 'getProducts').mockResolvedValue([
      product(1, 'Finance'), product(2, 'Finance'), product(3, 'Engineering'),
    ])
    render(<AuthShell><p>x</p></AuthShell>)
    expect(await screen.findByText('3')).toBeInTheDocument()   // 3 APIs
    expect(await screen.findByText('2')).toBeInTheDocument()   // 2 categories
  })

  it('falls back to blueprint numbers when the catalog fetch fails', async () => {
    vi.spyOn(api, 'getProducts').mockRejectedValue(new Error('down'))
    render(<AuthShell><p>x</p></AuthShell>)
    expect(await screen.findByText('9')).toBeInTheDocument()
    expect(await screen.findByText('4')).toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd web && npx vitest run src/components/AuthShell.test.tsx`
Expected: FAIL — `Cannot find module './AuthShell'` (or equivalent).

- [ ] **Step 3: Implement `web/src/components/AuthShell.tsx`**

```tsx
import { useEffect, useState, type ReactNode } from 'react'
import { getProducts } from '../api/client'
import '../styles/auth.css'

// Blueprint fallback numbers, used when the public catalog can't be fetched.
const FALLBACK = { apis: 9, categories: 4 }

// APISIX triangle mark from the login.html blueprint (white square chrome is CSS).
function Mark() {
  return (
    <svg viewBox="0 0 48 48" aria-hidden="true">
      <path fill="currentColor" d="M24 3 L45 45 L24 45 Z" />
      <path fill="currentColor" fillOpacity=".62" d="M24 3 L24 45 L3 45 Z" />
      <path fill="#fff" d="M24 16 L31 31 L17 31 Z" />
    </svg>
  )
}

function Brand() {
  return (
    <>
      <span className="a-mark"><Mark /></span>
      <span>
        <span className="name">APISIX</span>
        <span className="sub">Portail Développeur</span>
      </span>
    </>
  )
}

export function AuthShell({ children }: { children: ReactNode }) {
  const [stats, setStats] = useState(FALLBACK)

  useEffect(() => {
    let alive = true
    getProducts({})
      .then(ps => {
        if (!alive) return
        setStats({ apis: ps.length, categories: new Set(ps.map(p => p.category)).size })
      })
      .catch(() => { /* keep blueprint fallback */ })
    return () => { alive = false }
  }, [])

  return (
    <div className="auth-shell">
      <aside className="aside">
        <div className="a-brand"><Brand /></div>
        <div className="a-body">
          <span className="a-eyebrow"><span className="dot" /> Tous les services · 100 % disponibles</span>
          <h1>Vos API, un seul portail.</h1>
          <p>Parcourez le catalogue, testez les points de terminaison et gérez vos abonnements — tout au même endroit.</p>
        </div>
        <div className="a-stats">
          <div className="s"><b>{stats.apis}</b><span>API publiées</span></div>
          <div className="s"><b>{stats.categories}</b><span>catégories</span></div>
          <div className="s"><b>99.9%</b><span>disponibilité</span></div>
        </div>
      </aside>

      <main className="auth-main">
        <div className="acard">
          <div className="m-brand"><Brand /></div>
          {children}
        </div>
      </main>
    </div>
  )
}
```

Note: the shell owns `.acard` and the mobile brand, so pages only provide the form content. The blueprint puts `.m-brand` inside the card — same placement here.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd web && npx vitest run src/components/AuthShell.test.tsx`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/components/AuthShell.tsx web/src/components/AuthShell.test.tsx
git commit -m "feat(web): AuthShell split-screen vitrine with live catalog stats"
```

---

### Task 4: Rewrite `LoginPage` to the blueprint form

**Files:**
- Modify: `web/src/pages/LoginPage.tsx` (full rewrite)
- Modify: `web/src/pages/AuthPages.test.tsx`

- [ ] **Step 1: Update + extend the login tests (failing first)**

Replace the whole content of `web/src/pages/AuthPages.test.tsx` with:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { LoginPage } from './LoginPage'
import { AuthProvider } from '../auth/AuthProvider'
import * as api from '../api/client'

beforeEach(() => {
  localStorage.clear()
  vi.restoreAllMocks()
  // AuthShell fetches catalog stats on mount; neutralize it for page tests.
  vi.spyOn(api, 'getProducts').mockResolvedValue([])
})

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
    await userEvent.click(screen.getByRole('button', { name: 'Se connecter' }))
    await waitFor(() => expect(screen.getByText('CATALOG HOME')).toBeInTheDocument())
  })

  it('shows the server error on failure', async () => {
    vi.spyOn(api, 'login').mockRejectedValue(new Error('invalid credentials'))
    renderLogin()
    await userEvent.type(screen.getByLabelText('Email'), 'a@b.c')
    await userEvent.type(screen.getByLabelText('Mot de passe'), 'wrongpass')
    await userEvent.click(screen.getByRole('button', { name: 'Se connecter' }))
    await waitFor(() => expect(screen.getByText('invalid credentials')).toBeInTheDocument())
  })

  it('toggles password visibility with the eye button', async () => {
    renderLogin()
    const pw = screen.getByLabelText('Mot de passe')
    expect(pw).toHaveAttribute('type', 'password')
    await userEvent.click(screen.getByRole('button', { name: 'Afficher le mot de passe' }))
    expect(pw).toHaveAttribute('type', 'text')
    await userEvent.click(screen.getByRole('button', { name: 'Masquer le mot de passe' }))
    expect(pw).toHaveAttribute('type', 'password')
  })

  it('disables the submit button and shows loading label while pending', async () => {
    let resolveLogin!: (v: { user: { id: number; email: string; name: string; role: string }; token: string }) => void
    vi.spyOn(api, 'login').mockImplementation(() => new Promise(res => { resolveLogin = res }))
    renderLogin()
    await userEvent.type(screen.getByLabelText('Email'), 'a@b.c')
    await userEvent.type(screen.getByLabelText('Mot de passe'), 'pw12345678')
    await userEvent.click(screen.getByRole('button', { name: 'Se connecter' }))
    const pending = await screen.findByRole('button', { name: 'Connexion…' })
    expect(pending).toBeDisabled()
    resolveLogin({ user: { id: 1, email: 'a@b.c', name: '', role: 'developer' }, token: 'jwt' })
    await waitFor(() => expect(screen.getByText('CATALOG HOME')).toBeInTheDocument())
  })

  it('renders the blueprint placeholder controls', () => {
    renderLogin()
    expect(screen.getByText('Rester connecté')).toBeInTheDocument()
    expect(screen.getByText('Mot de passe oublié ?')).toBeInTheDocument()
    expect(screen.getByText('Se connecter via votre entreprise')).toBeInTheDocument()
    expect(screen.getByText(/Conditions/)).toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run tests to verify the new ones fail**

Run: `cd web && npx vitest run src/pages/AuthPages.test.tsx`
Expected: FAIL — old markup has button "Connexion", no eye toggle, no placeholders.

- [ ] **Step 3: Rewrite `web/src/pages/LoginPage.tsx`**

Replace the whole file with:

```tsx
import { useState, type FormEvent } from 'react'
import { useNavigate, Link } from 'react-router-dom'
import { useAuth } from '../auth/AuthProvider'
import { AuthShell } from '../components/AuthShell'

export function EyeIcon({ off }: { off: boolean }) {
  return off ? (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.7}>
      <path d="M3 3l18 18" strokeLinecap="round" />
      <path d="M10.6 10.6a3 3 0 0 0 4.2 4.2M9.4 5.3A9.6 9.6 0 0 1 12 5c6.5 0 10 7 10 7a17 17 0 0 1-3.2 4M6.2 6.2A17 17 0 0 0 2 12s3.5 7 10 7a9.7 9.7 0 0 0 3.4-.6" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  ) : (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.7}>
      <path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7-10-7-10-7Z" strokeLinecap="round" strokeLinejoin="round" />
      <circle cx="12" cy="12" r="3" />
    </svg>
  )
}

export function EnterpriseRow() {
  return (
    <div className="enterprise">
      <span>Membre d'une équipe ?</span>
      <a href="#">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8}>
          <path d="M12 2 4 6v6c0 5 3.4 8.5 8 10 4.6-1.5 8-5 8-10V6l-8-4Z" strokeLinejoin="round" />
        </svg>
        Se connecter via votre entreprise
      </a>
    </div>
  )
}

export function LegalLine() {
  return (
    <p className="legal">
      En continuant, vous acceptez nos <a href="#">Conditions</a> et notre <a href="#">Politique de confidentialité</a>.
    </p>
  )
}

export function LoginPage() {
  const { login } = useAuth()
  const nav = useNavigate()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [err, setErr] = useState('')
  const [showPw, setShowPw] = useState(false)
  const [loading, setLoading] = useState(false)

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setErr('')
    setLoading(true)
    try {
      await login(email, password)
      nav('/')
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Échec de connexion')
      setLoading(false)
    }
  }

  return (
    <AuthShell>
      <form onSubmit={onSubmit} noValidate={false}>
        <div className="m-head">
          <h2>Bon retour</h2>
          <p>Pas encore de compte ? <Link to="/register">Créer un compte</Link></p>
        </div>

        {err && <p className="form-err" role="alert">{err}</p>}

        <div className="field">
          <label htmlFor="login-email">Adresse email</label>
          <div className="wrap">
            <input
              id="login-email" aria-label="Email" type="email" placeholder="vous@entreprise.com"
              autoComplete="email" required value={email} onChange={e => setEmail(e.target.value)}
            />
          </div>
        </div>

        <div className="field">
          <label htmlFor="login-pw">Mot de passe</label>
          <div className="wrap">
            <input
              id="login-pw" aria-label="Mot de passe" type={showPw ? 'text' : 'password'} placeholder="••••••••"
              autoComplete="current-password" required value={password} onChange={e => setPassword(e.target.value)}
            />
            <button
              type="button" className="pw-toggle"
              aria-label={showPw ? 'Masquer le mot de passe' : 'Afficher le mot de passe'}
              onClick={() => setShowPw(s => !s)}
            >
              <EyeIcon off={showPw} />
            </button>
          </div>
        </div>

        <div className="row-between">
          <label className="remember"><input type="checkbox" /> Rester connecté</label>
          <a className="forgot" href="#">Mot de passe oublié ?</a>
        </div>

        <button type="submit" className={`submit ${loading ? 'loading' : ''}`} disabled={loading}>
          <span className="spin" /><span className="label">{loading ? 'Connexion…' : 'Se connecter'}</span>
        </button>

        <EnterpriseRow />
        <LegalLine />
      </form>
    </AuthShell>
  )
}
```

Notes for the implementer:
- `EyeIcon`, `EnterpriseRow`, `LegalLine` are exported so `RegisterPage` (Task 5) imports them instead of duplicating SVGs. They live here because login is the primary auth page; do not create a separate file for three tiny components.
- Both `aria-label` (kept for the existing tests) and a visible `<label htmlFor>` exist; `aria-label` wins for the accessible name, which is exactly what the tests query.
- On success we navigate away without resetting `loading` — the component unmounts; resetting state after `nav()` would be a no-op warning.

- [ ] **Step 4: Run the tests**

Run: `cd web && npx vitest run src/pages/AuthPages.test.tsx src/components/AuthShell.test.tsx`
Expected: PASS (8 tests: 5 login + 3 shell).

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/LoginPage.tsx web/src/pages/AuthPages.test.tsx
git commit -m "feat(web): login page redesigned per login.html blueprint"
```

---

### Task 5: Rewrite `RegisterPage` to match

**Files:**
- Modify: `web/src/pages/RegisterPage.tsx` (full rewrite)
- Modify: `web/src/pages/AuthPages.test.tsx` (append a describe block)

- [ ] **Step 1: Append register tests (failing first)**

In `web/src/pages/AuthPages.test.tsx`, add `RegisterPage` to the imports:

```tsx
import { RegisterPage } from './RegisterPage'
```

and append this describe block at the end of the file:

```tsx
function renderRegister() {
  return render(
    <MemoryRouter initialEntries={['/register']}>
      <AuthProvider>
        <Routes>
          <Route path="/register" element={<RegisterPage />} />
          <Route path="/" element={<div>CATALOG HOME</div>} />
        </Routes>
      </AuthProvider>
    </MemoryRouter>
  )
}

describe('RegisterPage', () => {
  it('registers and navigates home on success', async () => {
    vi.spyOn(api, 'register').mockResolvedValue({ user: { id: 1, email: 'a@b.c', name: 'Ana', role: 'developer' }, token: 'jwt' })
    renderRegister()
    await userEvent.type(screen.getByLabelText('Nom'), 'Ana')
    await userEvent.type(screen.getByLabelText('Email'), 'a@b.c')
    await userEvent.type(screen.getByLabelText('Mot de passe'), 'pw12345678')
    await userEvent.click(screen.getByRole('button', { name: 'Créer le compte' }))
    await waitFor(() => expect(screen.getByText('CATALOG HOME')).toBeInTheDocument())
  })

  it('shows a field error when the password is under 8 characters', async () => {
    const spy = vi.spyOn(api, 'register')
    renderRegister()
    await userEvent.type(screen.getByLabelText('Email'), 'a@b.c')
    await userEvent.type(screen.getByLabelText('Mot de passe'), 'short')
    await userEvent.click(screen.getByRole('button', { name: 'Créer le compte' }))
    expect(await screen.findByText('Mot de passe : 8 caractères minimum')).toBeInTheDocument()
    expect(spy).not.toHaveBeenCalled()
  })

  it('shows the server error on failure', async () => {
    vi.spyOn(api, 'register').mockRejectedValue(new Error('email already used'))
    renderRegister()
    await userEvent.type(screen.getByLabelText('Email'), 'a@b.c')
    await userEvent.type(screen.getByLabelText('Mot de passe'), 'pw12345678')
    await userEvent.click(screen.getByRole('button', { name: 'Créer le compte' }))
    await waitFor(() => expect(screen.getByText('email already used')).toBeInTheDocument())
  })
})
```

- [ ] **Step 2: Run tests to verify the new ones fail**

Run: `cd web && npx vitest run src/pages/AuthPages.test.tsx`
Expected: register tests FAIL (old markup: no "Créer le compte"-with-new-classes mismatch is fine — they fail on the password field error rendering).

- [ ] **Step 3: Rewrite `web/src/pages/RegisterPage.tsx`**

Replace the whole file with:

```tsx
import { useState, type FormEvent } from 'react'
import { useNavigate, Link } from 'react-router-dom'
import { useAuth } from '../auth/AuthProvider'
import { AuthShell } from '../components/AuthShell'
import { EyeIcon, EnterpriseRow, LegalLine } from './LoginPage'

export function RegisterPage() {
  const { register } = useAuth()
  const nav = useNavigate()
  const [name, setName] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [err, setErr] = useState('')
  const [pwErr, setPwErr] = useState('')
  const [showPw, setShowPw] = useState(false)
  const [loading, setLoading] = useState(false)

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setErr('')
    setPwErr('')
    if (password.length < 8) {
      setPwErr('Mot de passe : 8 caractères minimum')
      return
    }
    setLoading(true)
    try {
      await register(email, password, name)
      nav('/')
    } catch (e) {
      setErr(e instanceof Error ? e.message : "Échec de l'inscription")
      setLoading(false)
    }
  }

  return (
    <AuthShell>
      <form onSubmit={onSubmit}>
        <div className="m-head">
          <h2>Créer un compte</h2>
          <p>Déjà inscrit ? <Link to="/login">Se connecter</Link></p>
        </div>

        {err && <p className="form-err" role="alert">{err}</p>}

        <div className="field">
          <label htmlFor="reg-name">Nom</label>
          <div className="wrap">
            <input
              id="reg-name" aria-label="Nom" placeholder="Prénom Nom"
              autoComplete="name" value={name} onChange={e => setName(e.target.value)}
            />
          </div>
        </div>

        <div className="field">
          <label htmlFor="reg-email">Adresse email</label>
          <div className="wrap">
            <input
              id="reg-email" aria-label="Email" type="email" placeholder="vous@entreprise.com"
              autoComplete="email" required value={email} onChange={e => setEmail(e.target.value)}
            />
          </div>
        </div>

        <div className={`field ${pwErr ? 'invalid' : ''}`}>
          <label htmlFor="reg-pw">Mot de passe</label>
          <div className="wrap">
            <input
              id="reg-pw" aria-label="Mot de passe" type={showPw ? 'text' : 'password'} placeholder="8 caractères minimum"
              autoComplete="new-password" required value={password}
              onChange={e => { setPassword(e.target.value); if (e.target.value.length >= 8) setPwErr('') }}
            />
            <button
              type="button" className="pw-toggle"
              aria-label={showPw ? 'Masquer le mot de passe' : 'Afficher le mot de passe'}
              onClick={() => setShowPw(s => !s)}
            >
              <EyeIcon off={showPw} />
            </button>
          </div>
          <div className="err">{pwErr}</div>
        </div>

        <button type="submit" className={`submit ${loading ? 'loading' : ''}`} disabled={loading}>
          <span className="spin" /><span className="label">{loading ? 'Création…' : 'Créer le compte'}</span>
        </button>

        <EnterpriseRow />
        <LegalLine />
      </form>
    </AuthShell>
  )
}
```

Note: no "Rester connecté"/"Mot de passe oublié ?" row here — the blueprint is a login screen; those controls only make sense on login (spec, decision 2).

- [ ] **Step 4: Run the tests**

Run: `cd web && npx vitest run src/pages/AuthPages.test.tsx`
Expected: PASS (8 tests: 5 login + 3 register).

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/RegisterPage.tsx web/src/pages/AuthPages.test.tsx
git commit -m "feat(web): register page redesigned per login.html blueprint"
```

---

### Task 6: Remove dead `.authcard` styles + full verification

**Files:**
- Modify: `web/src/styles/base.css`

- [ ] **Step 1: Confirm `.authcard` is unused**

Run: `cd web && grep -rn 'authcard' src/`
Expected: only matches inside `src/styles/base.css`. If any `.tsx` still uses it, STOP — fix that page first.

- [ ] **Step 2: Delete the `.authcard` block from `web/src/styles/base.css`**

Remove exactly these lines (keep `.autherr` — catalog/applications still use it):

```css
.authcard{max-width:380px;margin:64px auto;display:flex;flex-direction:column;gap:14px;background:var(--surface);border:1px solid var(--border);border-radius:var(--r);padding:28px;box-shadow:var(--shadow)}
.authcard h1{font-family:var(--font-display);font-size:24px}
.authcard label{display:flex;flex-direction:column;gap:6px;font-size:13px;color:var(--muted)}
.authcard input{height:40px;border:1px solid var(--border-2);border-radius:10px;background:var(--bg);padding:0 12px;font-size:14px;color:var(--fg)}
.authcard input:focus{outline:none;border-color:var(--accent);box-shadow:0 0 0 4px var(--accent-soft)}
.authcard .subbtn{justify-content:center;border:1px solid var(--accent);color:#fff;background:var(--accent);padding:10px;border-radius:10px;font-weight:600}
```

- [ ] **Step 3: Run the entire web test suite**

Run: `cd web && npx vitest run`
Expected: ALL tests pass (82 pre-existing + 6 new = 88; exact count may differ, zero failures is the requirement).

- [ ] **Step 4: Visual verification against the blueprint**

With the dev stack running (portal `:8090`, vite `:5173` — see `/tmp/portal.log` recipe):

1. Open `http://localhost:5173/login` in the browser (Playwright MCP), screenshot.
2. Compare against `/login.html` opened as `file://` — vitrine gradient, brand, eyebrow, h1, stats, field chrome, remember/forgot row, submit, enterprise row, legal line must all be present and proportioned alike.
3. Toggle dark theme (the app's theme toggle is not on auth pages; set `document.documentElement.dataset.theme='dark'` via evaluate) — form side must stay readable (tokens), vitrine unchanged.
4. Screenshot `http://localhost:5173/register` — same chrome, Nom field present, no remember/forgot row.
5. Resize to 420×800 — vitrine hidden, mobile brand visible above the form.

Expected: no invisible elements, no unstyled controls, stats show the real catalog numbers (9 / 4 unless data changed).

- [ ] **Step 5: Commit**

```bash
git add web/src/styles/base.css
git commit -m "chore(web): drop dead .authcard styles replaced by auth.css"
```

---

## Self-review notes (already applied)

- **Spec coverage:** layout/vitrine (T3), stats live+fallback (T3), login form with placeholders (T4), register (T5), CSS port with token mapping + scoping (T1, T2), `.authcard` cleanup (T6), tests incl. pw-toggle/loading/short-password (T4, T5), visual + dark + mobile verification (T6).
- **Naming consistency:** `.acard`, `.auth-main`, `auth-spin`, `.form-err` used identically in Tasks 2–5; `EyeIcon`/`EnterpriseRow`/`LegalLine` exported from LoginPage and imported by RegisterPage.
- **Known trade-off:** placeholder links use `href="#"` exactly like the blueprint (user chose pixel-fidelity); tests pin their presence so a future cleanup is deliberate.
