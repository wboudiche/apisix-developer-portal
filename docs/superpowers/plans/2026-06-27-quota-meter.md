# Per-App Rate-Limit Meter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show a live per-app rate-limit headroom meter ("≈ used / limit over the last window") atop the Usage tab, from existing Prometheus metrics + the app's active plan.

**Architecture:** A new `GET /api/applications/{id}/quota` (subscriptions handler, owner-scoped) composes the app's active-plan limit/window (`reader.ActivePlanForApp`) with an approximate "used in last window" from a new `metrics.Service.RequestsInWindow`. The frontend `QuotaMeter` renders the bar on the Usage tab.

**Tech Stack:** Go 1.25 (chi, Prometheus client), React 19 + TS (Vite, vitest).

## Global Constraints

- Module `apisix-portal`; metrics in `internal/metrics`, endpoint in `internal/subscriptions`.
- Per-app (per-consumer) rate meter — NOT per-product, NOT a monthly quota.
- "Used" is approximate, from Prometheus `apisix_http_status{consumer="app_N"}`; UI labels it `≈` / "approx.".
- Endpoint owner-scoped via the handler's existing `authorize`; `Cache-Control: no-store`.
- `hasQuota:false` when the app has no credential or no active subscription; `available:false` when Prometheus is unconfigured/errors (return the limit, omit a fake `used`).
- Reuse the existing `countQuery` PromQL builder + `consumerRe` guard in `metrics`.
- pnpm for the frontend.

---

## Task 1: `metrics.Service.RequestsInWindow`

**Files:**
- Modify: `internal/metrics/service.go`
- Test: `internal/metrics/service_test.go`

**Interfaces:**
- Consumes: `consumerRe`, `countQuery`, the `Querier` (`Scalar`).
- Produces: `func (s *Service) RequestsInWindow(ctx context.Context, consumer string, windowSeconds int) (int64, error)` — `sum(increase(apisix_http_status{consumer="<c>"}[<windowSeconds>s]))`, rounded; rejects a malformed consumer.

- [ ] **Step 1: Write the failing test**

In `internal/metrics/service_test.go` (use the file's existing fake `Querier`; if it doesn't record the query string, add a recording fake or extend the existing one):
```go
func TestRequestsInWindow(t *testing.T) {
	q := &recordingQuerier{scalar: 41.6}
	s := NewService(q)
	got, err := s.RequestsInWindow(context.Background(), "app_7", 60)
	if err != nil {
		t.Fatalf("RequestsInWindow: %v", err)
	}
	if got != 42 { // 41.6 rounds to 42
		t.Errorf("got %d, want 42", got)
	}
	want := `sum(increase(apisix_http_status{consumer="app_7"}[60s]))`
	if q.lastQuery != want {
		t.Errorf("query = %q, want %q", q.lastQuery, want)
	}
}

func TestRequestsInWindow_RejectsBadConsumer(t *testing.T) {
	s := NewService(&recordingQuerier{})
	if _, err := s.RequestsInWindow(context.Background(), `app"7`, 60); err == nil {
		t.Fatal("expected error for malformed consumer")
	}
}
```
NOTE: add a `recordingQuerier` if absent:
```go
type recordingQuerier struct {
	scalar    float64
	lastQuery string
}
func (r *recordingQuerier) Scalar(_ context.Context, query string) (float64, error) {
	r.lastQuery = query
	return r.scalar, nil
}
func (r *recordingQuerier) Range(_ context.Context, _ string, _, _ time.Time, _ time.Duration) ([]Sample, error) {
	return nil, nil
}
```
(If the file already has a usable fake Querier, prefer recording on it instead of adding a new type.)

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/metrics/ -run TestRequestsInWindow -v`
Expected: FAIL — `RequestsInWindow` undefined.

- [ ] **Step 3: Implement**

