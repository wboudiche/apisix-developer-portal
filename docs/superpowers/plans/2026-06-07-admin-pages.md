# Admin Pages (Blueprint Port) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the three bare admin pages with a pixel-faithful port of the user's `/admin-products.html` blueprint, fully wired to the existing backend (no demo data).

**Architecture:** New scoped stylesheet `web/src/styles/admin.css` (`.adminpage` root, collision renames). Shared shell (`AdminShell` = pill sub-nav with real counts + page head) and `Composer` (collapsible create/edit card). The app-detail `Toast`/`ConfirmModal` are promoted to `web/src/components/` with their CSS moved to `web/src/styles/overlays.css`; `Toast` gains a `warn` kind. The API client gains an `ApiError` carrying the HTTP status so pages can detect 409.

**Tech Stack:** Vite + React 19 + TS, react-router v7, Vitest + RTL + jsdom. Spec: `docs/superpowers/specs/2026-06-07-admin-pages-design.md`. Blueprint: `/admin-products.html` (repo root — source of truth for all visuals/copy).

**Conventions for every task:** Run commands from `web/`. IDE/gopls diagnostics are stale — trust only command output. Copy code blocks verbatim; report any deviation with justification. One commit per task.

---

### Task 1: Tokens — `--raise` and `--c-data`

**Files:**
- Modify: `web/src/styles/tokens.css`

The blueprint uses `--raise` (raised surface, e.g. composer head background) and `--c-data` (Data category color). Neither exists in tokens.css.

- [ ] **Step 1: Add the two tokens to the LIGHT block**

In the `:root{...}` block, after the line containing `--c-eng:`, add:

```css
  --c-data: oklch(56% 0.12 230);
  --raise: oklch(99.4% 0.003 255);
```

- [ ] **Step 2: Add the dark equivalents**

In the `:root[data-theme="dark"]{...}` block, after its `--c-eng:` line (or with the other `--c-*` overrides if present; if the dark block has no `--c-*` overrides, add after `--surface:`), add:

```css
  --c-data: oklch(62% 0.11 230);
  --raise: oklch(24% 0.014 262);
```

- [ ] **Step 3: Verify the suite still passes**

Run: `npx vitest run`
Expected: 130 passed (no behavior change).

- [ ] **Step 4: Commit**

```bash
git add src/styles/tokens.css
git commit -m "feat(web): --raise and --c-data tokens for admin pages"
```

---

### Task 2: `ApiError` with HTTP status in the API client

**Files:**
- Modify: `web/src/api/client.ts`
- Test: `web/src/api/client.test.ts` (append)

Pages need to distinguish 409 (delete refused: active subscriptions) from other failures. Today `parse`/`sendAuthed` throw plain `Error` without the status.

- [ ] **Step 1: Write the failing test**

