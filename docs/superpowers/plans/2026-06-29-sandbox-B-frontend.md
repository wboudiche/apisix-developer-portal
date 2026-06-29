# Sandbox Environment — Plan 2 (Frontend) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Surface the sandbox backend in the UI — a Sandbox credential card (enable + rotate + base URL), a Production/Sandbox Try-it toggle on the product page, and a Sandbox-upstream field in the admin product Composer.

**Architecture:** Additive React 19 + TS work in `web/`. New client fns `enableSandbox`/`rotateSandboxKey` mirror the existing `rotateKey`; new optional sandbox fields on `AppDetail`/`SubscriptionView`/`AdminProduct`/the try-it context (the Go backend already returns them). The Credentials tab gets a second card; the product detail Try-it gets a server-URL toggle; the admin Composer gets one field.

**Tech Stack:** React 19 + TypeScript, Vite, vitest, pnpm.

## Global Constraints

- All commands run from `web/` with pnpm. French copy; reuse Atlas CSS tokens + existing `.keycard`/`.keygrid` styles.
- The new sandbox type fields are **optional** (`sandboxEnabled?`, `sandboxGatewayUrl?`, `sandboxAvailable?`, `sandboxUpstreamUrl?`) and components default them (`?? false` / `?? ''`). This avoids churning the ~10 existing test fixtures that build these types; the backend always sends them.
- Backend contract (already shipped, Plan 1): `GET /api/applications/{id}` returns `sandboxEnabled: boolean`, `sandboxGatewayUrl: string`, and each subscription has `sandboxAvailable: boolean`. `POST /api/applications/{id}/sandbox/enable` and `/sandbox/rotate` return `{ "sandboxApiKey": string }` (200) or 409. The sandbox **key value is NOT in the detail response** — it is revealed once on enable/rotate. `GET /api/try/{slug}/context` returns `{ apps, sandboxAvailable }`. The sandbox try-it proxy path is `/api/try/{slug}/{appId}/sandbox`. Admin product create/update accept `sandboxUpstreamUrl`; admin GET/list return it.
- Gate: `cd web && pnpm exec vitest run --exclude 'e2e/**' && pnpm exec tsc --noEmit && pnpm build` — all green per task.

---

## Task 1: Types + sandbox client fns

**Files:**
- Modify: `web/src/api/types.ts`
- Modify: `web/src/api/client.ts`
- Test: `web/src/api/client.sandbox.test.ts` (new)

**Interfaces:**
- Produces: `AppDetail.sandboxEnabled?: boolean`, `AppDetail.sandboxGatewayUrl?: string`, `SubscriptionView.sandboxAvailable?: boolean`, `AdminProduct.sandboxUpstreamUrl?: string`; `AppEventKind` gains `'sandbox_enabled' | 'sandbox_key_rotated'`; `enableSandbox(token, appId): Promise<{ sandboxApiKey: string }>`; `rotateSandboxKey(token, appId): Promise<{ sandboxApiKey: string }>`; `getTryContext` return type gains `sandboxAvailable?: boolean`.

- [ ] **Step 1: Write the failing client test**

Create `web/src/api/client.sandbox.test.ts`:
```ts
import { it, expect, vi, afterEach } from 'vitest'
import { enableSandbox, rotateSandboxKey } from './client'

afterEach(() => vi.restoreAllMocks())

it('enableSandbox POSTs the enable endpoint with auth', async () => {
  const f = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
    new Response(JSON.stringify({ sandboxApiKey: 'sb-1' }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
  const out = await enableSandbox('jwt', 7)
  expect(out.sandboxApiKey).toBe('sb-1')
  const [url, init] = f.mock.calls[0]
  expect(url).toBe('/api/applications/7/sandbox/enable')
  expect((init as RequestInit).method).toBe('POST')
  expect((init as RequestInit).headers).toMatchObject({ Authorization: 'Bearer jwt' })
})

it('rotateSandboxKey POSTs the rotate endpoint with auth', async () => {
  const f = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
    new Response(JSON.stringify({ sandboxApiKey: 'sb-2' }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
  const out = await rotateSandboxKey('jwt', 7)
  expect(out.sandboxApiKey).toBe('sb-2')
  expect(f.mock.calls[0][0]).toBe('/api/applications/7/sandbox/rotate')
  expect((f.mock.calls[0][1] as RequestInit).method).toBe('POST')
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && pnpm exec vitest run src/api/client.sandbox.test.ts`
Expected: FAIL — `enableSandbox`/`rotateSandboxKey` not exported.