In `internal/metrics/service.go` (near `Usage`):
```go
// RequestsInWindow returns the approximate number of requests by the consumer
// over the last windowSeconds, from Prometheus. Used by the per-app rate-limit
// meter; it is an approximation of the gateway's limit-count counter, not the
// exact value.
func (s *Service) RequestsInWindow(ctx context.Context, consumer string, windowSeconds int) (int64, error) {
	if !consumerRe.MatchString(consumer) {
		return 0, fmt.Errorf("invalid consumer %q", consumer)
	}
	sel := fmt.Sprintf(`consumer="%s"`, consumer)
	v, err := s.q.Scalar(ctx, countQuery(sel, time.Duration(windowSeconds)*time.Second))
	if err != nil {
		return 0, err
	}
	if v < 0 {
		v = 0
	}
	return int64(v + 0.5), nil
}
```

- [ ] **Step 4: Run to verify it passes + vet**

Run: `go test ./internal/metrics/ && go vet ./internal/metrics/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/metrics/service.go internal/metrics/service_test.go
git commit -m "feat(metrics): RequestsInWindow (approx requests over a window)"
```

---

## Task 2: Quota endpoint

**Files:**
- Modify: `internal/subscriptions/handler.go` (Quota type, interface additions, route, handler)
- Test: `internal/subscriptions/handler_test.go`

**Interfaces:**
- Consumes: `reader.GetCredential`, `reader.ActivePlanForApp` (new in Reader), `usage.RequestsInWindow` (new in UsageReader), `ErrNotFound`, `ErrNoActiveSubscription`, `PlanInfo`.
- Produces:
  - `type Quota struct { HasQuota bool; Used int64; Limit int; WindowSeconds int; Available bool }` with json `hasQuota,used,limit,windowSeconds,available`.
  - `GET /api/applications/{appID}/quota` → `200 Quota`.
  - `Reader` += `ActivePlanForApp(ctx, appID) (PlanInfo, error)`; `UsageReader` += `RequestsInWindow(ctx, consumer string, windowSeconds int) (int64, error)`.

- [ ] **Step 1: Write the failing tests**

In `internal/subscriptions/handler_test.go`: the existing fakes must satisfy the extended interfaces. Extend the fake `Reader` (`fakeReader`) with `ActivePlanForApp` and the fake `UsageReader` with `RequestsInWindow`. Add fields to drive them (e.g. `fakeReader.plan PlanInfo`, `fakeReader.planErr error`; a fake usage type with `used int64` / `usedErr error`). Then:
```go
func TestQuotaHappyPath(t *testing.T) {
	h, _ := newTestHandler()
	// configure the handler's reader to return a credential + active plan, and a
	// usage reader returning a used count. (Use the file's construction; if
	// newTestHandler doesn't let you inject these, build the handler inline like
	// newTestHandler does, with fakeReader{has:true, cred:..., plan:PlanInfo{Count:1000, WindowSeconds:60}}
	// and a fakeUsage{used:612}, owns app1/user5, then call SetUsageReader.)
	req := httptest.NewRequest(http.MethodGet, "/api/applications/1/quota", nil)
	req = req.WithContext(auth.WithUserID(req.Context(), 5))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}
	var q Quota
	_ = json.Unmarshal(rec.Body.Bytes(), &q)
	if !q.HasQuota || !q.Available || q.Used != 612 || q.Limit != 1000 || q.WindowSeconds != 60 {
		t.Fatalf("quota = %+v", q)
	}
}

func TestQuotaNoActiveSubscription(t *testing.T) {
	// reader returns ErrNoActiveSubscription from ActivePlanForApp
	// → expect 200 {hasQuota:false}
}

func TestQuotaMetricsUnavailable(t *testing.T) {
	// usage reader nil (do not SetUsageReader) but credential + active plan present
	// → expect 200 {hasQuota:true, available:false, limit:1000, windowSeconds:60}
}

func TestQuotaNonOwner403(t *testing.T) {
	// userID 9, owns false → 403/404
}
```
Fill in `TestQuotaNoActiveSubscription`, `TestQuotaMetricsUnavailable`, `TestQuotaNonOwner403` with the same construction style, asserting the documented shapes. Match the EXACT fake/handler construction this file already uses (`newTestHandler`, `fakeReader`, `SetUsageReader`).

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/subscriptions/ -run TestQuota -v`
Expected: FAIL — route 404 / interface methods undefined.

- [ ] **Step 3: Add the Quota type, interface methods, route, handler**

In `internal/subscriptions/handler.go`:
```go
type Quota struct {
	HasQuota      bool  `json:"hasQuota"`
	Used          int64 `json:"used"`
	Limit         int   `json:"limit"`
	WindowSeconds int   `json:"windowSeconds"`
	Available     bool  `json:"available"`
}
```
Extend the interfaces:
```go
// in Reader:
	ActivePlanForApp(ctx context.Context, appID int64) (PlanInfo, error)
