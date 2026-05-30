# APISIX Developer Portal — Subscribe UI (Plan 3b)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make "S'abonner" work in the browser — a logged-in developer clicks Subscribe on a catalog API, picks (or creates) an Application and a Plan, and gets their API key; an Applications page lists their apps with keys and subscriptions, and lets them unsubscribe.

**Architecture:** One small backend addition (an authenticated app-detail endpoint that returns an app's credential + its subscriptions) plus React UI built on the existing `web/` app (Plan 2). The API client gains authenticated functions (Bearer token from `useAuth`). A `SubscribeModal` drives the subscribe flow from the catalog; an `ApplicationsPage` manages apps/keys/subscriptions. Component tests mock the network.

**Tech Stack:** Go (existing backend packages) + React 19 / TS / Vitest (existing `web/`).

This is Plan 3b (follow-on to Plan 3). Backend wiring and the subscribe loop already work (Plan 3); the seeded products have `echo:8080` upstreams so subscribe succeeds end-to-end. Spec: `docs/superpowers/specs/2026-05-29-apisix-developer-portal-design.md`.

---

## Task 1 (backend): app-detail read endpoint (credential + subscriptions) — TDD

**Files:** `internal/subscriptions/view.go`, `internal/subscriptions/repo.go` (add methods), `internal/subscriptions/handler.go` (add route + reader), `internal/subscriptions/handler_test.go` (add tests). Wire in `cmd/portal/main.go`.

- [ ] **Step 1: Create `internal/subscriptions/view.go`**

```go
package subscriptions

// SubscriptionView is one of an application's active subscriptions, enriched
// with the product and plan names for display.
type SubscriptionView struct {
	ProductID   int64  `json:"productId"`
	ProductName string `json:"productName"`
	Version     string `json:"version"`
	ContextPath string `json:"contextPath"`
	PlanID      int64  `json:"planId"`
	PlanName    string `json:"planName"`
}

// AppDetail is the response for GET /api/applications/{id}: the app's gateway
// key (empty until it has at least one subscription) and its subscriptions.
type AppDetail struct {
	APIKey           string             `json:"apiKey"`
	ConsumerUsername string             `json:"consumerUsername"`
	Subscriptions    []SubscriptionView `json:"subscriptions"`
}
```

- [ ] **Step 2: Add reader methods to `internal/subscriptions/repo.go`** (append)

```go
// GetCredential returns the application's credential, or ErrNotFound if it has none yet.
func (r *Repo) GetCredential(ctx context.Context, appID int64) (Credential, error) {
	var c Credential
	err := r.pool.QueryRow(ctx,
		`SELECT application_id, api_key, consumer_username FROM credentials WHERE application_id=$1`, appID,
	).Scan(&c.ApplicationID, &c.APIKey, &c.ConsumerUsername)
	if errors.Is(err, pgx.ErrNoRows) {
		return Credential{}, ErrNotFound
	}
	return c, err
}

// SubscriptionsForApp returns the application's active subscriptions for display.
func (r *Repo) SubscriptionsForApp(ctx context.Context, appID int64) ([]SubscriptionView, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT s.api_product_id, p.name, p.version, p.context_path, s.plan_id, pl.name
		 FROM subscriptions s
		 JOIN api_products p ON p.id = s.api_product_id
		 JOIN plans pl ON pl.id = s.plan_id
		 WHERE s.application_id=$1 AND s.status='active'
		 ORDER BY p.name`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SubscriptionView
	for rows.Next() {
		var v SubscriptionView
		if err := rows.Scan(&v.ProductID, &v.ProductName, &v.Version, &v.ContextPath, &v.PlanID, &v.PlanName); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
```

- [ ] **Step 3: Extend the handler — add a `Reader` and the detail route in `internal/subscriptions/handler.go`**

Add the interface and field; update `NewHandler` signature and add the route + handler:

```go
// Reader is the read surface for app detail (satisfied by *Repo).
type Reader interface {
	GetCredential(ctx context.Context, appID int64) (Credential, error)
	SubscriptionsForApp(ctx context.Context, appID int64) ([]SubscriptionView, error)
}
```
Change `Handler` to hold `reader Reader`, change `NewHandler(svc *Service, reader Reader, owns OwnerCheck) *Handler`, and register `h.router.Get("/api/applications/{appID}", h.detail)`. Add:
```go
func (h *Handler) detail(w http.ResponseWriter, r *http.Request) {
	appID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	out := AppDetail{Subscriptions: []SubscriptionView{}}
	cred, err := h.reader.GetCredential(r.Context(), appID)
	if err == nil {
		out.APIKey = cred.APIKey
		out.ConsumerUsername = cred.ConsumerUsername
	} else if err != ErrNotFound {
		httpx.Error(w, http.StatusInternalServerError, "failed to load credential")
		return
	}
	subs, err := h.reader.SubscriptionsForApp(r.Context(), appID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to load subscriptions")
		return
	}
	if subs != nil {
		out.Subscriptions = subs
	}
	httpx.JSON(w, http.StatusOK, out)
}
```

- [ ] **Step 4: Update `internal/subscriptions/handler_test.go`** — the existing `newTestHandler` must pass a reader. Add a fake reader and a detail test:

```go
type fakeReader struct {
	cred Credential
	has  bool
	subs []SubscriptionView
}

func (f fakeReader) GetCredential(_ context.Context, _ int64) (Credential, error) {
	if !f.has {
		return Credential{}, ErrNotFound
	}
	return f.cred, nil
}
func (f fakeReader) SubscriptionsForApp(_ context.Context, _ int64) ([]SubscriptionView, error) {
	return f.subs, nil
}
```
Update `newTestHandler` to build the handler with a reader, e.g.:
```go
func newTestHandler() (*Handler, *apisix.Fake) {
	store := newMemStore()
	gw := apisix.NewFake()
	svc := NewService(store, gw, func() string { return "key-xyz" })
	owns := func(_ context.Context, appID, userID int64) (bool, error) { return appID == 1 && userID == 5, nil }
	reader := fakeReader{has: true, cred: Credential{ApplicationID: 1, APIKey: "key-xyz", ConsumerUsername: "app_1"},
		subs: []SubscriptionView{{ProductID: 3, ProductName: "PizzaShackAPI", PlanID: 2, PlanName: "Silver"}}}
	return NewHandler(svc, reader, owns), gw
}
```
Add the test:
```go
func TestAppDetailReturnsKeyAndSubscriptions(t *testing.T) {
	h, _ := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/applications/1", nil)
	req = req.WithContext(auth.WithUserID(req.Context(), 5))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"apiKey":"key-xyz"`) || !strings.Contains(body, `"productName":"PizzaShackAPI"`) {
		t.Fatalf("unexpected detail body: %s", body)
	}
}

func TestAppDetailRejectsNonOwner(t *testing.T) {
	h, _ := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/applications/1", nil)
	req = req.WithContext(auth.WithUserID(req.Context(), 999))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d want 403", rec.Code)
	}
}
```

- [ ] **Step 5: Wire the reader in `cmd/portal/main.go`** — the `subscriptions.NewRepo(pool)` already implements `Reader`; pass it to `NewHandler`:
```go
	subRepo := subscriptions.NewRepo(pool)
	subSvc := subscriptions.NewService(subRepo, gw, subscriptions.GenerateKey)
	subH := subscriptions.NewHandler(subSvc, subRepo, owns)
```
(Replace the existing two lines that build `subSvc`/`subH`.) Add `var _ Reader = (*Repo)(nil)` at the bottom of repo.go.

- [ ] **Step 6: Run + commit** — `go build ./...`, `go vet ./...`, `go test ./internal/... -v` all green (handler tests incl. the 2 new). Then:
```bash
git add internal/subscriptions/view.go internal/subscriptions/repo.go internal/subscriptions/handler.go internal/subscriptions/handler_test.go cmd/portal/main.go
git commit -m "feat: GET /api/applications/{id} app detail (key + subscriptions) (TDD)"
```

---

## Task 2 (frontend): authenticated API client functions — TDD

**Files:** `web/src/api/types.ts` (extend), `web/src/api/client.ts` (extend), `web/src/api/client.test.ts` (extend).

- [ ] **Step 1: Add types to `web/src/api/types.ts`**

```ts
export interface Plan {
  id: number
  name: string
  rateLimit: number
  windowSeconds: number
}

export interface Application {
  id: number
  ownerId: number
  name: string
  description: string
  createdAt: string
}

export interface Credential {
  applicationId: number
  apiKey: string
  consumerUsername: string
}

export interface SubscriptionView {
  productId: number
  productName: string
  version: string
  contextPath: string
  planId: number
  planName: string
}

export interface AppDetail {
  apiKey: string
  consumerUsername: string
  subscriptions: SubscriptionView[]
}
```

- [ ] **Step 2: Add failing tests to `web/src/api/client.test.ts`** (append inside the file)

```ts
import { getPlans, getApplications, createApplication, getApplicationDetail, subscribe } from './client'

describe('authenticated endpoints', () => {
  it('getApplications sends the Bearer token', async () => {
    mockFetch(200, [{ id: 1, name: 'A', ownerId: 5, description: '', createdAt: '' }])
    const out = await getApplications('tok-1')
    expect(out).toHaveLength(1)
    const [url, opts] = (globalThis.fetch as any).mock.calls[0]
    expect(url).toBe('/api/applications')
    expect(opts.headers.Authorization).toBe('Bearer tok-1')
  })

  it('createApplication POSTs name with auth', async () => {
    mockFetch(201, { id: 9, name: 'New', ownerId: 5, description: '', createdAt: '' })
    const a = await createApplication('tok', 'New', '')
    expect(a.id).toBe(9)
    const [url, opts] = (globalThis.fetch as any).mock.calls[0]
    expect(url).toBe('/api/applications')
    expect(opts.method).toBe('POST')
    expect(opts.headers.Authorization).toBe('Bearer tok')
  })

  it('subscribe POSTs productId+planId to the app subscriptions URL', async () => {
    mockFetch(201, { applicationId: 9, apiKey: 'k', consumerUsername: 'app_9' })
    const cred = await subscribe('tok', 9, 3, 2)
    expect(cred.apiKey).toBe('k')
    const [url, opts] = (globalThis.fetch as any).mock.calls[0]
    expect(url).toBe('/api/applications/9/subscriptions')
    expect(JSON.parse(opts.body)).toEqual({ productId: 3, planId: 2 })
  })

  it('getApplicationDetail GETs the detail with auth', async () => {
    mockFetch(200, { apiKey: 'k', consumerUsername: 'app_9', subscriptions: [] })
    const d = await getApplicationDetail('tok', 9)
    expect(d.apiKey).toBe('k')
    expect((globalThis.fetch as any).mock.calls[0][0]).toBe('/api/applications/9')
  })

  it('getPlans returns the array', async () => {
    mockFetch(200, [{ id: 1, name: 'Free', rateLimit: 60, windowSeconds: 60 }])
    expect(await getPlans()).toHaveLength(1)
  })
})
```

- [ ] **Step 3: Run → fails. Then add to `web/src/api/client.ts`:**

```ts
import type { Product, AuthResponse, ProductQuery, Plan, Application, Credential, AppDetail } from './types'

function authHeaders(token: string): HeadersInit {
  return { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` }
}

export async function getPlans(): Promise<Plan[]> {
  return parse<Plan[]>(await fetch('/api/plans'))
}

export async function getApplications(token: string): Promise<Application[]> {
  return parse<Application[]>(await fetch('/api/applications', { headers: authHeaders(token) }))
}

export async function createApplication(token: string, name: string, description: string): Promise<Application> {
  return parse<Application>(await fetch('/api/applications', {
    method: 'POST', headers: authHeaders(token), body: JSON.stringify({ name, description }),
  }))
}

export async function getApplicationDetail(token: string, appId: number): Promise<AppDetail> {
  return parse<AppDetail>(await fetch(`/api/applications/${appId}`, { headers: authHeaders(token) }))
}

export async function subscribe(token: string, appId: number, productId: number, planId: number): Promise<Credential> {
  return parse<Credential>(await fetch(`/api/applications/${appId}/subscriptions`, {
    method: 'POST', headers: authHeaders(token), body: JSON.stringify({ productId, planId }),
  }))
}

export async function unsubscribe(token: string, appId: number, productId: number): Promise<void> {
  const res = await fetch(`/api/applications/${appId}/subscriptions/${productId}`, {
    method: 'DELETE', headers: authHeaders(token),
  })
  if (!res.ok) throw new Error(`unsubscribe failed (${res.status})`)
}
```
(Merge the import line with the existing one in client.ts — don't duplicate the `parse`/`getProducts` code.)

- [ ] **Step 4: Run → passes** (`cd web && npx vitest run src/api/client.test.ts`). Build clean. Commit:
```bash
git add web/src/api/types.ts web/src/api/client.ts web/src/api/client.test.ts
git commit -m "feat(web): authenticated API client (plans, applications, subscribe) (TDD)"
```

---

## Task 3 (frontend): SubscribeModal — TDD

**Files:** `web/src/components/SubscribeModal.tsx`, `web/src/components/SubscribeModal.test.tsx`, append modal CSS to `web/src/styles/base.css`.

- [ ] **Step 1: Write `web/src/components/SubscribeModal.test.tsx`**

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SubscribeModal } from './SubscribeModal'
import { AuthProvider } from '../auth/AuthProvider'
import * as api from '../api/client'
import type { Product } from '../api/types'

const product: Product = { id: 3, name: 'PizzaShackAPI', slug: 'pizzashackapi', category: 'Engineering', version: '1.0.0', contextPath: '/pizzashack', description: 'demo', tags: [], icon: 'pi', rating: 4.5 }

beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('token', 'tok')
  localStorage.setItem('user', JSON.stringify({ id: 5, email: 'a@b.c', name: '', role: 'developer' }))
  vi.restoreAllMocks()
})

function renderModal() {
  return render(<AuthProvider><SubscribeModal product={product} onClose={() => {}} /></AuthProvider>)
}

describe('SubscribeModal', () => {
  it('loads apps + plans, subscribes, and shows the issued key', async () => {
    vi.spyOn(api, 'getApplications').mockResolvedValue([{ id: 9, name: 'My App', ownerId: 5, description: '', createdAt: '' }])
    vi.spyOn(api, 'getPlans').mockResolvedValue([{ id: 2, name: 'Silver', rateLimit: 300, windowSeconds: 60 }])
    const sub = vi.spyOn(api, 'subscribe').mockResolvedValue({ applicationId: 9, apiKey: 'SECRET-KEY', consumerUsername: 'app_9' })

    renderModal()
    await waitFor(() => expect(screen.getByText('My App')).toBeInTheDocument())
    await userEvent.click(screen.getByRole('button', { name: /s'abonner|subscribe|confirmer/i }))

    await waitFor(() => expect(screen.getByText('SECRET-KEY')).toBeInTheDocument())
    expect(sub).toHaveBeenCalledWith('tok', 9, 3, 2)
  })

  it('shows the server error when subscribe fails', async () => {
    vi.spyOn(api, 'getApplications').mockResolvedValue([{ id: 9, name: 'My App', ownerId: 5, description: '', createdAt: '' }])
    vi.spyOn(api, 'getPlans').mockResolvedValue([{ id: 2, name: 'Silver', rateLimit: 300, windowSeconds: 60 }])
    vi.spyOn(api, 'subscribe').mockRejectedValue(new Error('provisioning failed'))
    renderModal()
    await waitFor(() => expect(screen.getByText('My App')).toBeInTheDocument())
    await userEvent.click(screen.getByRole('button', { name: /s'abonner|subscribe|confirmer/i }))
    await waitFor(() => expect(screen.getByText('provisioning failed')).toBeInTheDocument())
  })
})
```

- [ ] **Step 2: Run → fails. Then write `web/src/components/SubscribeModal.tsx`:**

```tsx
import { useEffect, useState } from 'react'
import type { Product, Application, Plan } from '../api/types'
import { getApplications, getPlans, createApplication, subscribe } from '../api/client'
import { useAuth } from '../auth/AuthProvider'

export function SubscribeModal({ product, onClose }: { product: Product; onClose: () => void }) {
  const { token } = useAuth()
  const [apps, setApps] = useState<Application[]>([])
  const [plans, setPlans] = useState<Plan[]>([])
  const [appId, setAppId] = useState<number | 'new'>('new')
  const [newName, setNewName] = useState('')
  const [planId, setPlanId] = useState<number | null>(null)
  const [apiKey, setApiKey] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (!token) return
    Promise.all([getApplications(token), getPlans()])
      .then(([a, p]) => {
        setApps(a); setPlans(p)
        if (a.length) setAppId(a[0].id)
        if (p.length) setPlanId(p[0].id)
      })
      .catch(() => setErr('Impossible de charger les applications et les plans.'))
  }, [token])

  async function onSubmit() {
    if (!token || planId == null) return
    setErr(''); setBusy(true)
    try {
      let targetApp = appId
      if (targetApp === 'new') {
        const created = await createApplication(token, newName || 'Mon application', '')
        targetApp = created.id
      }
      const cred = await subscribe(token, targetApp as number, product.id, planId)
      setApiKey(cred.apiKey)
    } catch (e) {
      setErr(e instanceof Error ? e.message : "Échec de l'abonnement")
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={e => e.stopPropagation()} role="dialog" aria-label="S'abonner">
        <h2>S'abonner à {product.name}</h2>
        {apiKey ? (
          <div className="keybox">
            <p className="rescount">Votre clé d'API (copiez-la, elle ne sera plus affichée intégralement) :</p>
            <code className="apikey">{apiKey}</code>
            <button className="subbtn" onClick={() => navigator.clipboard?.writeText(apiKey)}>Copier</button>
            <button className="subbtn ghost" onClick={onClose}>Fermer</button>
          </div>
        ) : (
          <>
            <label>Application
              <select value={String(appId)} onChange={e => setAppId(e.target.value === 'new' ? 'new' : Number(e.target.value))} aria-label="Application">
                {apps.map(a => <option key={a.id} value={a.id}>{a.name}</option>)}
                <option value="new">+ Nouvelle application</option>
              </select>
            </label>
            {appId === 'new' && (
              <label>Nom de l'application
                <input value={newName} onChange={e => setNewName(e.target.value)} placeholder="Mon application" aria-label="Nom de l'application" />
              </label>
            )}
            <label>Plan
              <select value={planId ?? ''} onChange={e => setPlanId(Number(e.target.value))} aria-label="Plan">
                {plans.map(p => <option key={p.id} value={p.id}>{p.name} — {p.rateLimit}/{p.windowSeconds}s</option>)}
              </select>
            </label>
            {err && <p className="autherr" role="alert">{err}</p>}
            <div className="modal-actions">
              <button className="subbtn ghost" onClick={onClose}>Annuler</button>
              <button className="subbtn" onClick={onSubmit} disabled={busy || planId == null}>{busy ? '…' : "Confirmer l'abonnement"}</button>
            </div>
          </>
        )}
      </div>
    </div>
  )
}
```

- [ ] **Step 3: Append modal CSS to `web/src/styles/base.css`:**

```css
.modal-backdrop{position:fixed;inset:0;background:oklch(20% 0.03 262 /.5);backdrop-filter:blur(3px);display:grid;place-items:center;z-index:100}
.modal{background:var(--surface);border:1px solid var(--border);border-radius:var(--r);padding:26px;width:min(440px,92vw);box-shadow:var(--shadow-h);display:flex;flex-direction:column;gap:14px}
.modal h2{font-family:var(--font-display);font-size:21px}
.modal label{display:flex;flex-direction:column;gap:6px;font-size:13px;color:var(--muted)}
.modal select,.modal input{height:40px;border:1px solid var(--border-2);border-radius:10px;background:var(--bg);padding:0 12px;font-size:14px;color:var(--fg)}
.modal-actions{display:flex;gap:10px;justify-content:flex-end;margin-top:6px}
.subbtn.ghost{background:transparent;color:var(--fg);border:1px solid var(--border-2)}
.keybox{display:flex;flex-direction:column;gap:10px}
.apikey{font-family:var(--font-mono);font-size:14px;background:var(--bg);border:1px solid var(--border-2);border-radius:8px;padding:12px;word-break:break-all;color:var(--accent-d)}
```

- [ ] **Step 4: Run → passes** (`cd web && npx vitest run src/components/SubscribeModal.test.tsx`). Build clean. Commit:
```bash
git add web/src/components/SubscribeModal.tsx web/src/components/SubscribeModal.test.tsx web/src/styles/base.css
git commit -m "feat(web): SubscribeModal — choose app+plan, subscribe, reveal key (TDD)"
```

---

## Task 4 (frontend): wire ApiCard → CatalogPage modal (login-gated)

**Files:** `web/src/components/ApiCard.tsx` (add onSubscribe prop), `web/src/pages/CatalogPage.tsx` (modal state + auth gating), update `web/src/pages/CatalogPage.test.tsx` if needed.

- [ ] **Step 1: Add an `onSubscribe` prop to `ApiCard`** — change the signature to `{ p, onSubscribe }: { p: Product; onSubscribe: (p: Product) => void }` and make the button call it: `<button className="subbtn" onClick={() => onSubscribe(p)}>S'abonner</button>`.

- [ ] **Step 2: In `CatalogPage.tsx`** import `useAuth`, `useNavigate` (from react-router-dom), and `SubscribeModal`. Add state `const [modalProduct, setModalProduct] = useState<Product | null>(null)`. Add:
```tsx
  const { user } = useAuth()
  const nav = useNavigate()
  function handleSubscribe(p: Product) {
    if (!user) { nav('/login'); return }
    setModalProduct(p)
  }
```
Pass `onSubscribe={handleSubscribe}` to each `<ApiCard>`. Render the modal: `{modalProduct && <SubscribeModal product={modalProduct} onClose={() => setModalProduct(null)} />}`.

- [ ] **Step 3: Fix the existing CatalogPage test** — its `<ApiCard>` usages now need an `onSubscribe`. The test renders `CatalogPage` (not ApiCard directly), so it should still pass; run `cd web && npx vitest run src/pages/CatalogPage.test.tsx` and confirm. If a new ApiCard unit test exists and breaks, pass a no-op `onSubscribe={() => {}}`.

- [ ] **Step 4: Add a CatalogPage test for the login gate:**
Append to `CatalogPage.test.tsx`:
```tsx
it('redirects to /login when an anonymous user clicks Subscribe', async () => {
  vi.spyOn(api, 'getProducts').mockResolvedValue(sample)
  render(
    <MemoryRouter initialEntries={['/']}>
      <ThemeProvider><AuthProvider>
        <Routes>
          <Route path="/" element={<CatalogPage />} />
          <Route path="/login" element={<div>LOGIN PAGE</div>} />
        </Routes>
      </AuthProvider></ThemeProvider>
    </MemoryRouter>
  )
  await waitFor(() => expect(screen.getAllByTestId('api-card').length).toBeGreaterThan(0))
  await userEvent.click(screen.getAllByRole('button', { name: /s'abonner/i })[0])
  await waitFor(() => expect(screen.getByText('LOGIN PAGE')).toBeInTheDocument())
})
```
Add the needed imports (`Routes`, `Route` from react-router-dom) at the top of the test file. Run → passes.

- [ ] **Step 5: Build + commit**
```bash
cd /home/walidboudiche/working/apisix-developper-portal
git add web/src/components/ApiCard.tsx web/src/pages/CatalogPage.tsx web/src/pages/CatalogPage.test.tsx
git commit -m "feat(web): Subscribe button opens modal (login-gated) from the catalog (TDD)"
```

---

## Task 5 (frontend): Applications page — TDD

**Files:** `web/src/pages/ApplicationsPage.tsx`, `web/src/pages/ApplicationsPage.test.tsx`.

- [ ] **Step 1: Write `web/src/pages/ApplicationsPage.test.tsx`**

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { ApplicationsPage } from './ApplicationsPage'
import { ThemeProvider } from '../theme/ThemeProvider'
import { AuthProvider } from '../auth/AuthProvider'
import * as api from '../api/client'

beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('token', 'tok')
  localStorage.setItem('user', JSON.stringify({ id: 5, email: 'a@b.c', name: '', role: 'developer' }))
  vi.restoreAllMocks()
})