Read `web/src/api/client.test.ts` first to match its existing fetch-mocking style, then append (adapting mock helpers to the file's idiom — if it stubs `global.fetch` with `vi.stubGlobal`, do the same):

```ts
describe('ApiError', () => {
  it('carries the HTTP status on non-ok responses', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ error: 'product has active subscriptions' }), { status: 409 })))
    try {
      await adminDeleteProduct('t', 1)
      expect.unreachable('should have thrown')
    } catch (e) {
      expect(e).toBeInstanceOf(ApiError)
      expect((e as ApiError).status).toBe(409)
      expect((e as ApiError).message).toBe('product has active subscriptions')
    }
  })
})
```

Add `ApiError` and `adminDeleteProduct` to the test file's imports from `./client`.

- [ ] **Step 2: Run it to verify it fails**

Run: `npx vitest run src/api/client.test.ts`
Expected: FAIL — `ApiError` is not exported.

- [ ] **Step 3: Implement**

In `web/src/api/client.ts`, above `parse`:

```ts
// ApiError carries the HTTP status so callers can branch on it (e.g. 409 when
// deleting a product that still has active subscriptions).
export class ApiError extends Error {
  status: number
  constructor(message: string, status: number) {
    super(message)
    this.status = status
  }
}
```

In `parse`, replace `throw new Error(msg)` with `throw new ApiError(msg, res.status)`.
In `sendAuthed`, replace its `throw new Error(...)` equivalent the same way (keep the message expression as-is, wrap with the response's status).

- [ ] **Step 4: Run the full suite**

Run: `npx vitest run`
Expected: all pass (132 = 130 + 2 if the describe has 1 test; adjust count to actual — nothing may regress).

- [ ] **Step 5: Commit**

```bash
git add src/api/client.ts src/api/client.test.ts
git commit -m "feat(web): ApiError carries HTTP status"
```

---

### Task 3: Promote Toast + ConfirmModal to `components/`, styles to `overlays.css`

**Files:**
- Create: `web/src/styles/overlays.css`
- Create: `web/src/components/Toast.tsx` (moved + `kind` prop + `useToast` hook)
- Create: `web/src/components/ConfirmModal.tsx` (moved verbatim + css import)
- Move test: `web/src/pages/application/ConfirmModal.test.tsx` → `web/src/components/ConfirmModal.test.tsx`
- Test: `web/src/components/Toast.test.tsx` (new)
- Delete: `web/src/pages/application/Toast.tsx`, `web/src/pages/application/ConfirmModal.tsx`
- Modify: `web/src/styles/appdetail.css` (remove moved rules), `web/src/pages/application/{AppDetailPage,SettingsTab,CredentialsTab}.tsx`, `web/src/pages/application/{StaticTabs,CredentialsTab}.test.tsx` (import paths)

- [ ] **Step 1: Create `web/src/styles/overlays.css`**

Exact content (the toast/scrim/modal rules cut from `appdetail.css` lines ~190–212, plus the new `.warn` icon color):

```css
/* Shared overlay primitives (Toast + ConfirmModal), used by the application
   detail page and the admin pages. Class names kept from the appdetail port
   so existing tests/markup are unchanged. */

/* toast */
.appdetail-toast{position:fixed;bottom:24px;left:50%;transform:translateX(-50%) translateY(20px);background:var(--ink);color:#fff;font-size:13.5px;font-weight:500;padding:12px 18px;border-radius:11px;box-shadow:var(--shadow-h);display:flex;align-items:center;gap:9px;opacity:0;pointer-events:none;transition:.25s;z-index:90}
.appdetail-toast.show{opacity:1;transform:translateX(-50%) translateY(0)}
.appdetail-toast svg{width:17px;height:17px;color:var(--success)}
.appdetail-toast.warn svg{color:oklch(82% 0.14 78)}

/* modal */
.appdetail-scrim{position:fixed;inset:0;background:oklch(20% 0.02 262 /.45);backdrop-filter:blur(3px);display:grid;place-items:center;z-index:80;padding:20px}
.appdetail-scrim .dmodal{background:var(--surface);border-radius:18px;box-shadow:var(--shadow-h);max-width:440px;width:100%;padding:26px;animation:ad-pop .2s ease}
@keyframes ad-pop{from{opacity:0;transform:scale(.96)}to{opacity:1;transform:none}}
.appdetail-scrim .dmodal .mi{width:46px;height:46px;border-radius:12px;display:grid;place-items:center;margin-bottom:15px}
.appdetail-scrim .dmodal .mi svg{width:24px;height:24px}
.appdetail-scrim .dmodal h3{font-family:var(--font-display);font-size:19px;font-weight:700;letter-spacing:-.01em}
.appdetail-scrim .dmodal p{font-size:14px;color:var(--muted);line-height:1.55;margin-top:9px}
.appdetail-scrim .dmodal .ma{display:flex;gap:10px;margin-top:22px;justify-content:flex-end}
.appdetail-scrim .dmodal .field{margin:16px 0 0;max-width:none}
.appdetail-scrim .dmodal .field input{width:100%;border:1px solid var(--border-2);border-radius:11px;background:var(--bg);padding:11px 13px;font-size:14px;font-family:inherit;color:var(--fg)}
.appdetail-scrim .dmodal .btn{height:40px;padding:0 16px;border-radius:11px;font-size:14px;font-weight:600;display:inline-flex;align-items:center;gap:8px;border:1px solid transparent}
.appdetail-scrim .dmodal .btn.primary{background:linear-gradient(150deg,var(--accent),var(--accent-d));color:#fff}
.appdetail-scrim .dmodal .btn.ghost{background:var(--surface);border-color:var(--border-2);color:var(--fg)}
.appdetail-scrim .dmodal .btn.danger{background:var(--surface);border-color:var(--border-2);color:var(--danger)}
.appdetail-scrim .dmodal .btn.danger:hover{background:var(--danger-soft);border-color:var(--danger)}
```

- [ ] **Step 2: Remove those exact rules from `web/src/styles/appdetail.css`**

Delete the `/* toast */` block, the `/* modal */` block and the `@keyframes ad-pop` line from `appdetail.css` (everything you just copied, WITHOUT the new `.warn` line which never existed there). Nothing else in the file changes. Verify `grep -c "appdetail-toast\|appdetail-scrim\|ad-pop" src/styles/appdetail.css` → 0.

- [ ] **Step 3: Write the failing Toast test**

`web/src/components/Toast.test.tsx`:

```tsx
import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { Toast } from './Toast'

describe('Toast', () => {
  it('shows the message with the ok icon by default', () => {
    render(<Toast msg="Créé" />)
    const el = screen.getByRole('status')
    expect(el).toHaveTextContent('Créé')
    expect(el.className).toContain('show')
    expect(el.className).not.toContain('warn')
  })
  it('renders the warn variant', () => {
    render(<Toast msg="Supprimé" kind="warn" />)
    expect(screen.getByRole('status').className).toContain('warn')
  })
  it('stays hidden with a null message', () => {
    render(<Toast msg={null} />)
    expect(screen.getByRole('status').className).not.toContain('show')
  })
})
```

- [ ] **Step 4: Run it to verify it fails**

Run: `npx vitest run src/components/Toast.test.tsx`
Expected: FAIL — module `./Toast` not found.

- [ ] **Step 5: Create `web/src/components/Toast.tsx`**

```tsx
import { useCallback, useEffect, useRef, useState } from 'react'
import '../styles/overlays.css'

export type ToastKind = 'ok' | 'warn'

export function Toast({ msg, kind = 'ok' }: { msg: string | null; kind?: ToastKind }) {
  return (
    <div className={`appdetail-toast ${kind} ${msg ? 'show' : ''}`} role="status" aria-live="polite">
      {kind === 'warn' ? (
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} aria-hidden="true">
          <path d="M12 9v4M12 17h.01M10.3 3.9L2 18a2 2 0 001.7 3h16.6a2 2 0 001.7-3L13.7 3.9a2 2 0 00-3.4 0z" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
      ) : (
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2.2} aria-hidden="true">
          <path d="M20 6L9 17l-5-5" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
      )}
      <span>{msg}</span>
    </div>
  )
}

// Shared toast state: message + kind with auto-hide (blueprint: 2.6s) and
// timer cleanup on unmount.
export function useToast() {
  const [toast, setToast] = useState<{ msg: string; kind: ToastKind } | null>(null)
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined)
  useEffect(() => () => clearTimeout(timer.current), [])
  const notify = useCallback((msg: string, kind: ToastKind = 'ok') => {
    setToast({ msg, kind })
    clearTimeout(timer.current)
    timer.current = setTimeout(() => setToast(null), 2600)
  }, [])
  return { toast, notify }
}
```

- [ ] **Step 6: Move ConfirmModal**

`git mv src/pages/application/ConfirmModal.tsx src/components/ConfirmModal.tsx`, then add as its FIRST line:

```tsx
import '../styles/overlays.css'
```

(The rest of the file is unchanged — keep the existing `useEffect`/`useRef` import line after it.)

`git mv src/pages/application/ConfirmModal.test.tsx src/components/ConfirmModal.test.tsx` — its relative import `./ConfirmModal` still resolves; no edit needed.

Delete the old Toast: `git rm src/pages/application/Toast.tsx`.

- [ ] **Step 7: Update import paths in the application page**

- `src/pages/application/AppDetailPage.tsx`:
  - `import { ConfirmModal, type ModalSpec } from '../../components/ConfirmModal'`
  - `import { Toast } from '../../components/Toast'`
- `src/pages/application/SettingsTab.tsx`: `import type { ModalSpec } from '../../components/ConfirmModal'`
- `src/pages/application/CredentialsTab.tsx`: `import type { ModalSpec } from '../../components/ConfirmModal'`
- `src/pages/application/StaticTabs.test.tsx`: `import type { ModalSpec } from '../../components/ConfirmModal'`
- `src/pages/application/CredentialsTab.test.tsx`: `import type { ModalSpec } from '../../components/ConfirmModal'`

Verify nothing else references the old paths: `grep -rn "application/Toast\|application/ConfirmModal\|from './Toast'\|from './ConfirmModal'" src/` → only `src/components/ConfirmModal.test.tsx` (its own `./ConfirmModal`).

- [ ] **Step 8: Run the full suite + typecheck**

Run: `npx vitest run` — expected: previous count + 3 (new Toast tests), all pass.
Run: `npx tsc -b --force` — expected: clean.

- [ ] **Step 9: Commit**

```bash
git add -A src/components src/pages/application src/styles
git commit -m "refactor(web): promote Toast/ConfirmModal to components, overlay styles to overlays.css"
```

---

### Task 4: `admin.css` — scoped port of the blueprint styles

**Files:**
- Create: `web/src/styles/admin.css`

Renames applied (collision grep against catalog.css/base.css): `.ctx`→`.actx`, `.sep`→`.asep`, `.cat`→`.acat`, `.pill`→`.apill`, `.empty`→`.aempty`, `.ico`→`.aico`. The blueprint's `.toast`/`.scrim`/`.modal` are NOT ported (Toast/ConfirmModal from Task 3 are used instead). The blueprint's topbar styles are NOT ported (existing TopBar). `.subnav` items are `<a>` (router Links), not `<button>`. Light-gray literals become `color-mix()`/tokens so dark mode works. `@keyframes fade` renamed `adm-fade` (catalog owns `fade`).

- [ ] **Step 1: Create the file with EXACTLY this content**

```css
/* Admin pages — port of /admin-products.html blueprint, scoped under
   .adminpage. Renames (collision proofing vs catalog.css/base.css):
   .ctx→.actx .sep→.asep .cat→.acat .pill→.apill .empty→.aempty .ico→.aico
   keyframes: fade→adm-fade. Blueprint toast/scrim/modal are NOT ported —
   the shared Toast/ConfirmModal components (overlays.css) are used. */

.adminpage{max-width:1200px;margin:0 auto;padding:0 32px 60px}

/* sub-nav (pill tabs) */
.adminpage .subnav{display:flex;align-items:center;gap:4px;padding-top:22px}
.adminpage .subnav a{
  padding:9px 16px;border:0;background:transparent;border-radius:10px;
  font-size:14px;font-weight:500;color:var(--muted);transition:.15s;display:flex;align-items:center;gap:8px;
}
.adminpage .subnav a .ct{font-family:var(--font-mono);font-size:11px;font-weight:600;color:var(--faint);background:var(--surface);border:1px solid var(--border);border-radius:20px;padding:1px 7px;transition:.15s}
.adminpage .subnav a:hover{background:color-mix(in oklab, var(--fg) 4%, transparent);color:var(--fg)}
.adminpage .subnav a.active{background:var(--accent-soft);color:var(--accent);font-weight:600}
.adminpage .subnav a.active .ct{color:var(--accent);border-color:oklch(80% 0.08 27);background:var(--surface)}

/* page head */
.adminpage .phead{display:flex;align-items:flex-end;justify-content:space-between;gap:20px;margin:26px 0 18px}
.adminpage .phead h1{font-family:var(--font-display);font-size:30px;font-weight:700;letter-spacing:-.025em;line-height:1.05}
.adminpage .phead p{font-size:14px;color:var(--muted);margin-top:6px;max-width:62ch}
.adminpage .phead code{font-family:var(--font-mono);font-size:12.5px;color:var(--accent)}

/* buttons */
.adminpage .btn{display:inline-flex;align-items:center;gap:8px;height:42px;padding:0 18px;border-radius:11px;font-size:14px;font-weight:600;border:1px solid transparent;transition:.16s;white-space:nowrap}
.adminpage .btn svg{width:17px;height:17px}
.adminpage .btn-primary{color:#fff;background:linear-gradient(150deg,var(--accent),var(--accent-d));box-shadow:0 5px 16px oklch(55% 0.205 27 / .28)}
.adminpage .btn-primary:hover{transform:translateY(-1px);box-shadow:0 9px 22px oklch(55% 0.205 27 / .36)}
.adminpage .btn-primary:active{transform:translateY(0)}
.adminpage .btn-ghost{background:var(--surface);border-color:var(--border-2);color:var(--fg)}
.adminpage .btn-ghost:hover{border-color:var(--fg)}
.adminpage .btn-sm{height:34px;padding:0 13px;font-size:13px;border-radius:9px;font-weight:500}

/* composer (collapsible create/edit card) */
.adminpage .composer{background:var(--surface);border:1px solid var(--border);border-radius:var(--r);box-shadow:var(--shadow);overflow:hidden;margin-bottom:26px}
.adminpage .composer-head{display:flex;align-items:center;gap:12px;padding:16px 22px;border-bottom:1px solid var(--border);background:var(--raise)}
.adminpage .composer-head .dot{width:9px;height:9px;border-radius:50%;background:var(--accent);box-shadow:0 0 0 4px var(--accent-soft)}
.adminpage .composer-head h2{font-family:var(--font-display);font-size:15.5px;font-weight:600;letter-spacing:-.01em}
.adminpage .composer-head .hint{font-size:12.5px;color:var(--faint);margin-left:auto}
.adminpage .composer-body{padding:22px}
.adminpage .grid2{display:grid;grid-template-columns:1fr 1fr;gap:18px 22px}
.adminpage .grid2.plans{grid-template-columns:1.4fr 1fr 1fr}
.adminpage .field label{display:block;font-size:12.5px;font-weight:600;color:var(--fg);margin-bottom:7px}
.adminpage .field label .opt{font-weight:400;color:var(--faint);margin-left:5px}
.adminpage .field .ipt{
  width:100%;height:44px;border:1px solid var(--border-2);border-radius:10px;background:var(--bg);
  padding:0 13px;font-size:14px;color:var(--fg);transition:.15s;font-family:inherit;
}
.adminpage .field .ipt.mono{font-family:var(--font-mono);font-size:13px}
.adminpage .field .ipt::placeholder{color:var(--faint)}
.adminpage .field .ipt:hover{border-color:var(--border)}
.adminpage .field .ipt:focus{outline:none;border-color:var(--accent);box-shadow:0 0 0 4px var(--accent-soft);background:var(--surface)}
.adminpage .field .help{font-size:11.5px;color:var(--faint);margin-top:6px;font-family:var(--font-mono)}
.adminpage .composer-foot{display:flex;align-items:center;gap:18px;margin-top:20px;padding-top:18px;border-top:1px dashed var(--border)}
.adminpage .composer-foot .foot-acts{margin-left:auto;display:flex;gap:10px}
.adminpage .composer-foot .preview{font-size:12.5px;color:var(--faint);font-family:var(--font-mono)}
.adminpage .switch{display:flex;align-items:center;gap:9px;font-size:13.5px;font-weight:500;color:var(--fg);cursor:pointer;user-select:none}
.adminpage .switch input{appearance:none;width:38px;height:22px;border-radius:20px;background:var(--border-2);position:relative;transition:.18s;cursor:pointer;flex:none}
.adminpage .switch input::after{content:"";position:absolute;top:2px;left:2px;width:18px;height:18px;border-radius:50%;background:#fff;box-shadow:0 1px 3px rgba(0,0,0,.2);transition:.18s}
.adminpage .switch input:checked{background:var(--success)}
.adminpage .switch input:checked::after{transform:translateX(16px)}

/* list head + filter */
.adminpage .list-head{display:flex;align-items:center;gap:14px;margin-bottom:6px}
.adminpage .list-head h3{font-family:var(--font-display);font-size:15px;font-weight:600;color:var(--muted)}
.adminpage .list-filter{margin-left:auto;position:relative;display:flex;align-items:center}
.adminpage .list-filter svg{position:absolute;left:11px;width:15px;height:15px;color:var(--faint)}
.adminpage .list-filter input{height:36px;width:230px;border:1px solid var(--border-2);border-radius:9px;background:var(--surface);padding:0 12px 0 33px;font-size:13px;transition:.15s;color:var(--fg);font-family:inherit}
.adminpage .list-filter input:focus{outline:none;border-color:var(--accent);box-shadow:0 0 0 3px var(--accent-soft)}

/* rows */
.adminpage .rows{background:var(--surface);border:1px solid var(--border);border-radius:var(--r);box-shadow:var(--shadow);overflow:hidden}
.adminpage .row{display:flex;align-items:center;gap:16px;padding:15px 20px;border-bottom:1px solid var(--border);transition:background .14s}
.adminpage .row:last-child{border-bottom:0}
.adminpage .row:hover{background:var(--raise)}
.adminpage .row .swatch{width:38px;height:38px;flex:none;border-radius:10px;display:grid;place-items:center;color:#fff}
.adminpage .row .swatch svg{width:19px;height:19px}
.adminpage .row .main{min-width:0;flex:1}
.adminpage .row .main .nm{display:flex;align-items:center;gap:9px;flex-wrap:wrap}
.adminpage .row .main .nm b{font-family:var(--font-display);font-size:15px;font-weight:600;letter-spacing:-.01em}
.adminpage .row .main .nm .actx{font-family:var(--font-mono);font-size:12.5px;color:var(--accent);background:var(--accent-soft);padding:1px 8px;border-radius:6px}
.adminpage .row .main .meta{display:flex;align-items:center;gap:7px;margin-top:5px;font-size:12.5px;color:var(--muted);flex-wrap:wrap}
.adminpage .row .main .meta .up{font-family:var(--font-mono);font-size:12px;color:var(--faint)}
.adminpage .row .main .meta .asep{color:var(--border-2)}
.adminpage .row .main .meta .acat{font-weight:500}
.adminpage .apill{display:inline-flex;align-items:center;gap:6px;font-size:11.5px;font-weight:600;padding:3px 10px;border-radius:20px;letter-spacing:.01em}
.adminpage .apill .pdot{width:6px;height:6px;border-radius:50%}
.adminpage .apill.pub{color:var(--success);background:var(--success-soft)}
.adminpage .apill.pub .pdot{background:var(--success)}
.adminpage .apill.draft{color:var(--warn);background:var(--warn-soft)}
.adminpage .apill.draft .pdot{background:var(--warn)}
.adminpage .row .ver-col{font-family:var(--font-mono);font-size:12.5px;color:var(--muted);width:64px;text-align:right;flex:none}
.adminpage .row .actions{display:flex;align-items:center;gap:7px;flex:none;opacity:.55;transition:.15s}
.adminpage .row:hover .actions{opacity:1}
.adminpage .iact{width:34px;height:34px;border:1px solid var(--border-2);background:var(--surface);border-radius:9px;display:grid;place-items:center;color:var(--muted);transition:.15s}
.adminpage .iact svg{width:16px;height:16px}
.adminpage .iact:hover{color:var(--fg);border-color:var(--fg)}
.adminpage .iact.del:hover{color:var(--danger);border-color:var(--danger);background:var(--danger-soft)}

/* plan rows */
.adminpage .row.plan .swatch{background:linear-gradient(150deg,var(--accent),var(--accent-d))}
.adminpage .row .limit{font-family:var(--font-mono);font-size:13px;color:var(--fg);background:color-mix(in oklab, var(--fg) 5%, transparent);border:1px solid var(--border);padding:3px 10px;border-radius:8px}
.adminpage .tier-Free .swatch{background:linear-gradient(150deg,oklch(60% 0.02 262),oklch(50% 0.02 262))}
.adminpage .tier-Silver .swatch{background:linear-gradient(150deg,oklch(66% 0.03 250),oklch(54% 0.04 250))}
.adminpage .tier-Gold .swatch{background:linear-gradient(150deg,oklch(72% 0.12 78),oklch(60% 0.13 62))}

/* subscriptions (approval queue) */
.adminpage .sub-card{display:flex;align-items:center;gap:16px;padding:16px 20px;border-bottom:1px solid var(--border)}
.adminpage .sub-card:last-child{border-bottom:0}
.adminpage .sub-card .app-av{width:40px;height:40px;flex:none;border-radius:11px;display:grid;place-items:center;font-family:var(--font-display);font-weight:700;font-size:15px;color:#fff;background:linear-gradient(150deg,var(--c-eng),oklch(46% 0.12 288))}
.adminpage .sub-card .sub-main{flex:1;min-width:0}
.adminpage .sub-card .sub-main .ttl{font-size:14px;font-weight:500;color:var(--fg)}
.adminpage .sub-card .sub-main .ttl b{font-weight:600}
.adminpage .sub-card .sub-main .ttl .arr{color:var(--faint);margin:0 6px}
.adminpage .sub-card .sub-main .sub-meta{font-size:12.5px;color:var(--muted);margin-top:4px;display:flex;gap:8px;align-items:center;flex-wrap:wrap}
.adminpage .sub-meta .who2{font-weight:500;color:var(--fg)}
.adminpage .sub-meta time{font-family:var(--font-mono);font-size:12px;color:var(--faint)}
.adminpage .plan-tag{font-size:11.5px;font-weight:600;padding:3px 10px;border-radius:20px;color:oklch(45% 0.02 262);background:color-mix(in oklab, var(--fg) 6%, transparent)}
.adminpage .plan-tag.Silver{color:oklch(42% 0.04 250);background:oklch(93% 0.02 250)}
.adminpage .plan-tag.Gold{color:oklch(48% 0.13 62);background:var(--warn-soft)}
.adminpage .sub-card .sub-acts{display:flex;gap:8px;flex:none}
.adminpage .btn-approve{height:36px;padding:0 14px;border-radius:9px;font-size:13px;font-weight:600;color:#fff;background:var(--success);border:0}
.adminpage .btn-approve:hover{filter:brightness(1.06)}
.adminpage .btn-reject{height:36px;padding:0 14px;border-radius:9px;font-size:13px;font-weight:500;color:var(--danger);background:var(--surface);border:1px solid var(--border-2)}
.adminpage .btn-reject:hover{border-color:var(--danger);background:var(--danger-soft)}

/* empty state */
.adminpage .aempty{text-align:center;padding:64px 20px;color:var(--muted)}
.adminpage .aempty .aico{width:54px;height:54px;margin:0 auto 16px;border-radius:14px;display:grid;place-items:center;background:var(--success-soft);color:var(--success)}
.adminpage .aempty .aico svg{width:26px;height:26px}
.adminpage .aempty h4{font-family:var(--font-display);font-size:17px;font-weight:600;color:var(--fg);margin-bottom:5px}
.adminpage .aempty p{font-size:13.5px}

/* panel entrance */
.adminpage .apanel{animation:adm-fade .25s ease}
@keyframes adm-fade{from{opacity:0;transform:translateY(6px)}to{opacity:1;transform:none}}

/* dark-mode readability for blueprint literals */
:root[data-theme="dark"] .adminpage .apill.pub{color:oklch(80% 0.1 155)}
:root[data-theme="dark"] .adminpage .apill.draft{color:oklch(85% 0.1 85)}
:root[data-theme="dark"] .adminpage .plan-tag{color:oklch(80% 0.02 262)}
:root[data-theme="dark"] .adminpage .plan-tag.Silver{color:oklch(82% 0.03 250);background:oklch(34% 0.03 250)}
:root[data-theme="dark"] .adminpage .plan-tag.Gold{color:oklch(85% 0.1 78)}
:root[data-theme="dark"] .adminpage .subnav a.active .ct{border-color:oklch(45% 0.09 27)}

/* responsive (blueprint, minus topbar rules) */
@media (max-width:880px){
  .adminpage .grid2{grid-template-columns:1fr}
  .adminpage .grid2.plans{grid-template-columns:1fr}
  .adminpage .row .ver-col{display:none}
}
@media (max-width:560px){
  .adminpage{padding:0 16px 60px}
  .adminpage .row .main .meta .up{display:none}
  .adminpage .list-filter input{width:150px}
  .adminpage .phead{flex-direction:column;align-items:flex-start}
}
```

- [ ] **Step 2: Verify scoping**

Run: `grep -v -E "^(/\*|\s|\}|@|$)" src/styles/admin.css | grep -v -E "^(\.adminpage|:root\[data-theme)" | head`
Expected: empty output (every rule rooted at `.adminpage` or the dark override).

- [ ] **Step 3: Run suite (no regression — file not imported yet)**

Run: `npx vitest run` — all pass.

- [ ] **Step 4: Commit**

```bash
git add src/styles/admin.css
git commit -m "feat(web): scoped admin.css ported from admin-products.html blueprint"
```

---

### Task 5: Admin meta helpers (`slugify`, category meta, plan rates)

**Files:**
- Create: `web/src/pages/admin/meta.tsx`
- Test: `web/src/pages/admin/meta.test.tsx`

- [ ] **Step 1: Write the failing tests**

`web/src/pages/admin/meta.test.tsx`:

```tsx
import { describe, it, expect } from 'vitest'
import { slugify, catMeta, CAT_META, planRate, planPreview } from './meta'

describe('slugify', () => {
  it('matches the blueprint behavior', () => {
    expect(slugify('CurrencyConverterAPI')).toBe('currencyconverter')
    expect(slugify('  Phone Verification ')).toBe('phone-verification')
    expect(slugify('My-API')).toBe('my')
  })
})

describe('catMeta', () => {
  it('returns the blueprint meta for known categories', () => {
    expect(catMeta('Finance')).toBe(CAT_META.Finance)
    expect(catMeta('Data').color).toBe('var(--c-data)')
  })
  it('falls back deterministically for unknown categories', () => {
    const a = catMeta('Logistique')
    expect(a).toEqual(catMeta('Logistique')) // stable
    expect(a.color).toMatch(/^var\(--c-/)
  })
})

describe('plan rates', () => {
  it('formats sustained rates like the blueprint rows', () => {
    expect(planRate(60, 60)).toBe('≈ 1 req/s soutenu')
    expect(planRate(30, 60)).toBe('≈ 0.50 req/s soutenu')
  })
  it('formats the composer preview', () => {
    expect(planPreview(100, 60)).toBe('≈ 1.7 req/s soutenu')
    expect(planPreview(30, 60)).toBe('≈ 0.50 req/s soutenu')
  })
})
```

- [ ] **Step 2: Run to verify failure**

Run: `npx vitest run src/pages/admin/meta.test.tsx`
Expected: FAIL — module not found.

- [ ] **Step 3: Create `web/src/pages/admin/meta.tsx`**

```tsx
import type { ReactNode } from 'react'

// Blueprint slugify: lowercase, strip a trailing "api", non-alphanumerics → "-".
export const slugify = (s: string) =>
  s.toLowerCase().trim().replace(/api$/, '').replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '')

// Sustained-rate labels (blueprint: rows use 0 decimals ≥1, preview uses 1).
export const planRate = (limit: number, windowS: number) => {
  const r = limit / (windowS || 1)
  return `≈ ${r >= 1 ? r.toFixed(0) : r.toFixed(2)} req/s soutenu`
}
export const planPreview = (limit: number, windowS: number) => {
  const r = limit / (windowS || 1)
  return `≈ ${r >= 1 ? r.toFixed(1) : r.toFixed(2)} req/s soutenu`
}

export interface CatMeta { color: string; icon: ReactNode }

// Category swatch icons (blueprint CAT_META paths, verbatim).
export const CAT_META: Record<string, CatMeta> = {
  Finance: { color: 'var(--c-finance)', icon: <path d="M12 2v20M17 5H9.5a3.5 3.5 0 000 7h5a3.5 3.5 0 010 7H6" /> },
  Marketing: { color: 'var(--c-marketing)', icon: <path d="M3 11l18-5v12L3 14v-3zM3 11v3M11.6 16.8a3 3 0 01-5.8-1" /> },
  Engineering: { color: 'var(--c-eng)', icon: <path d="M8 9l-4 3 4 3M16 9l4 3-4 3M13 6l-2 12" /> },
  Data: { color: 'var(--c-data)', icon: <><ellipse cx="12" cy="5" rx="8" ry="3" /><path d="M4 5v6c0 1.7 3.6 3 8 3s8-1.3 8-3V5M4 11v6c0 1.7 3.6 3 8 3s8-1.3 8-3v-6" /></> },
  Administration: { color: 'var(--c-admin)', icon: <><circle cx="12" cy="8" r="3.2" /><path d="M5.5 20a6.5 6.5 0 0113 0" /></> },
}

const FALLBACK_COLORS = ['var(--c-finance)', 'var(--c-marketing)', 'var(--c-eng)', 'var(--c-data)', 'var(--c-admin)']

// Known categories use the blueprint meta; anything else gets a deterministic
// color (spec) with the Administration glyph.
export function catMeta(category: string): CatMeta {
  const m = CAT_META[category]
  if (m) return m
  let h = 0
  for (const ch of category) h = (h * 31 + ch.charCodeAt(0)) >>> 0
  return { color: FALLBACK_COLORS[h % FALLBACK_COLORS.length], icon: CAT_META.Administration.icon }
}
```

- [ ] **Step 4: Run the tests**

Run: `npx vitest run src/pages/admin/meta.test.tsx` — all pass. Then `npx tsc -b --force` — clean.

- [ ] **Step 5: Commit**

```bash
git add src/pages/admin/meta.tsx src/pages/admin/meta.test.tsx
git commit -m "feat(web): admin meta helpers — slugify, category swatches, plan rates"
```

---

### Task 6: `AdminShell` (pill nav with real counts + page head)

**Files:**
- Create: `web/src/pages/admin/AdminShell.tsx`
- Test: `web/src/pages/admin/AdminShell.test.tsx`

- [ ] **Step 1: Write the failing tests**

`web/src/pages/admin/AdminShell.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { AdminShell } from './AdminShell'
import { AuthProvider } from '../../auth/AuthProvider'
import * as api from '../../api/client'

beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('token', 'jwt')
  localStorage.setItem('user', JSON.stringify({ id: 1, email: 'a@b.c', name: 'Admin', role: 'admin' }))
  vi.restoreAllMocks()
  vi.spyOn(api, 'adminGetProducts').mockResolvedValue([{ name: 'P', slug: 'p', category: '', version: '', contextPath: '/p', description: '', tags: [], icon: '', upstreamUrl: '', published: true }])
  vi.spyOn(api, 'adminGetPlans').mockResolvedValue([])
  vi.spyOn(api, 'adminGetSubscriptions').mockResolvedValue([
    { id: 1, applicationName: 'A', ownerEmail: 'a@b.c', productName: 'P', version: '1', planName: 'Free', status: 'pending', createdAt: '2026-06-06T00:00:00Z' },
    { id: 2, applicationName: 'B', ownerEmail: 'b@b.c', productName: 'P', version: '1', planName: 'Free', status: 'pending', createdAt: '2026-06-06T00:00:00Z' },
  ])
})

function renderShell(counts?: { products?: number; plans?: number; pending?: number }) {
  return render(
    <MemoryRouter>
      <AuthProvider>
        <AdminShell active="products" title="Produits" description="desc" counts={counts}>
          <p>CHILD</p>
        </AdminShell>
      </AuthProvider>
    </MemoryRouter>
  )
}

describe('AdminShell', () => {
  it('renders nav, head and children with fetched counts', async () => {
    renderShell()
    expect(screen.getByText('CHILD')).toBeInTheDocument()
    expect(screen.getByRole('heading', { level: 1, name: 'Produits' })).toBeInTheDocument()
    const prodLink = await screen.findByRole('link', { name: /Produits/ })
    expect(prodLink).toHaveClass('active')
    expect(await screen.findByText('2')).toBeInTheDocument()   // pending badge
  })
  it('a provided count overrides fetching for that tab', async () => {
    renderShell({ products: 9 })
    expect(await screen.findByText('9')).toBeInTheDocument()
    expect(api.adminGetProducts).not.toHaveBeenCalled()
  })
})
```

- [ ] **Step 2: Run to verify failure**

Run: `npx vitest run src/pages/admin/AdminShell.test.tsx` — FAIL (module not found).

- [ ] **Step 3: Create `web/src/pages/admin/AdminShell.tsx`**

```tsx
import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { TopBar } from '../../components/TopBar'
import { useAuth } from '../../auth/AuthProvider'
import { adminGetProducts, adminGetPlans, adminGetSubscriptions } from '../../api/client'
import '../../styles/admin.css'

export type AdminTab = 'products' | 'plans' | 'approvals'

// Pill sub-nav with real count badges + blueprint page head. A page passes the
// count it already knows (its own list length) via `counts`; the shell fetches
// the others once on mount.
export function AdminShell({ active, title, description, action, counts, children }: {
  active: AdminTab
  title: string
  description: ReactNode
  action?: ReactNode
  counts?: { products?: number; plans?: number; pending?: number }
  children: ReactNode
}) {
  const { token } = useAuth()
  const [fetched, setFetched] = useState<{ products?: number; plans?: number; pending?: number }>({})
  // Which keys the page provides is fixed per call site — capture at mount.
  const provided = useRef({
    products: counts?.products !== undefined,
    plans: counts?.plans !== undefined,
    pending: counts?.pending !== undefined,
  })

  useEffect(() => {
    if (!token) return
    let alive = true
    if (!provided.current.products)
      adminGetProducts(token).then(l => { if (alive) setFetched(f => ({ ...f, products: l.length })) }).catch(() => {})
    if (!provided.current.plans)
      adminGetPlans(token).then(l => { if (alive) setFetched(f => ({ ...f, plans: l.length })) }).catch(() => {})
    if (!provided.current.pending)
      adminGetSubscriptions(token, 'pending').then(l => { if (alive) setFetched(f => ({ ...f, pending: l.length })) }).catch(() => {})
    return () => { alive = false }
  }, [token])

  const n = {
    products: counts?.products ?? fetched.products,
    plans: counts?.plans ?? fetched.plans,
    pending: counts?.pending ?? fetched.pending,
  }
  const badge = (v?: number) => v === undefined ? null : <span className="ct">{v}</span>

  return (
    <>
      <TopBar search="" onSearch={() => {}} />
      <div className="adminpage">
        <nav className="subnav">
          <Link className={active === 'products' ? 'active' : ''} to="/admin/products">Produits {badge(n.products)}</Link>
          <Link className={active === 'plans' ? 'active' : ''} to="/admin/plans">Plans {badge(n.plans)}</Link>
          <Link className={active === 'approvals' ? 'active' : ''} to="/admin/approvals">Abonnements {badge(n.pending)}</Link>
        </nav>
        <div className="apanel">
          <div className="phead">
            <div>
              <h1>{title}</h1>
              <p>{description}</p>
            </div>
            {action}
          </div>
          {children}
        </div>
      </div>
    </>
  )
}
```

- [ ] **Step 4: Run the tests**

Run: `npx vitest run src/pages/admin/AdminShell.test.tsx` — 2 pass. Full `npx vitest run` — all pass; `npx tsc -b --force` — clean.

- [ ] **Step 5: Commit**

```bash
git add src/pages/admin/AdminShell.tsx src/pages/admin/AdminShell.test.tsx
git commit -m "feat(web): AdminShell — pill nav with real counts, blueprint page head"
```

---

### Task 7: `Composer` (collapsible create/edit card)

**Files:**
- Create: `web/src/pages/admin/Composer.tsx`

Structural component; behavior is exercised through the page tests of Tasks 8–9 (no standalone suite — it has no logic beyond rendering).

- [ ] **Step 1: Create `web/src/pages/admin/Composer.tsx`**

```tsx
import type { FormEvent, ReactNode } from 'react'

// Blueprint .composer: header (dot + title + hint), body (page-provided field
// grid), dashed-top foot (left slot + Annuler/submit). Unmounted when closed,
// so autoFocus on the page's first field fires on every open.
export function Composer({ open, title, hint, submitLabel, onSubmit, onCancel, footLeft, children }: {
  open: boolean
  title: string
  hint: string
  submitLabel: string
  onSubmit: () => void
  onCancel: () => void
  footLeft?: ReactNode
  children: ReactNode
}) {
  if (!open) return null
  function submit(e: FormEvent) { e.preventDefault(); onSubmit() }
  return (
    <form className="composer" onSubmit={submit}>
      <div className="composer-head">
        <span className="dot" />
        <h2>{title}</h2>
        <span className="hint">{hint}</span>
      </div>
      <div className="composer-body">
        {children}
        <div className="composer-foot">
          {footLeft}
          <div className="foot-acts">
            <button type="button" className="btn btn-ghost btn-sm" onClick={onCancel}>Annuler</button>
            <button type="submit" className="btn btn-primary btn-sm">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} aria-hidden="true"><path d="M20 6L9 17l-5-5" strokeLinecap="round" strokeLinejoin="round" /></svg>
              {submitLabel}
            </button>
          </div>
        </div>
      </div>
    </form>
  )
}
```

- [ ] **Step 2: Typecheck and commit**

Run: `npx tsc -b --force` — clean.

```bash
git add src/pages/admin/Composer.tsx
git commit -m "feat(web): admin Composer card"
```

---

### Task 8: Products page

**Files:**
- Create: `web/src/pages/admin/ProductsPage.tsx`
- Test: `web/src/pages/admin/ProductsPage.test.tsx`

- [ ] **Step 1: Write the failing tests**

`web/src/pages/admin/ProductsPage.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { ProductsPage } from './ProductsPage'
import { AuthProvider } from '../../auth/AuthProvider'
import * as api from '../../api/client'
import { ApiError } from '../../api/client'
import type { AdminProduct } from '../../api/types'

const products: AdminProduct[] = [
  { id: 1, name: 'CurrencyConverterAPI', slug: 'currency-converter', category: 'Finance', version: '1.0.0', contextPath: '/currencyconv', description: '', tags: [], icon: '', upstreamUrl: 'echo:8080', published: true },
  { id: 2, name: 'PizzaShackAPI', slug: 'pizzashack', category: 'Data', version: '0.9.0', contextPath: '/pizzashack', description: '', tags: [], icon: '', upstreamUrl: 'echo:8080', published: false },
]

beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('token', 'jwt')
  localStorage.setItem('user', JSON.stringify({ id: 1, email: 'a@b.c', name: 'Admin', role: 'admin' }))
  vi.restoreAllMocks()
  vi.spyOn(api, 'adminGetProducts').mockResolvedValue(products)
  vi.spyOn(api, 'adminGetPlans').mockResolvedValue([])
  vi.spyOn(api, 'adminGetSubscriptions').mockResolvedValue([])
})

function renderPage() {
  return render(
    <MemoryRouter><AuthProvider><ProductsPage /></AuthProvider></MemoryRouter>
  )
}

describe('ProductsPage', () => {
  it('renders rows with context chip, upstream and status pill', async () => {
    renderPage()
    expect(await screen.findByText('CurrencyConverterAPI')).toBeInTheDocument()
    expect(screen.getByText('/currencyconv')).toBeInTheDocument()
    expect(screen.getByText('echo:8080', { selector: '.up' })).toBeInTheDocument()
    expect(screen.getByText('publié')).toBeInTheDocument()
    expect(screen.getByText('brouillon')).toBeInTheDocument()
    expect(screen.getByText('v1.0.0')).toBeInTheDocument()
  })

  it('filters rows client-side', async () => {
    renderPage()
    await screen.findByText('CurrencyConverterAPI')
    await userEvent.type(screen.getByPlaceholderText('Filtrer les produits…'), 'pizza')
    expect(screen.queryByText('CurrencyConverterAPI')).not.toBeInTheDocument()
    expect(screen.getByText('PizzaShackAPI')).toBeInTheDocument()
  })

  it('creates a product with an auto-generated slug', async () => {
    const create = vi.spyOn(api, 'adminCreateProduct').mockResolvedValue(products[0])
    renderPage()
    await screen.findByText('CurrencyConverterAPI')
    await userEvent.click(screen.getByRole('button', { name: /Nouveau produit/ }))
    await userEvent.type(screen.getByLabelText('Nom'), 'OrdersAPI')
    expect(screen.getByLabelText('Slug')).toHaveValue('orders')
    await userEvent.click(screen.getByRole('button', { name: /Créer le produit/ }))
    await waitFor(() => expect(create).toHaveBeenCalled())
    const payload = create.mock.calls[0][1]
    expect(payload.name).toBe('OrdersAPI')
    expect(payload.slug).toBe('orders')
    expect(payload.contextPath).toBe('/orders')
    expect(payload.published).toBe(true)
  })

  it('edit opens the composer prefilled and saves with PUT', async () => {
    const update = vi.spyOn(api, 'adminUpdateProduct').mockResolvedValue(products[0])
    renderPage()
    await screen.findByText('CurrencyConverterAPI')
    await userEvent.click(screen.getAllByRole('button', { name: 'Modifier' })[0])
    expect(screen.getByLabelText('Nom')).toHaveValue('CurrencyConverterAPI')
    await userEvent.clear(screen.getByLabelText('Upstream'))
    await userEvent.type(screen.getByLabelText('Upstream'), 'fx:9000')
    await userEvent.click(screen.getByRole('button', { name: /Enregistrer/ }))
    await waitFor(() => expect(update).toHaveBeenCalled())
    expect(update.mock.calls[0][1]).toBe(1)
    expect(update.mock.calls[0][2].upstreamUrl).toBe('fx:9000')
  })

  it('the eye toggle flips published', async () => {
    const update = vi.spyOn(api, 'adminUpdateProduct').mockResolvedValue(products[0])
    renderPage()
    await screen.findByText('CurrencyConverterAPI')
    await userEvent.click(screen.getAllByRole('button', { name: 'Dépublier' })[0])
    await waitFor(() => expect(update).toHaveBeenCalled())
    expect(update.mock.calls[0][2].published).toBe(false)
  })

  it('delete goes through the confirm modal', async () => {
    const del = vi.spyOn(api, 'adminDeleteProduct').mockResolvedValue(undefined)
    renderPage()
    await screen.findByText('CurrencyConverterAPI')
    await userEvent.click(screen.getAllByRole('button', { name: 'Supprimer' })[0])
    const dialog = await screen.findByRole('dialog')
    expect(dialog).toHaveTextContent('/currencyconv')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Supprimer' }))
    await waitFor(() => expect(del).toHaveBeenCalledWith('jwt', 1))
  })

  it('a 409 delete shows the active-subscriptions toast and keeps the row', async () => {
    vi.spyOn(api, 'adminDeleteProduct').mockRejectedValue(new ApiError('conflict', 409))
    renderPage()
    await screen.findByText('CurrencyConverterAPI')
    await userEvent.click(screen.getAllByRole('button', { name: 'Supprimer' })[0])
    const dialog = await screen.findByRole('dialog')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Supprimer' }))
    expect(await screen.findByText('Suppression impossible : abonnements actifs.')).toBeInTheDocument()
    expect(screen.getByText('CurrencyConverterAPI')).toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run to verify failure**

Run: `npx vitest run src/pages/admin/ProductsPage.test.tsx` — FAIL (module not found).

- [ ] **Step 3: Create `web/src/pages/admin/ProductsPage.tsx`**

```tsx
import { useCallback, useEffect, useState } from 'react'
import type { AdminProduct } from '../../api/types'
import { adminGetProducts, adminCreateProduct, adminUpdateProduct, adminDeleteProduct, ApiError } from '../../api/client'
import { useAuth } from '../../auth/AuthProvider'
import { AdminShell } from './AdminShell'
import { Composer } from './Composer'
import { catMeta, slugify } from './meta'
import { Toast, useToast } from '../../components/Toast'
import { ConfirmModal, type ModalSpec } from '../../components/ConfirmModal'

interface FormState {
  name: string; slug: string; category: string; contextPath: string
  upstreamUrl: string; version: string; published: boolean
}
const EMPTY: FormState = { name: '', slug: '', category: '', contextPath: '', upstreamUrl: '', version: '1.0.0', published: true }

function PlusIcon() {
  return <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} aria-hidden="true"><path d="M12 5v14M5 12h14" strokeLinecap="round" /></svg>
}

export function ProductsPage() {
  const { token } = useAuth()
  const [products, setProducts] = useState<AdminProduct[]>([])
  const [filter, setFilter] = useState('')
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<AdminProduct | null>(null)
  const [form, setForm] = useState<FormState>(EMPTY)
  const [slugTouched, setSlugTouched] = useState(false)
  const [modal, setModal] = useState<ModalSpec | null>(null)
  const [err, setErr] = useState('')
  const { toast, notify } = useToast()

  const reload = useCallback(() => {
    if (!token) return
    adminGetProducts(token).then(setProducts).catch(() => setErr('Impossible de charger les produits.'))
  }, [token])
  useEffect(reload, [reload])

  const categories = [...new Set(products.map(p => p.category).filter(Boolean))].sort()
  const q = filter.trim().toLowerCase()
  const shown = products.filter(p => !q || `${p.name} ${p.contextPath} ${p.category} ${p.upstreamUrl}`.toLowerCase().includes(q))

  function set<K extends keyof FormState>(k: K, v: FormState[K]) { setForm(f => ({ ...f, [k]: v })) }

  function openCreate() { setEditing(null); setForm(EMPTY); setSlugTouched(false); setOpen(true) }
  function openEdit(p: AdminProduct) {
    setEditing(p)
    setForm({ name: p.name, slug: p.slug, category: p.category, contextPath: p.contextPath, upstreamUrl: p.upstreamUrl, version: p.version, published: p.published })
    setSlugTouched(true)
    setOpen(true)
  }

  async function submit() {
    if (!token || !form.name.trim()) return
    const slug = form.slug.trim() || slugify(form.name)
    const payload: AdminProduct = {
      ...(editing ?? { description: '', tags: [], icon: '' }),
      name: form.name.trim(),
      slug,
      category: form.category.trim(),
      contextPath: form.contextPath.trim() || `/${slug}`,
      upstreamUrl: form.upstreamUrl.trim(),
      version: form.version.trim() || '1.0.0',
      published: form.published,
    }
    try {
      if (editing?.id != null) {
        await adminUpdateProduct(token, editing.id, payload)
        notify(`${payload.name} enregistré`)
      } else {
        await adminCreateProduct(token, payload)
        notify(`${payload.name} créé${payload.published ? '' : ' (brouillon)'}`)
      }
      setOpen(false)
      reload()
    } catch (e) { notify(e instanceof Error ? e.message : "Échec de l'enregistrement.", 'warn') }
  }

  async function togglePub(p: AdminProduct) {
    if (!token || p.id == null) return
    try {
      await adminUpdateProduct(token, p.id, { ...p, published: !p.published })
      notify(`${p.name}${p.published ? ' retiré du catalogue' : ' publié au catalogue'}`, p.published ? 'warn' : 'ok')
      reload()
    } catch (e) { notify(e instanceof Error ? e.message : 'Échec.', 'warn') }
  }

  function askDelete(p: AdminProduct) {
    setModal({
      title: 'Supprimer le produit ?',
      body: `La route APISIX ${p.contextPath} de ${p.name} sera retirée de la gateway. Les abonnements liés seront révoqués.`,
      confirmLabel: 'Supprimer', danger: true,
      onConfirm: () => {
        if (!token || p.id == null) return
        adminDeleteProduct(token, p.id)
          .then(() => { notify(`${p.name} supprimé`, 'warn'); reload() })
          .catch(e => notify(
            e instanceof ApiError && e.status === 409
              ? 'Suppression impossible : abonnements actifs.'
              : 'Échec de la suppression.',
            'warn'))
      },
    })
  }

  return (
    <AdminShell
      active="products"
      title="Produits"
      description="Les produits exposent vos services en amont (upstream) à travers la passerelle APISIX, avec un contexte de routage et une version publiables au catalogue développeur."
      counts={{ products: products.length }}
      action={
        <button className="btn btn-primary" onClick={() => open ? setOpen(false) : openCreate()}>
          <PlusIcon />Nouveau produit
        </button>
      }
    >
      {err && <p className="autherr" role="alert">{err}</p>}

      <Composer
        open={open}
        title={editing ? 'Modifier le produit' : 'Créer un produit'}
        hint="Le routage APISIX est appliqué à la publication"
        submitLabel={editing ? 'Enregistrer' : 'Créer le produit'}
        onSubmit={submit}
        onCancel={() => setOpen(false)}
        footLeft={
          <label className="switch">
            <input type="checkbox" checked={form.published} onChange={e => set('published', e.target.checked)} />
            Publié au catalogue
          </label>
        }
      >
        <div className="grid2">
          <div className="field">
            <label htmlFor="f-name">Nom</label>
            <input id="f-name" className="ipt" placeholder="CurrencyConverterAPI" autoComplete="off" autoFocus
              value={form.name}
              onChange={e => { set('name', e.target.value); if (!slugTouched) set('slug', slugify(e.target.value)) }} />
          </div>
          <div className="field">
            <label htmlFor="f-slug">Slug</label>
            <input id="f-slug" className="ipt mono" placeholder="currency-converter" autoComplete="off"
              value={form.slug}
              onChange={e => { setSlugTouched(true); set('slug', e.target.value) }} />
            <div className="help">généré depuis le nom — modifiable</div>
          </div>
          <div className="field">
            <label htmlFor="f-cat">Catégorie</label>
            <input id="f-cat" className="ipt" list="cat-options" autoComplete="off"
              value={form.category} onChange={e => set('category', e.target.value)} />
            <datalist id="cat-options">
              {categories.map(c => <option key={c} value={c} />)}
            </datalist>
          </div>
          <div className="field">
            <label htmlFor="f-ctx">Context path</label>
            <input id="f-ctx" className="ipt mono" placeholder="/currencyconv" autoComplete="off"
              value={form.contextPath} onChange={e => set('contextPath', e.target.value)} />
            <div className="help">préfixe de route exposé par la gateway</div>
          </div>
          <div className="field">
            <label htmlFor="f-up">Upstream <span className="opt">host:port</span></label>
            <input id="f-up" className="ipt mono" placeholder="echo:8080" autoComplete="off"
              value={form.upstreamUrl} onChange={e => set('upstreamUrl', e.target.value)} />
          </div>
          <div className="field">
            <label htmlFor="f-ver">Version</label>
            <input id="f-ver" className="ipt mono" autoComplete="off"
              value={form.version} onChange={e => set('version', e.target.value)} />
          </div>
        </div>
      </Composer>

      <div className="list-head">
        <h3>Tous les produits</h3>
        <div className="list-filter">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} aria-hidden="true"><circle cx="11" cy="11" r="7" /><path d="M21 21l-4-4" strokeLinecap="round" /></svg>
          <input type="search" placeholder="Filtrer les produits…" aria-label="Filtrer"
            value={filter} onChange={e => setFilter(e.target.value)} />
        </div>
      </div>

      <div className="rows">
        {shown.length === 0 && (
          <div className="aempty"><h4>Aucun résultat</h4><p>Aucun produit ne correspond à ce filtre.</p></div>
        )}
        {shown.map(p => {
          const m = catMeta(p.category)
          return (
            <div className="row" key={p.id}>
              <div className="swatch" style={{ background: m.color }}>
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">{m.icon}</svg>
              </div>
              <div className="main">
                <div className="nm"><b>{p.name}</b><span className="actx">{p.contextPath}</span></div>
                <div className="meta">
                  <span className="acat" style={{ color: m.color }}>{p.category}</span>
                  <span className="asep">·</span>
                  <span className="up">{p.upstreamUrl || 'pas d’upstream'}</span>
                  <span className="asep">·</span>
                  <span className={`apill ${p.published ? 'pub' : 'draft'}`}><span className="pdot" />{p.published ? 'publié' : 'brouillon'}</span>
                </div>
              </div>
              <span className="ver-col">v{p.version}</span>
              <div className="actions">
                <button className="iact" title={p.published ? 'Dépublier' : 'Publier'} aria-label={p.published ? 'Dépublier' : 'Publier'} onClick={() => togglePub(p)}>
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} aria-hidden="true">
                    {p.published
                      ? <path d="M9.9 4.2A9 9 0 0121 12M14.1 19.8A9 9 0 013 12M2 2l20 20M9.9 9.9a3 3 0 004.2 4.2" strokeLinecap="round" />
                      : <><path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7-10-7-10-7z" /><circle cx="12" cy="12" r="3" /></>}
                  </svg>
                </button>
                <button className="iact" title="Modifier" aria-label="Modifier" onClick={() => openEdit(p)}>
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="M12 20h9M16.5 3.5a2.1 2.1 0 013 3L7 19l-4 1 1-4z" /></svg>
                </button>
                <button className="iact del" title="Supprimer" aria-label="Supprimer" onClick={() => askDelete(p)}>
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="M3 6h18M8 6V4a2 2 0 012-2h4a2 2 0 012 2v2m2 0v14a2 2 0 01-2 2H7a2 2 0 01-2-2V6" /></svg>
                </button>
              </div>
            </div>
          )
        })}
      </div>

      <Toast msg={toast?.msg ?? null} kind={toast?.kind} />
      <ConfirmModal spec={modal} onClose={() => setModal(null)} />
    </AdminShell>
  )
}
```

Note on the test querying `screen.getByLabelText('Upstream')`: the label text is `Upstream host:port` (label + opt span). If `getByLabelText('Upstream')` fails on exact matching, use `screen.getByLabelText(/Upstream/)` in the test — adjust the TEST, not the markup.

- [ ] **Step 4: Run the page tests, then everything**

Run: `npx vitest run src/pages/admin/ProductsPage.test.tsx` — 7 pass.
Run: `npx vitest run` — all pass; `npx tsc -b --force` — clean.

- [ ] **Step 5: Commit**

```bash
git add src/pages/admin/ProductsPage.tsx src/pages/admin/ProductsPage.test.tsx
git commit -m "feat(web): admin products page — blueprint rows, composer, real CRUD"
```

---

### Task 9: Plans page

**Files:**
- Create: `web/src/pages/admin/PlansPage.tsx`
- Test: `web/src/pages/admin/PlansPage.test.tsx`

- [ ] **Step 1: Write the failing tests**

`web/src/pages/admin/PlansPage.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { PlansPage } from './PlansPage'
import { AuthProvider } from '../../auth/AuthProvider'
import * as api from '../../api/client'
import { ApiError } from '../../api/client'
import type { Plan } from '../../api/types'