- [ ] **Step 3: Add the type fields**

In `web/src/api/types.ts`:
- In `SubscriptionView` (after `status`): `sandboxAvailable?: boolean`.
- In `AppDetail` (after `consumerUsername`): `sandboxEnabled?: boolean` and `sandboxGatewayUrl?: string`.
- In `AdminProduct` (after `openapiSpec?`): `sandboxUpstreamUrl?: string`.
- In `AppEventKind` union, add `| 'sandbox_enabled' | 'sandbox_key_rotated'`.

- [ ] **Step 4: Add the client fns + try-it context field**

In `web/src/api/client.ts`, add next to `rotateKey` (mirror its shape exactly):
```ts
export async function enableSandbox(token: string, appId: number): Promise<{ sandboxApiKey: string }> {
  const url = `/api/applications/${appId}/sandbox/enable`
  return parse<{ sandboxApiKey: string }>(await fetch(url, { method: 'POST', headers: authHeaders(token) }), url)
}

export async function rotateSandboxKey(token: string, appId: number): Promise<{ sandboxApiKey: string }> {
  const url = `/api/applications/${appId}/sandbox/rotate`
  return parse<{ sandboxApiKey: string }>(await fetch(url, { method: 'POST', headers: authHeaders(token) }), url)
}
```
Change the `getTryContext` return type to include the new field (the cast at the `parse` call follows):
```ts
export async function getTryContext(token: string, slug: string): Promise<{ apps: TryApp[]; sandboxAvailable?: boolean }> {
  const url = `/api/try/${encodeURIComponent(slug)}/context`
  return parse<{ apps: TryApp[]; sandboxAvailable?: boolean }>(await fetch(url, { headers: authHeaders(token) }), url)
}
```

- [ ] **Step 5: Run to verify it passes + gate**

Run: `cd web && pnpm exec vitest run src/api/client.sandbox.test.ts && pnpm exec tsc --noEmit && pnpm build`
Expected: all green.

- [ ] **Step 6: Commit**

```bash
git add web/src/api/types.ts web/src/api/client.ts web/src/api/client.sandbox.test.ts
git commit -m "feat(web): sandbox client fns + types"
```

---

## Task 2: Sandbox card on the Credentials tab

**Files:**
- Modify: `web/src/pages/application/CredentialsTab.tsx`
- Modify: `web/src/pages/application/AppDetailPage.tsx` (pass the new props)
- Test: `web/src/pages/application/CredentialsTab.test.tsx` (new, or extend if present)

**Interfaces:**
- Consumes: `enableSandbox`, `rotateSandboxKey` (Task 1); `AppDetail.sandboxEnabled`/`sandboxGatewayUrl`; `SubscriptionView.sandboxAvailable`.
- Produces: `CredentialsTab` gains props `sandboxEnabled?: boolean`, `sandboxGatewayUrl?: string`, `sandboxEligible: boolean`.

- [ ] **Step 1: Write the failing tests**

