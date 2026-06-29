# OAuth2 for Consumers — Plan 2 (Frontend) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Surface OAuth2 in the UI — an admin "Auth method" selector on products, an OAuth2 credentials card where a developer enters their app's OIDC `client_id`, and an "OAuth2" badge on oauth2 products.

**Architecture:** Additive React 19 + TS work in `web/`. A new `setOidcClient` client fn (over the existing `sendAuthed`) + optional sandbox-style type fields the Go backend already returns. The admin Composer gets an `authType` select; the Credentials tab gets an OAuth2 card gated on `oauthEligible`; the catalog card + product detail get an OAuth2 badge.

**Tech Stack:** React 19 + TypeScript, Vite, vitest, pnpm.

## Global Constraints

- All commands run from `web/` with pnpm. French copy; reuse Atlas CSS tokens + existing `.keycard`/`.keygrid`/`.pill` styles.
- New sandbox-style type fields are **optional** (`authType?`, `oidcClientId?`, `oauthEligible?`, `oidcIssuer?`) so the ~10 existing fixtures don't churn; components default them. The backend always sends them.
- Backend contract (shipped, Plan 1): `GET /api/applications/{id}` returns `oidcClientId: string`, `oauthEligible: bool`, `oidcIssuer: string`. `PUT /api/applications/{id}/oidc-client` `{ "clientId": "..." }` → 200 (no body) / 400 on bad charset. Admin product create/update accept `authType` (`key-auth`|`oauth2`) and return 400 (`"OAuth2 is not configured on this portal"`) for oauth2 when OIDC is unconfigured. Catalog/admin product reads expose `authType`.
- The client_id is **not a secret** (plaintext input, no reveal/mask). The portal never asks for the client secret.
- Gate: `cd web && pnpm exec vitest run --exclude 'e2e/**' && pnpm exec tsc --noEmit && pnpm build` — green per task.

---

## Task 1: Types + setOidcClient client fn

**Files:**
- Modify: `web/src/api/types.ts`
- Modify: `web/src/api/client.ts`
- Test: `web/src/api/client.oauth.test.ts` (new)

**Interfaces:**
- Produces: `Product.authType?: string`, `AdminProduct.authType?: string`, `AppDetail.oidcClientId?: string`/`oauthEligible?: boolean`/`oidcIssuer?: string`; `setOidcClient(token: string, appId: number, clientId: string): Promise<void>`.

- [ ] **Step 1: Write the failing client test**

Create `web/src/api/client.oauth.test.ts`:
```ts
import { it, expect, vi, afterEach } from 'vitest'
import { setOidcClient } from './client'

afterEach(() => vi.restoreAllMocks())

it('setOidcClient PUTs the oidc-client endpoint with auth + body', async () => {
  const f = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(null, { status: 200 }))
  await setOidcClient('jwt', 7, 'client-abc')
  const [url, init] = f.mock.calls[0]
  expect(url).toBe('/api/applications/7/oidc-client')
  expect((init as RequestInit).method).toBe('PUT')
  expect((init as RequestInit).headers).toMatchObject({ Authorization: 'Bearer jwt' })
  expect(JSON.parse((init as RequestInit).body as string)).toEqual({ clientId: 'client-abc' })
})

it('setOidcClient throws on a 400 (bad client id)', async () => {
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(
    new Response(JSON.stringify({ error: 'invalid client id' }), { status: 400, headers: { 'Content-Type': 'application/json' } }))
  await expect(setOidcClient('jwt', 7, 'bad id')).rejects.toThrow(/invalid client id/i)
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && pnpm exec vitest run src/api/client.oauth.test.ts`
Expected: FAIL — `setOidcClient` not exported.

- [ ] **Step 3: Add the type fields**

In `web/src/api/types.ts`:
- In `Product` (after `ratingCount`): `authType?: string`.
- In `AdminProduct` (after `sandboxUpstreamUrl?`): `authType?: string`.
- In `AppDetail` (after `sandboxGatewayUrl?`): `oidcClientId?: string`, `oauthEligible?: boolean`, `oidcIssuer?: string`.

- [ ] **Step 4: Add the client fn**

In `web/src/api/client.ts`, add near `enableSandbox` (reuse the existing private `sendAuthed` helper, which throws the backend error message on non-2xx):
```ts
export async function setOidcClient(token: string, appId: number, clientId: string): Promise<void> {
  return sendAuthed('PUT', `/api/applications/${appId}/oidc-client`, token, { clientId })
}
```

- [ ] **Step 5: Run to verify it passes + gate**

Run: `cd web && pnpm exec vitest run src/api/client.oauth.test.ts && pnpm exec tsc --noEmit && pnpm build`
Expected: all green.

- [ ] **Step 6: Commit**