const plans: Plan[] = [
  { id: 1, name: 'Free', rateLimit: 60, windowSeconds: 60 },
  { id: 3, name: 'Gold', rateLimit: 1000, windowSeconds: 60 },
]

beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('token', 'jwt')
  localStorage.setItem('user', JSON.stringify({ id: 1, email: 'a@b.c', name: 'Admin', role: 'admin' }))
  vi.restoreAllMocks()
  vi.spyOn(api, 'adminGetPlans').mockResolvedValue(plans)
  vi.spyOn(api, 'adminGetProducts').mockResolvedValue([])
  vi.spyOn(api, 'adminGetSubscriptions').mockResolvedValue([])
})

const renderPage = () => render(
  <MemoryRouter><AuthProvider><PlansPage /></AuthProvider></MemoryRouter>
)

describe('PlansPage', () => {
  it('renders plan rows with limit chip and sustained rate', async () => {
    renderPage()
    expect(await screen.findByText('Free')).toBeInTheDocument()
    expect(screen.getByText('60 req / 60s')).toBeInTheDocument()
    expect(screen.getByText(/≈ 1 req\/s soutenu/)).toBeInTheDocument()
    expect(screen.getByText(/≈ 17 req\/s soutenu/)).toBeInTheDocument()
  })

  it('creates a plan and shows the live preview', async () => {
    const create = vi.spyOn(api, 'adminCreatePlan').mockResolvedValue(plans[0])
    renderPage()
    await screen.findByText('Free')
    await userEvent.click(screen.getByRole('button', { name: /Nouveau plan/ }))
    await userEvent.type(screen.getByLabelText('Nom du plan'), 'Platinum')
    expect(screen.getByText('≈ 1.7 req/s soutenu')).toBeInTheDocument() // 100/60 default
    await userEvent.click(screen.getByRole('button', { name: /Créer le plan/ }))
    await waitFor(() => expect(create).toHaveBeenCalled())
    expect(create.mock.calls[0][1]).toMatchObject({ name: 'Platinum', rateLimit: 100, windowSeconds: 60 })
  })

  it('edit opens prefilled and saves with PUT', async () => {
    const update = vi.spyOn(api, 'adminUpdatePlan').mockResolvedValue(plans[0])
    renderPage()
    await screen.findByText('Free')
    await userEvent.click(screen.getAllByRole('button', { name: 'Modifier' })[0])
    expect(screen.getByLabelText('Nom du plan')).toHaveValue('Free')
    await userEvent.clear(screen.getByLabelText(/Limite/))
    await userEvent.type(screen.getByLabelText(/Limite/), '120')
    await userEvent.click(screen.getByRole('button', { name: /Enregistrer/ }))
    await waitFor(() => expect(update).toHaveBeenCalled())
    expect(update.mock.calls[0][2]).toMatchObject({ name: 'Free', rateLimit: 120 })
  })

  it('a 409 delete shows the plan-in-use toast', async () => {
    vi.spyOn(api, 'adminDeletePlan').mockRejectedValue(new ApiError('in use', 409))
    renderPage()
    await screen.findByText('Free')
    await userEvent.click(screen.getAllByRole('button', { name: 'Supprimer' })[0])
    const dialog = await screen.findByRole('dialog')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Supprimer' }))
    expect(await screen.findByText('Suppression impossible : des abonnements utilisent ce plan.')).toBeInTheDocument()
    expect(screen.getByText('Free')).toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run to verify failure**

Run: `npx vitest run src/pages/admin/PlansPage.test.tsx` — FAIL (module not found).

- [ ] **Step 3: Create `web/src/pages/admin/PlansPage.tsx`**

```tsx
import { useCallback, useEffect, useState } from 'react'
import type { Plan } from '../../api/types'
import { adminGetPlans, adminCreatePlan, adminUpdatePlan, adminDeletePlan, ApiError } from '../../api/client'
import { useAuth } from '../../auth/AuthProvider'
import { AdminShell } from './AdminShell'
import { Composer } from './Composer'
import { planRate, planPreview } from './meta'
import { Toast, useToast } from '../../components/Toast'
import { ConfirmModal, type ModalSpec } from '../../components/ConfirmModal'

const TIERS = ['Free', 'Silver', 'Gold']

function PlusIcon() {
  return <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} aria-hidden="true"><path d="M12 5v14M5 12h14" strokeLinecap="round" /></svg>
}

export function PlansPage() {
  const { token } = useAuth()
  const [plans, setPlans] = useState<Plan[]>([])
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<Plan | null>(null)
  const [name, setName] = useState('')
  const [limit, setLimit] = useState(100)
  const [windowS, setWindowS] = useState(60)
  const [modal, setModal] = useState<ModalSpec | null>(null)
  const [err, setErr] = useState('')
  const { toast, notify } = useToast()

  const reload = useCallback(() => {
    if (!token) return
    adminGetPlans(token).then(setPlans).catch(() => setErr('Impossible de charger les plans.'))
  }, [token])
  useEffect(reload, [reload])

  function openCreate() { setEditing(null); setName(''); setLimit(100); setWindowS(60); setOpen(true) }
  function openEdit(p: Plan) { setEditing(p); setName(p.name); setLimit(p.rateLimit); setWindowS(p.windowSeconds); setOpen(true) }

  async function submit() {
    if (!token || !name.trim()) return
    const payload: Plan = { id: editing?.id ?? 0, name: name.trim(), rateLimit: limit || 100, windowSeconds: windowS || 60 }
    try {
      if (editing) { await adminUpdatePlan(token, editing.id, payload); notify(`Plan ${payload.name} enregistré`) }
      else { await adminCreatePlan(token, payload); notify(`Plan ${payload.name} créé`) }
      setOpen(false)
      reload()
    } catch (e) { notify(e instanceof Error ? e.message : "Échec de l'enregistrement.", 'warn') }
  }

  function askDelete(p: Plan) {
    setModal({
      title: 'Supprimer le plan ?',
      body: `Le plan ${p.name} (${p.rateLimit} req/${p.windowSeconds}s) ne pourra plus être choisi pour de nouveaux abonnements.`,
      confirmLabel: 'Supprimer', danger: true,
      onConfirm: () => {
        if (!token) return
        adminDeletePlan(token, p.id)
          .then(() => { notify(`Plan ${p.name} supprimé`, 'warn'); reload() })
          .catch(e => notify(
            e instanceof ApiError && e.status === 409
              ? 'Suppression impossible : des abonnements utilisent ce plan.'
              : 'Échec de la suppression.',
            'warn'))
      },
    })
  }

  return (
    <AdminShell
      active="plans"
      title="Plans de débit"
      description={<>Chaque plan applique une politique <code>limit-count</code> : un nombre de requêtes autorisé sur une fenêtre glissante. Les abonnements lient une application à une API selon un plan.</>}
      counts={{ plans: plans.length }}
      action={
        <button className="btn btn-primary" onClick={() => open ? setOpen(false) : openCreate()}>
          <PlusIcon />Nouveau plan
        </button>
      }
    >
      {err && <p className="autherr" role="alert">{err}</p>}

      <Composer
        open={open}
        title={editing ? 'Modifier le plan' : 'Créer un plan'}
        hint="mappé sur limit-count à la sauvegarde"
        submitLabel={editing ? 'Enregistrer' : 'Créer le plan'}
        onSubmit={submit}
        onCancel={() => setOpen(false)}
        footLeft={<span className="preview">{planPreview(limit, windowS)}</span>}
      >
        <div className="grid2 plans">
          <div className="field">
            <label htmlFor="p-name">Nom du plan</label>
            <input id="p-name" className="ipt" placeholder="Platinum" autoComplete="off" autoFocus
              value={name} onChange={e => setName(e.target.value)} />
          </div>
          <div className="field">
            <label htmlFor="p-limit">Limite <span className="opt">requêtes</span></label>
            <input id="p-limit" className="ipt mono" type="number" min={1}
              value={limit} onChange={e => setLimit(Number(e.target.value))} />
          </div>
          <div className="field">
            <label htmlFor="p-window">Fenêtre <span className="opt">secondes</span></label>
            <input id="p-window" className="ipt mono" type="number" min={1}
              value={windowS} onChange={e => setWindowS(Number(e.target.value))} />
          </div>
        </div>
      </Composer>

      <div className="list-head"><h3>Plans disponibles</h3></div>
      <div className="rows">
        {plans.length === 0 && (
          <div className="aempty"><h4>Aucun plan</h4><p>Créez un premier plan de débit.</p></div>
        )}
        {plans.map(p => (
          <div className={`row plan ${TIERS.includes(p.name) ? `tier-${p.name}` : ''}`} key={p.id}>
            <div className="swatch">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="M13 2L3 14h7l-1 8 10-12h-7z" /></svg>
            </div>
            <div className="main">
              <div className="nm"><b>{p.name}</b><span className="limit">{p.rateLimit} req / {p.windowSeconds}s</span></div>
              <div className="meta">{planRate(p.rateLimit, p.windowSeconds)} · politique <span className="up">limit-count</span></div>
            </div>
            <div className="actions">
              <button className="iact" title="Modifier" aria-label="Modifier" onClick={() => openEdit(p)}>
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="M12 20h9M16.5 3.5a2.1 2.1 0 013 3L7 19l-4 1 1-4z" /></svg>
              </button>
              <button className="iact del" title="Supprimer" aria-label="Supprimer" onClick={() => askDelete(p)}>
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="M3 6h18M8 6V4a2 2 0 012-2h4a2 2 0 012 2v2m2 0v14a2 2 0 01-2 2H7a2 2 0 01-2-2V6" /></svg>
              </button>
            </div>
          </div>
        ))}
      </div>

      <Toast msg={toast?.msg ?? null} kind={toast?.kind} />
      <ConfirmModal spec={modal} onClose={() => setModal(null)} />
    </AdminShell>
  )
}
```

- [ ] **Step 4: Run the page tests, then everything**

Run: `npx vitest run src/pages/admin/PlansPage.test.tsx` — 4 pass.
Run: `npx vitest run` — all pass; `npx tsc -b --force` — clean.

- [ ] **Step 5: Commit**

```bash
git add src/pages/admin/PlansPage.tsx src/pages/admin/PlansPage.test.tsx
git commit -m "feat(web): admin plans page — blueprint rows, composer, real CRUD"
```

---

### Task 10: Approvals page

**Files:**
- Create: `web/src/pages/admin/ApprovalsPage.tsx`
- Test: `web/src/pages/admin/ApprovalsPage.test.tsx`

- [ ] **Step 1: Write the failing tests**

`web/src/pages/admin/ApprovalsPage.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { ApprovalsPage } from './ApprovalsPage'
import { AuthProvider } from '../../auth/AuthProvider'
import * as api from '../../api/client'
import type { AdminSubscription } from '../../api/types'

const subs: AdminSubscription[] = [
  { id: 11, applicationName: 'MobileCheckout', ownerEmail: 'lina@acme.io', productName: 'CurrencyConverterAPI', version: '1.0.0', planName: 'Silver', status: 'pending', createdAt: '2026-06-06T10:00:00Z' },
  { id: 12, applicationName: 'PartnerSync', ownerEmail: 'dev@partner.dev', productName: 'PeopleAPI', version: '1.3.2', planName: 'Free', status: 'pending', createdAt: '2026-06-04T09:00:00Z' },
]

beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('token', 'jwt')
  localStorage.setItem('user', JSON.stringify({ id: 1, email: 'a@b.c', name: 'Admin', role: 'admin' }))
  vi.restoreAllMocks()
  vi.spyOn(api, 'adminGetSubscriptions').mockResolvedValue(subs)
  vi.spyOn(api, 'adminGetProducts').mockResolvedValue([])
  vi.spyOn(api, 'adminGetPlans').mockResolvedValue([])
})

const renderPage = () => render(
  <MemoryRouter><AuthProvider><ApprovalsPage /></AuthProvider></MemoryRouter>
)

describe('ApprovalsPage', () => {
  it('renders pending rows: avatar initials, app → product, plan tag, requester and date', async () => {
    renderPage()
    expect(await screen.findByText('MobileCheckout')).toBeInTheDocument()
    expect(screen.getByText('MC')).toBeInTheDocument()
    expect(screen.getByText('CurrencyConverterAPI')).toBeInTheDocument()
    expect(screen.getByText('Silver')).toBeInTheDocument()
    expect(screen.getByText('lina@acme.io')).toBeInTheDocument()
    expect(screen.getByText('2026-06-06')).toBeInTheDocument()
  })

  it('approve calls the API and removes the row', async () => {
    const approve = vi.spyOn(api, 'adminApproveSubscription').mockResolvedValue(undefined)
    vi.spyOn(api, 'adminGetSubscriptions')
      .mockResolvedValueOnce(subs)            // initial load
      .mockResolvedValue([subs[1]])           // after approval
    renderPage()
    await screen.findByText('MobileCheckout')
    await userEvent.click(screen.getAllByRole('button', { name: 'Approuver' })[0])
    await waitFor(() => expect(approve).toHaveBeenCalledWith('jwt', 11))
    await waitFor(() => expect(screen.queryByText('MobileCheckout')).not.toBeInTheDocument())
    expect(screen.getByText(/approuvé — consumer APISIX créé/)).toBeInTheDocument()
  })

  it('reject calls the API and removes the row', async () => {
    const reject = vi.spyOn(api, 'adminRejectSubscription').mockResolvedValue(undefined)
    vi.spyOn(api, 'adminGetSubscriptions')
      .mockResolvedValueOnce(subs)
      .mockResolvedValue([subs[0]])
    renderPage()
    await screen.findByText('PartnerSync')
    await userEvent.click(screen.getAllByRole('button', { name: 'Refuser' })[1])
    await waitFor(() => expect(reject).toHaveBeenCalledWith('jwt', 12))
    await waitFor(() => expect(screen.queryByText('PartnerSync')).not.toBeInTheDocument())
  })

  it('shows the blueprint empty state when the queue is empty', async () => {
    vi.spyOn(api, 'adminGetSubscriptions').mockResolvedValue([])
    renderPage()
    expect(await screen.findByText("File d'attente vide")).toBeInTheDocument()
    expect(screen.getByText('Aucun abonnement en attente de validation.')).toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run to verify failure**

Run: `npx vitest run src/pages/admin/ApprovalsPage.test.tsx` — FAIL (module not found).

- [ ] **Step 3: Create `web/src/pages/admin/ApprovalsPage.tsx`**

```tsx
import { useCallback, useEffect, useState } from 'react'
import type { AdminSubscription } from '../../api/types'
import { adminGetSubscriptions, adminApproveSubscription, adminRejectSubscription } from '../../api/client'
import { useAuth } from '../../auth/AuthProvider'
import { AdminShell } from './AdminShell'
import { Toast, useToast } from '../../components/Toast'

// Blueprint avatar initials: capitals of the app name, else first two letters.
const subInitials = (name: string) =>
  name.replace(/[^A-Z]/g, '').slice(0, 2) || name.slice(0, 2).toUpperCase()

const KNOWN_TAGS = ['Free', 'Silver', 'Gold']

export function ApprovalsPage() {
  const { token } = useAuth()
  const [subs, setSubs] = useState<AdminSubscription[]>([])
  const [loaded, setLoaded] = useState(false)
  const [err, setErr] = useState('')
  const { toast, notify } = useToast()

  const reload = useCallback(() => {
    if (!token) return
    adminGetSubscriptions(token, 'pending')
      .then(l => { setSubs(l); setLoaded(true) })
      .catch(() => setErr('Impossible de charger les abonnements.'))
  }, [token])
  useEffect(reload, [reload])

  async function approve(s: AdminSubscription) {
    if (!token) return
    try {
      await adminApproveSubscription(token, s.id)
      notify(`Abonnement de ${s.applicationName} approuvé — consumer APISIX créé`)
      reload()
    } catch (e) { notify(e instanceof Error ? e.message : "Échec de l'approbation.", 'warn') }
  }

  async function reject(s: AdminSubscription) {
    if (!token) return
    try {
      await adminRejectSubscription(token, s.id)
      notify(`Demande de ${s.applicationName} refusée`, 'warn')
      reload()
    } catch (e) { notify(e instanceof Error ? e.message : 'Échec du refus.', 'warn') }
  }

  return (
    <AdminShell
      active="approvals"
      title="Abonnements en attente"
      description="Validez ou refusez les demandes d'abonnement des applications. À l'approbation, APISIX crée le consumer et active la politique de débit du plan choisi."
      counts={{ pending: loaded ? subs.length : undefined }}
    >
      {err && <p className="autherr" role="alert">{err}</p>}

      {loaded && subs.length === 0 ? (
        <div className="aempty">
          <div className="aico">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} aria-hidden="true"><path d="M20 6L9 17l-5-5" strokeLinecap="round" strokeLinejoin="round" /></svg>
          </div>
          <h4>File d'attente vide</h4>
          <p>Aucun abonnement en attente de validation.</p>
        </div>
      ) : (
        <div className="rows">
          {subs.map(s => (
            <div className="sub-card" key={s.id}>
              <div className="app-av">{subInitials(s.applicationName)}</div>
              <div className="sub-main">
                <div className="ttl"><b>{s.applicationName}</b><span className="arr">→</span>{s.productName}</div>
                <div className="sub-meta">
                  <span className={`plan-tag ${KNOWN_TAGS.includes(s.planName) ? s.planName : ''}`}>{s.planName}</span>
                  <span>demandé par <span className="who2">{s.ownerEmail}</span></span>
                  ·
                  <time>{s.createdAt.slice(0, 10)}</time>
                </div>
              </div>
              <div className="sub-acts">
                <button className="btn-reject" onClick={() => reject(s)}>Refuser</button>
                <button className="btn-approve" onClick={() => approve(s)}>Approuver</button>
              </div>
            </div>
          ))}
        </div>
      )}

      <Toast msg={toast?.msg ?? null} kind={toast?.kind} />
    </AdminShell>
  )
}
```

- [ ] **Step 4: Run the page tests, then everything**

Run: `npx vitest run src/pages/admin/ApprovalsPage.test.tsx` — 4 pass.
Run: `npx vitest run` — all pass; `npx tsc -b --force` — clean.

- [ ] **Step 5: Commit**

```bash
git add src/pages/admin/ApprovalsPage.tsx src/pages/admin/ApprovalsPage.test.tsx
git commit -m "feat(web): admin approvals page — blueprint queue, real approve/reject"
```

---

### Task 11: Routes, cleanup of the old pages, full verification

**Files:**
- Modify: `web/src/App.tsx`
- Delete: `web/src/pages/AdminProductsPage.tsx`, `web/src/pages/AdminPlansPage.tsx`, `web/src/pages/AdminApprovalsPage.tsx`, their three `.test.tsx` files, `web/src/components/AdminNav.tsx`

- [ ] **Step 1: Update `web/src/App.tsx`**

Replace the three old admin imports with:

```tsx
import { Navigate } from 'react-router-dom'   // merge into the existing react-router-dom import
import { ProductsPage } from './pages/admin/ProductsPage'
import { PlansPage } from './pages/admin/PlansPage'
import { ApprovalsPage } from './pages/admin/ApprovalsPage'
```

Replace the three admin routes with:

```tsx
      <Route path="/admin" element={<Navigate to="/admin/products" replace />} />
      <Route path="/admin/products" element={<AdminGuard><ProductsPage /></AdminGuard>} />
      <Route path="/admin/plans" element={<AdminGuard><PlansPage /></AdminGuard>} />
      <Route path="/admin/approvals" element={<AdminGuard><ApprovalsPage /></AdminGuard>} />
```

- [ ] **Step 2: Delete the old pages and nav**

```bash
git rm src/pages/AdminProductsPage.tsx src/pages/AdminProductsPage.test.tsx \
       src/pages/AdminPlansPage.tsx src/pages/AdminPlansPage.test.tsx \
       src/pages/AdminApprovalsPage.tsx src/pages/AdminApprovalsPage.test.tsx \
       src/components/AdminNav.tsx
```

Verify nothing references them: `grep -rn "AdminNav\|AdminProductsPage\|AdminPlansPage\|AdminApprovalsPage" src/` → no hits (the new pages have different names).

- [ ] **Step 3: Full gates**

Run: `npx vitest run` — all pass (report the final count).
Run: `npx tsc -b --force` — clean.
Run: `npm run build` — success.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "feat(web): admin routes use blueprint pages, /admin redirect, old pages removed"
```

- [ ] **Step 5: Browser verification (executor with browser access)**

With the dev stack up (vite :5174, portal :8090, blueprint server :8888 — see repo conventions; admin login `admin@portal.local`/`adminpass`):
1. `/admin` redirects to `/admin/products`; pill nav shows three real counts.
2. Products: rows match `http://localhost:8888/admin-products.html` (swatch colors, ctx chip, pills, version, hover-revealed actions); filter works; "Nouveau produit" opens the composer (auto-slug while typing); edit prefills; eye toggles publié/brouillon with toast; delete shows the modal; deleting a product WITH active subscriptions shows «Suppression impossible : abonnements actifs.» and keeps the row.
3. Plans: chips + sustained rates; composer preview updates live.
4. Abonnements: row layout (avatar, app → product, plan tag, requester, date), green Approuver; approving moves the subscription (check the app detail page sees it active); empty state appears when the queue is drained.
5. Dark mode and 880px/560px widths on all three pages.
6. No leakage: catalog, application detail, auth pages unchanged.

---

## Self-review notes (already applied)

- Spec coverage: decisions 1–3 → Tasks 6/8/11 (three tabs, kept routes + `/admin` redirect, datalist combo). Real-vs-blueprint deltas → Task 8 (real edit, 409 toast, eye toggle), Task 10 (real queue fields). Toast/ConfirmModal promotion + overlays.css → Task 3. Collision renames + dark + responsive → Task 4. ApiError → Task 2.
- The blueprint's localStorage tab memory (`apisix:admin:tab`) is intentionally NOT ported: tabs are real routes (decision 2), the URL is the state.
- The blueprint's create-defaults `up:'echo:8080'` is intentionally NOT ported (an empty upstream stays empty — fabricating a default upstream would create wrong routing); the placeholder still shows `echo:8080`.
- Type consistency: `FormState`/`AdminProduct` field names match `web/src/api/types.ts`; `ModalSpec` comes from the promoted `components/ConfirmModal`; `useToast` returns `{ toast, notify }` used identically in Tasks 8–10.