Create `web/src/pages/application/CredentialsTab.test.tsx`:
```tsx
import { it, expect, vi, afterEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { CredentialsTab } from './CredentialsTab'
import * as api from '../../api/client'

afterEach(() => vi.restoreAllMocks())

const base = {
  apiKey: 'prod-key-123', appId: 7, token: 'jwt',
  notify: vi.fn(), openModal: vi.fn(), onRotated: vi.fn(),
}

it('shows an enable button when sandbox-eligible but not enabled, and reveals the key', async () => {
  const spy = vi.spyOn(api, 'enableSandbox').mockResolvedValue({ sandboxApiKey: 'sb-new' })
  render(<CredentialsTab {...base} sandboxEligible sandboxEnabled={false} sandboxGatewayUrl="http://localhost:9081" />)
  await userEvent.click(screen.getByRole('button', { name: /Activer le sandbox/i }))
  await waitFor(() => expect(spy).toHaveBeenCalledWith('jwt', 7))
  expect(await screen.findByText('sb-new')).toBeInTheDocument()
})

it('shows the sandbox base URL + Régénérer when already enabled', () => {
  render(<CredentialsTab {...base} sandboxEligible sandboxEnabled sandboxGatewayUrl="http://localhost:9081" />)
  expect(screen.getByText(/localhost:9081/)).toBeInTheDocument()
  expect(screen.getByRole('button', { name: /Régénérer/i })).toBeInTheDocument()
})

it('hides the sandbox card entirely when not eligible', () => {
  render(<CredentialsTab {...base} sandboxEligible={false} sandboxEnabled={false} sandboxGatewayUrl="" />)
  expect(screen.queryByText(/sandbox/i)).not.toBeInTheDocument()
})
```
NOTE: the existing CredentialsTab has a single "Régénérer" button (production). To keep these queries unambiguous, the Production card's rotate button stays labelled "Régénérer" and the Sandbox card's reveal/rotate uses the SAME "Régénérer" label but lives inside a `.keycard.sandbox` — the third test asserts no sandbox markers at all, and the second asserts the base URL is present. If the production "Régénérer" makes the second test's `getByRole` ambiguous, scope it: `within(screen.getByText(/Sandbox/).closest('.keycard')!)`. Use `within` from `@testing-library/react`.

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && pnpm exec vitest run src/pages/application/CredentialsTab.test.tsx`
Expected: FAIL — sandbox card/props absent.

- [ ] **Step 3: Add the props + sandbox card**

In `web/src/pages/application/CredentialsTab.tsx`:
- Import the new client fns: `import { rotateKey, enableSandbox, rotateSandboxKey } from '../../api/client'`.
- Extend the props type:
```tsx
export function CredentialsTab({ apiKey, appId, token, lastRotatedAt, notify, openModal, onRotated,
  sandboxEnabled, sandboxGatewayUrl, sandboxEligible }: {
  apiKey: string
  appId: number
  token: string
  lastRotatedAt?: string
  notify: (msg: string) => void
  openModal: (spec: ModalSpec) => void
  onRotated: () => void
  sandboxEnabled?: boolean
  sandboxGatewayUrl?: string
  sandboxEligible: boolean
}) {
```
- Add sandbox state near the existing `shownKey`/`revealed`:
```tsx
  const [sbKey, setSbKey] = useState('')        // revealed once after enable/rotate
  const [sbRevealed, setSbRevealed] = useState(false)
  const [sbBusy, setSbBusy] = useState(false)
  const hasSandbox = (sandboxEnabled ?? false) || sbKey !== ''

  async function onEnableSandbox() {
    if (sbBusy) return
    setSbBusy(true)
    try {
      const { sandboxApiKey } = await enableSandbox(token, appId)
      setSbKey(sandboxApiKey); setSbRevealed(true)
      notify('Sandbox activé'); onRotated()
    } catch (e) {
      notify(e instanceof Error ? e.message : 'Échec de l’activation du sandbox.')
    } finally { setSbBusy(false) }
  }

  function onRotateSandbox() {
    openModal({
      title: 'Régénérer la clé sandbox ?',
      body: 'L’ancienne clé sandbox sera révoquée immédiatement sur la passerelle sandbox.',
      confirmLabel: 'Régénérer la clé', danger: true,
      onConfirm: async () => {
        try {
          const { sandboxApiKey } = await rotateSandboxKey(token, appId)
          setSbKey(sandboxApiKey); setSbRevealed(true)
          notify('Nouvelle clé sandbox générée'); onRotated()
        } catch (e) {
          notify(e instanceof Error ? e.message : 'Échec de la rotation.')
        }
      },
    })
  }
```
- Inside the `.keygrid`, AFTER the existing `.keycard.prod` div, render the sandbox card only when `sandboxEligible`:
```tsx
        {sandboxEligible && (
          <div className="keycard sandbox">
            <div className="kh"><span className="env">Sandbox <span className="envtag">test</span></span></div>
            <div className="kb">
              {hasSandbox ? (
                <>
                  <div className="keyrow">
                    <code data-testid="key-sandbox">{sbRevealed && sbKey ? sbKey : maskKey(sbKey || '••••••••••••••••')}</code>
                    {sbKey && (
                      <button className="iconbtn" aria-label="Afficher / masquer" aria-pressed={sbRevealed} onClick={() => setSbRevealed(r => !r)}><EyeIcon /></button>
                    )}
                    {sbKey && (
                      <button className="iconbtn" aria-label="Copier" onClick={() => void copyText(sbKey).then(() => notify('Clé sandbox copiée'))}><CopyIcon /></button>
                    )}
                  </div>
                  <div className="keymeta">
                    <span>Passerelle · <span className="mono">{sandboxGatewayUrl || '—'}</span></span>
                    <button className="rotate" onClick={onRotateSandbox}><RotateIcon />Régénérer</button>
                  </div>
                  {!sbKey && <p className="keyhint">Régénérez pour révéler une nouvelle clé sandbox.</p>}
                </>
              ) : (
                <div className="keymeta">
                  <span>Testez vos intégrations sans toucher la production.</span>
                  <button className="rotate" disabled={sbBusy} onClick={onEnableSandbox}>Activer le sandbox</button>
                </div>
              )}
            </div>
          </div>
        )}
```

- [ ] **Step 4: Wire the props in AppDetailPage**

In `web/src/pages/application/AppDetailPage.tsx`, the `tab === 'creds'` block renders `<CredentialsTab ... />`. Add the three props:
```tsx
                sandboxEnabled={detail.sandboxEnabled}
                sandboxGatewayUrl={detail.sandboxGatewayUrl}
                sandboxEligible={subs.some(s => s.sandboxAvailable)}
```
(`subs` is already `detail?.subscriptions ?? []` in that component.)

- [ ] **Step 5: Run to verify it passes + gate**

Run: `cd web && pnpm exec vitest run --exclude 'e2e/**' && pnpm exec tsc --noEmit && pnpm build`
Expected: all green.

- [ ] **Step 6: Commit**

```bash
git add web/src/pages/application/CredentialsTab.tsx web/src/pages/application/CredentialsTab.test.tsx web/src/pages/application/AppDetailPage.tsx
git commit -m "feat(web): sandbox credential card (enable + rotate + base URL)"
```

---

## Task 3: Production/Sandbox Try-it toggle on the product page

**Files:**
- Modify: `web/src/pages/ProductDetailPage.tsx`
- Test: `web/src/pages/ProductDetailPage.test.tsx` (extend; it already mocks `ScalarDocs` + `getTryContext`)

**Interfaces:**
- Consumes: `getTryContext` (now returns `sandboxAvailable`); `ScalarDocs` `serverUrl` prop.
- Produces: a prod/sandbox toggle that switches `serverUrl` between `/api/try/{slug}/{appId}` and `/api/try/{slug}/{appId}/sandbox`.

- [ ] **Step 1: Write the failing test**

In `web/src/pages/ProductDetailPage.test.tsx` (match the file's existing mocking of `ScalarDocs` — typically the mock records the `serverUrl` prop; reuse that), add:
```tsx
it('toggles the Try-it server between production and sandbox', async () => {
  vi.spyOn(api, 'getTryContext').mockResolvedValue({ apps: [{ id: 5, name: 'App' }], sandboxAvailable: true })
  // ...render ProductDetailPage for a spec'd product as an authed user, mirroring the existing try-it test setup...
  // The Sandbox toggle appears because sandboxAvailable && appId != null:
  const sandboxToggle = await screen.findByRole('button', { name: /Sandbox/i })
  await userEvent.click(sandboxToggle)
  // assert the ScalarDocs mock received serverUrl ending in /sandbox
  await waitFor(() => expect(lastServerUrl()).toBe('/api/try/<slug>/5/sandbox'))
})
```
NOTE: adapt to the test file's real harness — reuse its product/spec/auth setup and its `ScalarDocs` mock accessor (e.g. a captured prop or a `data-server` attribute). Replace `<slug>` with the slug the test uses. If the existing `ScalarDocs` mock renders `serverUrl` into the DOM, assert via that instead of a helper.

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && pnpm exec vitest run src/pages/ProductDetailPage.test.tsx`
Expected: FAIL — no Sandbox toggle.

- [ ] **Step 3: Implement the toggle**

In `web/src/pages/ProductDetailPage.tsx`:
- Add state: `const [sandboxAvailable, setSandboxAvailable] = useState(false)` and `const [tryMode, setTryMode] = useState<'prod' | 'sandbox'>('prod')`.
- In the `getTryContext` `.then`, also set sandbox availability:
```tsx
      .then(r => { if (alive) { setApps(r.apps); setAppId(r.apps[0]?.id ?? null); setSandboxAvailable(r.sandboxAvailable ?? false) } })
```
  (and in the `.catch`, `setSandboxAvailable(false)`.)
- Change `serverUrl` to honor the mode:
```tsx
  const serverUrl = appId != null
    ? `/api/try/${slug}/${appId}${tryMode === 'sandbox' ? '/sandbox' : ''}`
    : undefined
```
- Render the toggle near the existing app-picker block (only when authed, an app is selected, and the product has a sandbox upstream):
```tsx
            {token && appId != null && sandboxAvailable && (
              <div className="try-mode" role="group" aria-label="Environnement">
                <button type="button" className={tryMode === 'prod' ? 'on' : ''} onClick={() => setTryMode('prod')}>Production</button>
                <button type="button" className={tryMode === 'sandbox' ? 'on' : ''} onClick={() => setTryMode('sandbox')}>Sandbox</button>
              </div>
            )}
```
- Add minimal CSS to `web/src/styles/productdetail.css` (near the try-banner rules):
```css
.apidetail .try-mode{display:inline-flex;gap:4px;margin:8px 0;border:1px solid var(--border-2);border-radius:10px;padding:3px;width:max-content}
.apidetail .try-mode button{font:inherit;font-size:13px;padding:5px 12px;border:none;background:none;color:var(--muted);border-radius:7px;cursor:pointer}
.apidetail .try-mode button.on{background:var(--accent);color:#fff}
```

- [ ] **Step 4: Run to verify it passes + gate**

Run: `cd web && pnpm exec vitest run --exclude 'e2e/**' && pnpm exec tsc --noEmit && pnpm build`
Expected: all green.

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/ProductDetailPage.tsx web/src/pages/ProductDetailPage.test.tsx web/src/styles/productdetail.css
git commit -m "feat(web): production/sandbox Try-it toggle on the product page"
```

---

## Task 4: Sandbox upstream field in the admin product Composer

**Files:**
- Modify: `web/src/pages/admin/ProductsPage.tsx`
- Test: `web/src/pages/admin/ProductsPage.test.tsx` (extend if present; else a focused new test)

**Interfaces:**
- Consumes: `AdminProduct.sandboxUpstreamUrl` (Task 1).
- Produces: the Composer persists `sandboxUpstreamUrl` through create + update; edit prefills it.

- [ ] **Step 1: Write the failing test**

In `web/src/pages/admin/ProductsPage.test.tsx` add (mirror the file's existing "create product" test — same render + admin auth + open-composer steps):
```tsx
it('sends sandboxUpstreamUrl when creating a product', async () => {
  const spy = vi.spyOn(api, 'adminCreateProduct').mockResolvedValue({} as never)
  // ...render ProductsPage as admin and open the create composer (reuse the file's helper)...
  await userEvent.type(screen.getByLabelText(/Sandbox/i), 'sandbox.example.com:443')
  // ...fill the other required fields the existing create test fills, then submit...
  await waitFor(() => expect(spy).toHaveBeenCalled())
  expect(spy.mock.calls[0][1]).toMatchObject({ sandboxUpstreamUrl: 'sandbox.example.com:443' })
})
```
NOTE: adapt to the existing create-test harness (it already fills name/slug/category/contextPath/upstream and submits) — copy that flow and add the sandbox field. The label text must match Step 3's `<label>`.

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && pnpm exec vitest run src/pages/admin/ProductsPage.test.tsx`
Expected: FAIL — no Sandbox field.

- [ ] **Step 3: Add the field to FormState + Composer + payload**

In `web/src/pages/admin/ProductsPage.tsx`:
- `interface FormState` — add `sandboxUpstreamUrl: string` (next to `upstreamUrl`).
- `EMPTY` — add `sandboxUpstreamUrl: ''`.
- The import-draft prefill (`setForm({...})` from a draft) — add `sandboxUpstreamUrl: draft.sandboxUpstreamUrl ?? ''`.
- The edit prefill (`setForm({...})` from an existing product `p`) — add `sandboxUpstreamUrl: p.sandboxUpstreamUrl ?? ''`.
- The submit payload (where `upstreamUrl: form.upstreamUrl.trim()` is built) — add `sandboxUpstreamUrl: form.sandboxUpstreamUrl.trim()`.
- In the Composer JSX, right after the Upstream field (`<label htmlFor="f-up">Upstream …`), add:
```tsx
      <label htmlFor="f-sbup">Sandbox <span className="opt">host:port — optionnel</span></label>
      <input id="f-sbup" placeholder="ex. sandbox.example.com:443"
        value={form.sandboxUpstreamUrl} onChange={e => set('sandboxUpstreamUrl', e.target.value)} />
```

- [ ] **Step 4: Run to verify it passes + gate**

Run: `cd web && pnpm exec vitest run --exclude 'e2e/**' && pnpm exec tsc --noEmit && pnpm build`
Expected: all green.

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/admin/ProductsPage.tsx web/src/pages/admin/ProductsPage.test.tsx
git commit -m "feat(web): admin sandbox upstream field"
```

---

## Task 5: Live verification

- [ ] **Step 1: Stack + portal + vite running**

The sandbox gateway + portal (`:8090`, sandbox env) + vite (`:5173`, `PORTAL_PROXY=:8090`) should be up from Plan 1's live check; restart any that are down.

- [ ] **Step 2: Browser walkthrough**

As an approved subscriber whose app subscribes to a sandbox-enabled product (e.g. pizzashackapi):
- Applications → the app → **Identifiants**: the Sandbox card shows "Activer le sandbox"; click → the sandbox key is revealed once; the card now shows the sandbox gateway URL + "Régénérer". Régénérer reveals a new key.
- A product/detail page for that product: the Try-it shows a **Production / Sandbox** toggle; switching to Sandbox points Scalar's server at `/api/try/{slug}/{appId}/sandbox`; a request routes to the sandbox backend.
- Admin → Produits → edit a product: the **Sandbox** upstream field is present, prefilled, and saving persists it.
- **Look at the screenshot.**

---

## Self-Review notes

- **Spec coverage:** client fns + types (T1) ✅; Credentials Sandbox card with enable/rotate/base-URL, eligibility-gated (T2) ✅; product-detail Try-it prod/sandbox toggle (T3) ✅; admin Composer sandbox upstream field (T4) ✅; live (T5) ✅. NEW-1 from Plan 1's final review (gating `sandboxAvailable` display): handled implicitly — the Sandbox card is gated on `sandboxEligible` (a sub being sandboxAvailable) AND, for the enabled state, on `sandboxEnabled`/`sandboxGatewayUrl`, which the backend only populates when sandbox is configured; the Try-it toggle is gated on `sandboxAvailable` from the context endpoint, which IS gated on `h.sandbox != ""`. So a sandbox-disabled deployment shows no sandbox affordances.
- **Type consistency:** `enableSandbox`/`rotateSandboxKey(token, appId) → {sandboxApiKey}` consistent (T1 defines, T2 consumes); `AppDetail.sandboxEnabled?/sandboxGatewayUrl?` + `SubscriptionView.sandboxAvailable?` consistent (T1 → T2 props); `getTryContext` `sandboxAvailable?` (T1 → T3); `AdminProduct.sandboxUpstreamUrl?` (T1 → T4).
- **Implementer notes:** the sandbox key is reveal-once (NOT in the detail response) — the card shows the masked placeholder + "Régénérer pour révéler" when enabled-but-no-key-in-hand. Tests that mock `getTryContext` must include `sandboxAvailable` in the resolved value. Match each test to the real harness (ProductDetailPage already mocks ScalarDocs + getTryContext; ProductsPage has a create-composer flow).