// in UsageReader:
	RequestsInWindow(ctx context.Context, consumer string, windowSeconds int) (int64, error)
```
Register the route in `NewHandler` (after `/usage`):
```go
	h.router.Get("/api/applications/{appID}/quota", h.quotaHandler)
```
Add the handler:
```go
func (h *Handler) quotaHandler(w http.ResponseWriter, r *http.Request) {
	appID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")

	cred, err := h.reader.GetCredential(r.Context(), appID)
	if errors.Is(err, ErrNotFound) {
		httpx.JSON(w, http.StatusOK, Quota{HasQuota: false})
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to load credential")
		return
	}
	plan, err := h.reader.ActivePlanForApp(r.Context(), appID)
	if errors.Is(err, ErrNoActiveSubscription) {
		httpx.JSON(w, http.StatusOK, Quota{HasQuota: false})
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to load plan")
		return
	}

	q := Quota{HasQuota: true, Limit: plan.Count, WindowSeconds: plan.WindowSeconds}
	if h.usage != nil {
		used, err := h.usage.RequestsInWindow(r.Context(), cred.ConsumerUsername, plan.WindowSeconds)
		if err != nil {
			log.Printf("quota used for app %d (consumer %s): %v", appID, cred.ConsumerUsername, err)
		} else {
			q.Used = used
			q.Available = true
		}
	}
	httpx.JSON(w, http.StatusOK, q)
}
```

- [ ] **Step 4: Run to verify they pass + full backend**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/... ./cmd/... && go vet ./...`
Expected: PASS (the real `*Repo` already implements `ActivePlanForApp`; the real `*metrics.Service` implements `RequestsInWindow` from Task 1, so server.go's `SetUsageReader(metrics.NewService(...))` still satisfies `UsageReader`).

- [ ] **Step 5: Commit**

```bash
git add internal/subscriptions/handler.go internal/subscriptions/handler_test.go
git commit -m "feat(subscriptions): GET /api/applications/{id}/quota"
```

---

## Task 3: Frontend client `getQuota` + type

**Files:**
- Modify: `web/src/api/client.ts`, `web/src/api/types.ts`
- Test: `web/src/api/client.quota.test.ts` (new)

**Interfaces:**
- Produces: `type Quota = { hasQuota: boolean; used?: number; limit?: number; windowSeconds?: number; available?: boolean }`; `getQuota(token, appId): Promise<Quota>` → `GET /api/applications/{id}/quota`.

- [ ] **Step 1: Write the failing test**

Create `web/src/api/client.quota.test.ts`:
```ts
import { it, expect, vi, afterEach } from 'vitest'
import { getQuota } from './client'

afterEach(() => vi.restoreAllMocks())

it('GETs the quota endpoint with auth', async () => {
  const body = { hasQuota: true, used: 612, limit: 1000, windowSeconds: 60, available: true }
  const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
    new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } }),
  )
  const out = await getQuota('jwt', 7)
  expect(out.used).toBe(612)
  const [url, init] = fetchMock.mock.calls[0]
  expect(url).toBe('/api/applications/7/quota')
  expect((init as RequestInit).headers).toMatchObject({ Authorization: 'Bearer jwt' })
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && pnpm exec vitest run src/api/client.quota.test.ts`
Expected: FAIL — `getQuota` not exported.

- [ ] **Step 3: Implement**

`web/src/api/types.ts`:
```ts
export interface Quota {
  hasQuota: boolean
  used?: number
  limit?: number
  windowSeconds?: number
  available?: boolean
}
```
`web/src/api/client.ts` (import `Quota` in the types import block; add near `getUsage`):
```ts
export async function getQuota(token: string, appId: number): Promise<Quota> {
  const url = `/api/applications/${appId}/quota`
  return parse<Quota>(await fetch(url, { headers: authHeaders(token) }), url)
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd web && pnpm exec vitest run src/api/client.quota.test.ts && pnpm exec tsc --noEmit`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/api/client.ts web/src/api/types.ts web/src/api/client.quota.test.ts
git commit -m "feat(web): getQuota client fn + Quota type"
```

---

## Task 4: `QuotaMeter` on the Usage tab

**Files:**
- Create: `web/src/pages/application/QuotaMeter.tsx`
- Modify: `web/src/pages/application/UsageTab.tsx`
- Create: `web/src/pages/application/QuotaMeter.test.tsx`
- Modify: `web/src/styles/appdetail.css` (meter styles)

**Interfaces:**
- Consumes: `getQuota` (Task 3), `Quota` type.
- Produces: `QuotaMeter({ token, appId }: { token: string; appId: number })` rendered atop `UsageTab`.

- [ ] **Step 1: Write the failing tests**

Create `web/src/pages/application/QuotaMeter.test.tsx`:
```tsx
import { it, expect, vi, afterEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { QuotaMeter } from './QuotaMeter'
import * as api from '../../api/client'

afterEach(() => vi.restoreAllMocks())

it('renders the bar and approx label when available', async () => {
  vi.spyOn(api, 'getQuota').mockResolvedValue({ hasQuota: true, used: 612, limit: 1000, windowSeconds: 60, available: true })
  render(<QuotaMeter token="jwt" appId={7} />)
  expect(await screen.findByText(/612/)).toBeInTheDocument()
  expect(screen.getByText(/1000/)).toBeInTheDocument()
  expect(screen.getByText(/60\s*s/)).toBeInTheDocument()
})

it('shows métriques indisponibles when not available', async () => {
  vi.spyOn(api, 'getQuota').mockResolvedValue({ hasQuota: true, limit: 1000, windowSeconds: 60, available: false })
  render(<QuotaMeter token="jwt" appId={7} />)
  expect(await screen.findByText(/indisponibles/i)).toBeInTheDocument()
})

it('renders nothing when the app has no active subscription', async () => {
  vi.spyOn(api, 'getQuota').mockResolvedValue({ hasQuota: false })
  const { container } = render(<QuotaMeter token="jwt" appId={7} />)
  await waitFor(() => expect(api.getQuota).toHaveBeenCalled())
  expect(container.textContent).toBe('')
})
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd web && pnpm exec vitest run src/pages/application/QuotaMeter.test.tsx`
Expected: FAIL — cannot find `./QuotaMeter`.

- [ ] **Step 3: Implement the component**

Create `web/src/pages/application/QuotaMeter.tsx`:
```tsx
import { useEffect, useState } from 'react'
import { getQuota } from '../../api/client'
import type { Quota } from '../../api/types'

export function QuotaMeter({ token, appId }: { token: string; appId: number }) {
  const [quota, setQuota] = useState<Quota | null>(null)
  useEffect(() => {
    let alive = true
    getQuota(token, appId).then(q => { if (alive) setQuota(q) }).catch(() => { if (alive) setQuota(null) })
    return () => { alive = false }
  }, [token, appId])

  if (!quota || !quota.hasQuota) return null

  if (!quota.available) {
    return (
      <div className="quota-meter">
        <div className="qm-row"><span className="qm-title">Débit · plan</span><span className="qm-na">métriques indisponibles</span></div>
        <div className="qm-sub">Limite {quota.limit} req / {quota.windowSeconds}s</div>
      </div>
    )
  }

  const used = quota.used ?? 0
  const limit = quota.limit ?? 0
  const pct = limit > 0 ? Math.min(100, Math.round((used / limit) * 100)) : 0
  const level = pct >= 100 ? 'danger' : pct >= 80 ? 'warn' : 'ok'
  return (
    <div className="quota-meter">
      <div className="qm-row">
        <span className="qm-title">Débit · plan</span>
        <span className="qm-count">≈ {used} / {limit}</span>
      </div>
      <div className="qm-bar"><span className={`qm-fill ${level}`} style={{ width: `${pct}%` }} /></div>
      <div className="qm-sub">sur les dernières {quota.windowSeconds}s · approx.</div>
    </div>
  )
}
```

- [ ] **Step 4: Add styles**

Append to `web/src/styles/appdetail.css`:
```css
/* Rate-limit meter (Usage tab) */
.appdetail .quota-meter{border:1px solid var(--border-2);border-radius:14px;padding:16px 18px;margin-bottom:18px;background:var(--surface)}
.appdetail .qm-row{display:flex;align-items:baseline;justify-content:space-between;gap:12px}
.appdetail .qm-title{font-size:13px;color:var(--muted)}
.appdetail .qm-count{font-family:var(--font-mono);font-size:15px;font-weight:700}
.appdetail .qm-na{font-size:13px;color:var(--muted)}
.appdetail .qm-bar{height:8px;border-radius:6px;background:var(--bg);overflow:hidden;margin:10px 0 6px}
.appdetail .qm-fill{display:block;height:100%;border-radius:6px;background:var(--accent);transition:width .3s}
.appdetail .qm-fill.warn{background:var(--warn,#d98300)}
.appdetail .qm-fill.danger{background:var(--danger,#c0341d)}
.appdetail .qm-sub{font-size:12px;color:var(--faint)}
```

- [ ] **Step 5: Render it in `UsageTab`**

In `web/src/pages/application/UsageTab.tsx`, import and render the meter above the chart:
```tsx
import { QuotaMeter } from './QuotaMeter'
```
```tsx
  return (
    <section className="panel">
      <QuotaMeter token={token} appId={appId} />
      <UsageChart state={usage} range={range} onRange={setRange} />
    </section>
  )
```

- [ ] **Step 6: Run tests + full gate**

Run: `cd web && pnpm exec vitest run --exclude 'e2e/**' && pnpm exec tsc --noEmit && pnpm build`
Expected: all green.

- [ ] **Step 7: Commit**

```bash
git add web/src/pages/application/QuotaMeter.tsx web/src/pages/application/QuotaMeter.test.tsx web/src/pages/application/UsageTab.tsx web/src/styles/appdetail.css
git commit -m "feat(web): QuotaMeter rate-limit bar on the Usage tab"
```

---

## Task 5: Live verification

- [ ] **Step 1: Stack + portal + vite running**

Dev `docker compose` up (postgres + apisix + echo + prometheus); portal on `:8090` (with `APISIX_GATEWAY_URL` default); Vite on `:5173`.

- [ ] **Step 2: Generate traffic, then read the quota endpoint**

Reuse the try-it/echo product's approved app (has a consumer + plan). Send several gateway requests with the app's key, then:
```bash
curl -s http://localhost:8090/api/applications/<APPID>/quota -H "Authorization: Bearer <DEV_TOKEN>"
```
Expected: `{"hasQuota":true,"used":<N>,"limit":<plan count>,"windowSeconds":<plan window>,"available":true}` with `used` reflecting recent requests (Prometheus scrape lag of a few seconds is normal).

- [ ] **Step 3: Browser**

Open the app's **Usage** tab: the meter shows the bar + `≈ used / limit` + "sur les dernières Ns · approx.". For an app with no active subscription, the meter is absent. **Look at the screenshot.**

---

## Self-Review notes

- **Spec coverage:** per-app meter from ActivePlanForApp + RequestsInWindow (T1+T2) ✅; endpoint owner-scoped + no-store (T2) ✅; hasQuota:false no-cred/no-active-sub (T2) ✅; available:false when usage nil/errors (T2) ✅; approximate label + warning/danger colors (T4) ✅; placement atop Usage tab (T4) ✅; tests Go+vitest+live ✅.
- **Type consistency:** `RequestsInWindow(consumer, windowSeconds) → int64`, `Quota{HasQuota,Used,Limit,WindowSeconds,Available}` ↔ TS `Quota`, `getQuota(token,appId)`, `QuotaMeter({token,appId})` consistent across tasks.
- **Implementer notes:** adding `ActivePlanForApp` to `Reader` and `RequestsInWindow` to `UsageReader` requires the handler-test fakes to implement them — extend the existing `fakeReader` / fake usage reader in `handler_test.go`. The real `*subscriptions.Repo` already has `ActivePlanForApp`; the real `*metrics.Service` gets `RequestsInWindow` in Task 1 — so server.go wiring stays valid with no change.