function renderPage() {
  return render(<MemoryRouter><ThemeProvider><AuthProvider><ApplicationsPage /></AuthProvider></ThemeProvider></MemoryRouter>)
}

describe('ApplicationsPage', () => {
  it('lists the user apps and shows a selected app key + subscriptions', async () => {
    vi.spyOn(api, 'getApplications').mockResolvedValue([{ id: 9, name: 'My App', ownerId: 5, description: '', createdAt: '' }])
    vi.spyOn(api, 'getApplicationDetail').mockResolvedValue({
      apiKey: 'KEY-9', consumerUsername: 'app_9',
      subscriptions: [{ productId: 3, productName: 'PizzaShackAPI', version: '1.0.0', contextPath: '/pizzashack', planId: 2, planName: 'Silver' }],
    })
    renderPage()
    await waitFor(() => expect(screen.getByText('My App')).toBeInTheDocument())
    await waitFor(() => expect(screen.getByText('KEY-9')).toBeInTheDocument())
    expect(screen.getByText('PizzaShackAPI')).toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run → fails. Then write `web/src/pages/ApplicationsPage.tsx`:**

```tsx
import { useEffect, useState } from 'react'
import type { Application, AppDetail } from '../api/types'
import { getApplications, getApplicationDetail, createApplication, unsubscribe } from '../api/client'
import { useAuth } from '../auth/AuthProvider'
import { TopBar } from '../components/TopBar'
import '../styles/catalog.css'

export function ApplicationsPage() {
  const { token } = useAuth()
  const [apps, setApps] = useState<Application[]>([])
  const [selected, setSelected] = useState<number | null>(null)
  const [detail, setDetail] = useState<AppDetail | null>(null)
  const [newName, setNewName] = useState('')
  const [err, setErr] = useState('')

  function reloadApps() {
    if (!token) return
    getApplications(token).then(a => {
      setApps(a)
      if (a.length && selected == null) setSelected(a[0].id)
    }).catch(() => setErr('Impossible de charger les applications.'))
  }
  useEffect(reloadApps, [token])

  useEffect(() => {
    if (!token || selected == null) { setDetail(null); return }
    getApplicationDetail(token, selected).then(setDetail).catch(() => setDetail(null))
  }, [token, selected])

  async function onCreate() {
    if (!token || !newName) return
    const a = await createApplication(token, newName, '')
    setNewName(''); setSelected(a.id); reloadApps()
  }
  async function onUnsub(productId: number) {
    if (!token || selected == null) return
    await unsubscribe(token, selected, productId)
    getApplicationDetail(token, selected).then(setDetail)
  }

  return (
    <>
      <TopBar search="" onSearch={() => {}} />
      <div className="content">
        <div className="chead"><div className="titlewrap"><h1>Mes applications</h1></div></div>
        {err && <p className="autherr" role="alert">{err}</p>}
        <div style={{ display: 'flex', gap: 10, marginBottom: 18 }}>
          <input value={newName} onChange={e => setNewName(e.target.value)} placeholder="Nom de la nouvelle application" aria-label="Nom de la nouvelle application"
            style={{ height: 40, padding: '0 12px', border: '1px solid var(--border-2)', borderRadius: 10, background: 'var(--bg)', color: 'var(--fg)' }} />
          <button className="subbtn" onClick={onCreate}>Créer</button>
        </div>
        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginBottom: 18 }}>
          {apps.map(a => (
            <button key={a.id} className={`tag ${selected === a.id ? 'active' : ''}`} onClick={() => setSelected(a.id)}>{a.name}</button>
          ))}
        </div>
        {detail && (
          <div className="card" style={{ padding: 22 }}>
            <div className="cmeta"><span className="pill">Clé d'API</span></div>
            <code className="apikey">{detail.apiKey || '— aucune (abonnez une API)'}</code>
            <h3 style={{ marginTop: 18 }}>Abonnements</h3>
            {detail.subscriptions.length === 0 && <p className="rescount">Aucun abonnement.</p>}
            {detail.subscriptions.map(s => (
              <div key={s.productId} className="cfoot" style={{ justifyContent: 'space-between' }}>
                <span>{s.productName} <span className="ctx">{s.contextPath}</span> · {s.planName}</span>
                <button className="subbtn ghost" onClick={() => onUnsub(s.productId)}>Se désabonner</button>
              </div>
            ))}
          </div>
        )}
      </div>
    </>
  )
}
```

- [ ] **Step 3: Run → passes** (`cd web && npx vitest run src/pages/ApplicationsPage.test.tsx`). Commit:
```bash
git add web/src/pages/ApplicationsPage.tsx web/src/pages/ApplicationsPage.test.tsx
git commit -m "feat(web): Applications page — apps, keys, subscriptions, unsubscribe (TDD)"
```

---

## Task 6 (frontend): nav link + route + build + manual smoke

**Files:** `web/src/components/TopBar.tsx`, `web/src/App.tsx`.

- [ ] **Step 1: Add an Applications nav link in `TopBar.tsx`** — in the `.nav-tabs` nav, add `<Link to="/applications">Applications</Link>` after the APIs link (only meaningful when logged in, but harmless to always show; optionally render it only when `user` is set).

- [ ] **Step 2: Add the route in `App.tsx`** — import `ApplicationsPage` and add `<Route path="/applications" element={<ApplicationsPage />} />`.

- [ ] **Step 3: Full test + build** — `cd web && npm run test && npm run build` — all suites pass, production build clean. Paste counts.

- [ ] **Step 4: Manual browser smoke** — backend up (`cd .. && make up && go run ./cmd/portal &` on :8080, or another free port) + `cd web && npm run dev`. In the browser: log in (or register), click **S'abonner** on an API → modal → pick "+ Nouvelle application", a plan, Confirmer → the **API key appears**. Visit **Applications** → the app, its key, and the subscription are listed; **Se désabonner** removes it. (Report what you observed; stop the servers after.)

- [ ] **Step 5: Commit**
```bash
cd /home/walidboudiche/working/apisix-developper-portal
git add web/src/components/TopBar.tsx web/src/App.tsx
git commit -m "feat(web): Applications nav + route; subscribe UI end-to-end"
```

---

## Self-review notes (author)

- **Spec coverage:** subscribe from the catalog with app+plan selection and key reveal (Tasks 3–4) ✓; Applications page with keys + subscriptions + unsubscribe (Tasks 1,2,5) ✓; login-gating of subscribe (Task 4) ✓; authenticated API client (Task 2) ✓; the one backend gap (no read endpoint for an app's key/subscriptions) is closed by Task 1.
- **No placeholders:** all code complete; CSS appended to existing files; reuses existing `parse`/`authHeaders` patterns.
- **Type consistency:** `Plan`/`Application`/`Credential`/`AppDetail`/`SubscriptionView` defined once in `api/types.ts` (frontend) mirroring the Go JSON shapes from Task 1; client functions take `token` and are consumed via `useAuth().token`; `subscribe(token, appId, productId, planId)` signature is identical across client, modal, and tests.
- **Deferred (by design):** key rotation, soft-delete/audit of subscriptions, and pagination — not needed for this slice.
```