```bash
git add web/src/api/types.ts web/src/api/client.ts web/src/api/client.oauth.test.ts
git commit -m "feat(web): oauth2 client fn + types"
```

---

## Task 2: Admin Composer auth-method selector

**Files:**
- Modify: `web/src/pages/admin/ProductsPage.tsx`
- Test: `web/src/pages/admin/ProductsPage.test.tsx`

**Interfaces:**
- Consumes: `AdminProduct.authType` (Task 1).
- Produces: the Composer persists `authType` through create + update; edit/import prefill it.

**Note:** the selector is shown unconditionally (both options always available). The portal exposes no "is OIDC configured" flag to the admin client; if an admin picks **OAuth2** on a portal without OIDC configured, the backend returns the 400 `"OAuth2 is not configured on this portal"`, which the Composer already surfaces via its inline error alert. This avoids adding a config-exposure endpoint (YAGNI).

- [ ] **Step 1: Write the failing test**

In `web/src/pages/admin/ProductsPage.test.tsx` add (mirror the existing "creates a product" test — open the composer, type Nom='OrdersAPI', submit "Créer le produit"; `adminCreateProduct(token, product)` → payload is arg index 1):
```tsx
it('sends authType when creating a product', async () => {
  const create = vi.spyOn(api, 'adminCreateProduct').mockResolvedValue(products[0])
  setup() // mirror the existing create test's setup()
  await userEvent.click(screen.getByRole('button', { name: /Nouveau produit/ }))
  await userEvent.type(screen.getByLabelText('Nom'), 'OrdersAPI')
  await userEvent.selectOptions(screen.getByLabelText(/Méthode d.authentification/i), 'oauth2')
  await userEvent.click(screen.getByRole('button', { name: /Créer le produit/ }))
  await waitFor(() => expect(create).toHaveBeenCalled())
  expect(create.mock.calls[0][1]).toMatchObject({ authType: 'oauth2' })
})
```
(Adapt `setup()`/`products` to the file's real helpers — they exist from the sandbox/admin tests.)

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && pnpm exec vitest run src/pages/admin/ProductsPage.test.tsx`
Expected: FAIL — no auth-method control.

- [ ] **Step 3: Add authType to FormState + Composer + payload**

In `web/src/pages/admin/ProductsPage.tsx`:
- `interface FormState`: add `authType: string` (next to `upstreamUrl`/`sandboxUpstreamUrl`).
- `EMPTY`: add `authType: 'key-auth'`.
- Import-draft prefill (`setForm({...})` from a draft): add `authType: draft.authType ?? 'key-auth'`.
- Edit prefill (`setForm({...})` from a product `p`): add `authType: p.authType ?? 'key-auth'`.
- Submit payload (where `upstreamUrl: form.upstreamUrl.trim()` is built): add `authType: form.authType`.
- In the Composer JSX, after the Sandbox upstream field, add a select:
```tsx
      <label htmlFor="f-auth">Méthode d’authentification</label>
      <select id="f-auth" className="ipt" value={form.authType} onChange={e => set('authType', e.target.value)}>
        <option value="key-auth">Clé API (key-auth)</option>
        <option value="oauth2">OAuth2 (OIDC)</option>
      </select>
      {form.authType === 'oauth2' && (
        <p className="fieldhint">Les routes OAuth2 valident les jetons Bearer auprès de l’émetteur OIDC configuré ; les abonnés s’authentifient avec leur propre client.</p>
      )}
```
(Use the same input class the other Composer fields use — `ipt`/`ipt mono` — so styling matches; the select uses `ipt`.)

- [ ] **Step 4: Run to verify it passes + gate**

Run: `cd web && pnpm exec vitest run --exclude 'e2e/**' && pnpm exec tsc --noEmit && pnpm build`
Expected: all green.

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/admin/ProductsPage.tsx web/src/pages/admin/ProductsPage.test.tsx
git commit -m "feat(web): admin auth-method (key-auth/OAuth2) selector"
```

---

## Task 3: OAuth2 credentials card

**Files:**
- Modify: `web/src/pages/application/CredentialsTab.tsx`
- Modify: `web/src/pages/application/AppDetailPage.tsx` (pass new props)
- Test: `web/src/pages/application/CredentialsTab.test.tsx`

**Interfaces:**
- Consumes: `setOidcClient` (Task 1); `AppDetail.oidcClientId`/`oauthEligible`/`oidcIssuer`.
- Produces: `CredentialsTab` gains props `oauthEligible?: boolean`, `oidcClientId?: string`, `oidcIssuer?: string`.

- [ ] **Step 1: Write the failing tests**

In `web/src/pages/application/CredentialsTab.test.tsx` add (the file already builds the tab via a `base` object + render; mirror it, adding the new props):
```tsx
it('shows the OAuth2 card when oauthEligible and saves the client id', async () => {
  const spy = vi.spyOn(api, 'setOidcClient').mockResolvedValue(undefined)
  render(<CredentialsTab {...base} sandboxEligible={false} oauthEligible oidcIssuer="https://idp.example" oidcClientId="" />)
  expect(screen.getByText(/https:\/\/idp.example/)).toBeInTheDocument()
  await userEvent.type(screen.getByLabelText(/Client ID/i), 'client-abc')
  await userEvent.click(screen.getByRole('button', { name: /Enregistrer/i }))
  await waitFor(() => expect(spy).toHaveBeenCalledWith('jwt', 7, 'client-abc'))
})

it('prefills the client id input from oidcClientId', () => {
  render(<CredentialsTab {...base} sandboxEligible={false} oauthEligible oidcIssuer="https://idp.example" oidcClientId="existing-client" />)
  expect((screen.getByLabelText(/Client ID/i) as HTMLInputElement).value).toBe('existing-client')
})

it('hides the OAuth2 card when not oauthEligible', () => {
  render(<CredentialsTab {...base} sandboxEligible={false} oauthEligible={false} />)
  expect(screen.queryByLabelText(/Client ID/i)).not.toBeInTheDocument()
})
```
(`base` includes `appId: 7`, `token: 'jwt'`, `notify`, `openModal`, `onRotated`, `apiKey`. Confirm its shape in the file and reuse it.)

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && pnpm exec vitest run src/pages/application/CredentialsTab.test.tsx`
Expected: FAIL — no OAuth2 card.

- [ ] **Step 3: Add the props + OAuth2 card**

In `web/src/pages/application/CredentialsTab.tsx`:
- Import the client fn: add `setOidcClient` to the existing `import { ... } from '../../api/client'`.
- Extend the props destructure + type with `oauthEligible?: boolean`, `oidcClientId?: string`, `oidcIssuer?: string`.
- Add state near the sandbox state:
```tsx
  const [clientId, setClientId] = useState(oidcClientId ?? '')
  const [oauthBusy, setOauthBusy] = useState(false)
  useEffect(() => { setClientId(oidcClientId ?? '') }, [oidcClientId])

  async function onSaveClientId() {
    if (oauthBusy) return
    setOauthBusy(true)
    try {
      await setOidcClient(token, appId, clientId.trim())
      notify('Client OIDC enregistré'); onRotated()
    } catch (e) {
      notify(e instanceof Error ? e.message : 'Échec de l’enregistrement.')
    } finally { setOauthBusy(false) }
  }
```
- After the sandbox card (still inside `.keygrid` or right below it), render the OAuth2 card only when `oauthEligible`:
```tsx
      {oauthEligible && (
        <div className="keycard oauth">
          <div className="kh"><span className="env">OAuth2 <span className="envtag">OIDC</span></span></div>
          <div className="kb">
            <label htmlFor="oidc-cid" className="oauthlabel">Client ID</label>
            <div className="keyrow">
              <input id="oidc-cid" className="ipt mono" placeholder="votre client_id OIDC"
                value={clientId} onChange={e => setClientId(e.target.value)} />
              <button className="btn btn-primary" disabled={oauthBusy} onClick={onSaveClientId}>Enregistrer</button>
            </div>
            <p className="keymeta">
              <span>Émetteur · <span className="mono">{oidcIssuer || '—'}</span></span>
              <span><span className="mono">grant_type=client_credentials</span></span>
            </p>
            <p className="keyhint">Enregistrez votre client auprès de votre fournisseur OIDC, puis collez son <span className="mono">client_id</span> ici. Le portail ne stocke jamais le secret.</p>
          </div>
        </div>
      )}
```
(If `.keygrid` is a 2-col grid and three cards crowd it, the existing CSS wraps; no new layout CSS required. Reuse `.keycard`/`.kh`/`.kb`/`.keyrow`/`.keymeta`/`.keyhint`/`.btn`/`.ipt mono`.)

- [ ] **Step 4: Wire props in AppDetailPage**

In `web/src/pages/application/AppDetailPage.tsx`, the `tab === 'creds'` `<CredentialsTab ... />` render — add:
```tsx
                oauthEligible={detail.oauthEligible}
                oidcClientId={detail.oidcClientId}
                oidcIssuer={detail.oidcIssuer}
```

- [ ] **Step 5: Run to verify it passes + gate**

Run: `cd web && pnpm exec vitest run --exclude 'e2e/**' && pnpm exec tsc --noEmit && pnpm build`
Expected: all green.

- [ ] **Step 6: Commit**

```bash
git add web/src/pages/application/CredentialsTab.tsx web/src/pages/application/CredentialsTab.test.tsx web/src/pages/application/AppDetailPage.tsx
git commit -m "feat(web): OAuth2 credentials card (client_id + issuer)"
```

---

## Task 4: OAuth2 badge on catalog card + product detail

**Files:**
- Modify: `web/src/components/ApiCard.tsx`
- Modify: `web/src/pages/ProductDetailPage.tsx`
- Test: `web/src/components/ApiCard.test.tsx`

**Interfaces:**
- Consumes: `Product.authType` (Task 1).

- [ ] **Step 1: Write the failing test**

In `web/src/components/ApiCard.test.tsx` add (cards render inside `<MemoryRouter>`; mirror the existing card tests):
```tsx
it('shows an OAuth2 badge for oauth2 products', () => {
  const p = { id: 1, name: 'X', slug: 'x', category: 'C', version: '1', contextPath: '/x', description: '', tags: [], icon: '', rating: 0, ratingCount: 0, authType: 'oauth2' }
  render(<MemoryRouter><ApiCard p={p} onSubscribe={() => {}} /></MemoryRouter>)
  expect(screen.getByText('OAuth2')).toBeInTheDocument()
})

it('shows no OAuth2 badge for key-auth products', () => {
  const p = { id: 1, name: 'X', slug: 'x', category: 'C', version: '1', contextPath: '/x', description: '', tags: [], icon: '', rating: 0, ratingCount: 0, authType: 'key-auth' }
  render(<MemoryRouter><ApiCard p={p} onSubscribe={() => {}} /></MemoryRouter>)
  expect(screen.queryByText('OAuth2')).not.toBeInTheDocument()
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && pnpm exec vitest run src/components/ApiCard.test.tsx`
Expected: FAIL — no badge.

- [ ] **Step 3: Add the badge**

In `web/src/components/ApiCard.tsx`, in the `.cmeta` pill row (next to the `v{version}` / `contextPath` pills), add:
```tsx
          {p.authType === 'oauth2' && <span className="pill oauth">OAuth2</span>}
```
Add minimal CSS to `web/src/styles/catalog.css` (near the `.pill` rules):
```css
.pill.oauth{background:var(--accent);color:#fff;font-weight:600}
```
In `web/src/pages/ProductDetailPage.tsx`, in the `<p className="sub">` header line, append an OAuth2 marker:
```tsx
{product.authType === 'oauth2' && <> · <span className="pill oauth">OAuth2</span></>}
```

- [ ] **Step 4: Run to verify it passes + gate**

Run: `cd web && pnpm exec vitest run --exclude 'e2e/**' && pnpm exec tsc --noEmit && pnpm build`
Expected: all green.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/ApiCard.tsx web/src/components/ApiCard.test.tsx web/src/pages/ProductDetailPage.tsx web/src/styles/catalog.css
git commit -m "feat(web): OAuth2 badge on catalog card + product detail"
```

---

## Task 5: Live verification

- [ ] **Step 1: Stack + portal + vite running**

The disposable `oidc-issuer` container + the branch portal (`:8090`, `OIDC_ISSUER=http://oidc-issuer:8888`) should be up from the Plan 1 live check; restart any that are down. Vite on `:5173` with `PORTAL_PROXY=http://localhost:8090`.

- [ ] **Step 2: Browser walkthrough**

- Admin → Produits → Nouveau produit (or edit one): the **Méthode d’authentification** select offers Key-auth / OAuth2; pick OAuth2, save → persists (or, on a portal without OIDC, the inline error shows "OAuth2 is not configured").
- A catalog card + the product detail for an oauth2 product show the **OAuth2** badge.
- Applications → an app with an active subscription to an oauth2 product → **Identifiants**: the OAuth2 card shows a Client ID input + the issuer + the `grant_type=client_credentials` hint; enter the test client_id, Enregistrer → saved (and, per Plan 1, the gateway route whitelists it).
- **Look at the screenshot.**

---

## Self-Review notes

- **Spec coverage:** client fn + types (T1) ✅; admin auth-method selector (T2) ✅; OAuth2 credentials card with client_id + issuer (T3) ✅; OAuth2 badge (T4) ✅; live (T5) ✅. Per-app OAuth2 rate-limiting + interactive Try-it for oauth2 deferred per spec.
- **Type consistency:** `setOidcClient(token, appId, clientId)→Promise<void>` (T1 defines, T3 consumes); `AppDetail.oidcClientId/oauthEligible/oidcIssuer` (T1 → T3 props → AppDetailPage); `AdminProduct.authType` (T1 → T2); `Product.authType` (T1 → T4).
- **Implementer notes:** the selector is always shown (no config-exposure endpoint); the 400 from the backend surfaces via the existing Composer error alert. The client_id is plaintext (not a secret) — no reveal/mask. Match each test to the real harness (ProductsPage create flow; CredentialsTab `base` object; ApiCard MemoryRouter).
