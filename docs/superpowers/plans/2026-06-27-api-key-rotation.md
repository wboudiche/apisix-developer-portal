# Real API Key Rotation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the Credentials-tab "Régénérer" button real: rotate an application's key-auth key server-side via APISIX (immediate revoke of the old key), and remove the demo Sandbox card.

**Architecture:** A new `Service.RotateKey(appID)` generates a fresh key, installs it on the app's APISIX consumer (`EnsureConsumer` PUT — old key 401s instantly), then persists the encrypted key; gateway-before-DB so a gateway failure leaves the old key live. The rate limit is re-derived from the app's most-recent active subscription plan. A new `POST /api/applications/{id}/credentials/rotate` route (owner-scoped) exposes it. The frontend drops the demo Sandbox card and wires the Production card's rotate to the real endpoint.

**Tech Stack:** Go 1.25 (chi, pgx), React 19 + TS (Vite, vitest).

## Global Constraints

- Module `apisix-portal`; subscriptions code in `internal/subscriptions`, events in `internal/events`.
- One key-auth key per app consumer — **immediate revoke** (no grace window).
- Rotation preserves the app's limit, re-derived from its **most-recent active** subscription's plan.
- Gateway call BEFORE the DB write (fail toward "gateway ahead of DB", which a retry reconverges).
- Keys are encrypted at rest via the repo's `cipher` (mirror `GetOrCreateCredential`).
- Endpoint is owner-scoped via the handler's existing `authorize` (non-owner → 403/404).
- 409 when the app has no credential or no active subscription.
- Frontend: pnpm; French copy; remove the demo Sandbox card entirely.

---

## Task 1: `Service.RotateKey` + Store interface + event kind

**Files:**
- Modify: `internal/events/events.go` (add `KindKeyRotated`)
- Modify: `internal/subscriptions/service.go` (RotateKey, sentinels, Store interface)
- Test: `internal/subscriptions/service_test.go`

**Interfaces:**
- Consumes: `apisix.RateLimit`, `events.KindKeyRotated`, `ErrNotFound`.
- Produces:
  - `var ErrNoCredential = errors.New("subscriptions: application has no credential")`
  - `var ErrNoActiveSubscription = errors.New("subscriptions: application has no active subscription")`
  - Store interface gains: `GetCredential(ctx, appID) (Credential, error)`, `ActivePlanForApp(ctx, appID) (PlanInfo, error)`, `UpdateCredentialKey(ctx, appID int64, newKey string) error`.
  - `func (s *Service) RotateKey(ctx context.Context, appID int64) (string, error)` — returns the new key.

- [ ] **Step 1: Add the event kind**

In `internal/events/events.go`, add to the Kind constants block (after `KindUnsubscribed`):
```go
	KindKeyRotated   = "key_rotated"
```

- [ ] **Step 2: Write the failing service test**

In `internal/subscriptions/service_test.go`, first extend the existing fake Store used by service tests with the three new methods (find the fake struct that implements `Store` and add):
```go
func (f *fakeStore) GetCredential(_ context.Context, appID int64) (Credential, error) {
	if f.cred == (Credential{}) {
		return Credential{}, ErrNotFound
	}
	return f.cred, nil
}
func (f *fakeStore) ActivePlanForApp(_ context.Context, _ int64) (PlanInfo, error) {
	if f.activePlan == (PlanInfo{}) {
		return PlanInfo{}, ErrNoActiveSubscription
	}
	return f.activePlan, nil
}
func (f *fakeStore) UpdateCredentialKey(_ context.Context, _ int64, newKey string) error {
	f.updatedKey = newKey
	return f.updateErr
}
```
Add fields `cred Credential`, `activePlan PlanInfo`, `updatedKey string`, `updateErr error` to the fake struct (match its existing name/shape). Then add the tests:
```go
func TestRotateKey_HappyPath(t *testing.T) {
	gw := apisix.NewFake()
	store := &fakeStore{
		cred:       Credential{ApplicationID: 1, APIKey: "old", ConsumerUsername: "app_1"},
		activePlan: PlanInfo{ID: 3, Count: 100, WindowSeconds: 60},
	}
	svc := NewService(store, gw, func() string { return "newkey123" }, nil)

	got, err := svc.RotateKey(context.Background(), 1)
	if err != nil || got != "newkey123" {
		t.Fatalf("RotateKey = %q, %v", got, err)
	}
	if store.updatedKey != "newkey123" {
		t.Errorf("DB not updated with new key: %q", store.updatedKey)
	}
	if k := gw.ConsumerKey("app_1"); k != "newkey123" {
		t.Errorf("gateway consumer key = %q, want newkey123", k)
	}
}

func TestRotateKey_NoCredential(t *testing.T) {
	svc := NewService(&fakeStore{}, apisix.NewFake(), func() string { return "x" }, nil)
	if _, err := svc.RotateKey(context.Background(), 1); !errors.Is(err, ErrNoCredential) {
		t.Fatalf("want ErrNoCredential, got %v", err)
	}
}

func TestRotateKey_NoActiveSubscription(t *testing.T) {
	store := &fakeStore{cred: Credential{ApplicationID: 1, APIKey: "old", ConsumerUsername: "app_1"}}
	svc := NewService(store, apisix.NewFake(), func() string { return "x" }, nil)
	if _, err := svc.RotateKey(context.Background(), 1); !errors.Is(err, ErrNoActiveSubscription) {
		t.Fatalf("want ErrNoActiveSubscription, got %v", err)
	}
}

func TestRotateKey_GatewayFailureKeepsOldKey(t *testing.T) {
	gw := apisix.NewFake()
	gw.FailEnsureConsumer = true // see NOTE below
	store := &fakeStore{
		cred:       Credential{ApplicationID: 1, APIKey: "old", ConsumerUsername: "app_1"},
		activePlan: PlanInfo{ID: 3, Count: 100, WindowSeconds: 60},
	}
	svc := NewService(store, gw, func() string { return "newkey123" }, nil)
	if _, err := svc.RotateKey(context.Background(), 1); err == nil {
		t.Fatal("expected gateway error")
	}
	if store.updatedKey != "" {
		t.Errorf("DB key must NOT change on gateway failure, got %q", store.updatedKey)
	}
}
```
NOTE: the fake gateway (`internal/apisix/fake.go`) needs a way to (a) report a consumer's current key (`ConsumerKey(username) string`) and (b) force `EnsureConsumer` to fail (`FailEnsureConsumer bool`). If the Fake doesn't expose these, add them in this task (they're test affordances on the Fake): store consumers in a map keyed by username and return the key; when `FailEnsureConsumer` is set, return an error from `EnsureConsumer`.

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/subscriptions/ -run TestRotateKey -v`
Expected: FAIL — `RotateKey` undefined (and Fake helpers if added).

- [ ] **Step 4: Extend the Fake gateway (test affordances)**

In `internal/apisix/fake.go`, ensure the Fake records consumers and can fail. If not already present:
```go
// add fields to Fake:
//   consumers map[string]string // username -> apiKey
//   FailEnsureConsumer bool
```
In `EnsureConsumer`: if `f.FailEnsureConsumer { return errors.New("fake: ensure consumer failed") }`; else record `f.consumers[username] = apiKey` (lazily init the map). Add:
```go
func (f *Fake) ConsumerKey(username string) string { return f.consumers[username] }
```
(Keep existing behaviour/return values otherwise; only ADD the map write, the fail flag, and the accessor.)

- [ ] **Step 5: Add sentinels, Store methods, and RotateKey**

In `internal/subscriptions/service.go`, add the sentinels near the other `Err…` vars:
```go
var (
	ErrNoCredential         = errors.New("subscriptions: application has no credential")
	ErrNoActiveSubscription = errors.New("subscriptions: application has no active subscription")
)
```
Add to the `Store` interface:
```go
	GetCredential(ctx context.Context, appID int64) (Credential, error)
	ActivePlanForApp(ctx context.Context, appID int64) (PlanInfo, error)
	UpdateCredentialKey(ctx context.Context, appID int64, newKey string) error
```
Add the method:
```go
// RotateKey issues a fresh key-auth key for the application, installs it on the
// APISIX consumer (the old key 401s immediately), then persists it. The gateway
// call precedes the DB write so a gateway failure leaves the old, still-live key
// in the DB. The consumer's rate limit is preserved from the app's most-recent
// active subscription plan. Returns the new key.
func (s *Service) RotateKey(ctx context.Context, appID int64) (string, error) {
	cred, err := s.store.GetCredential(ctx, appID)
	if errors.Is(err, ErrNotFound) {
		return "", ErrNoCredential
	}
	if err != nil {
		return "", err
	}
	plan, err := s.store.ActivePlanForApp(ctx, appID)
	if err != nil {
		return "", err // ErrNoActiveSubscription bubbles to a 409 at the handler
	}
	newKey := s.genKey()
	if err := s.gw.EnsureConsumer(ctx, cred.ConsumerUsername, newKey,
		apisix.RateLimit{Count: plan.Count, WindowSeconds: plan.WindowSeconds}); err != nil {
		return "", err
	}
	if err := s.store.UpdateCredentialKey(ctx, appID, newKey); err != nil {
		return "", err
	}
	s.logEvent(ctx, appID, events.KindKeyRotated, nil, nil)
	return newKey, nil
}
```

- [ ] **Step 6: Run the tests + vet**

Run: `go test ./internal/subscriptions/ -run TestRotateKey -v && go vet ./internal/subscriptions/ ./internal/apisix/ && go build ./...`
Expected: PASS (4 tests). NOTE: adding methods to the `Store` interface makes the package fail to build until the real `Repo` implements them (Task 2). If `go build ./...` fails ONLY with "*Repo does not implement Store (missing ActivePlanForApp/UpdateCredentialKey)", that is expected — the `-run TestRotateKey` unit tests use the fake and still pass. Proceed to Task 2 to satisfy the interface. (If you prefer a green build at every step, do Task 2's repo methods before this step's `go build`.)

- [ ] **Step 7: Commit**

```bash
git add internal/events/events.go internal/subscriptions/service.go internal/subscriptions/service_test.go internal/apisix/fake.go
git commit -m "feat(subscriptions): RotateKey service + key_rotated event"
```

---

## Task 2: Repo `ActivePlanForApp` + `UpdateCredentialKey`

**Files:**
- Modify: `internal/subscriptions/repo.go`
- Test: `internal/subscriptions/repo_test.go`

**Interfaces:**
- Consumes: `r.cipher`, `ErrNotFound`, `ErrNoActiveSubscription` (from Task 1), `PlanInfo`.
- Produces: `*Repo.ActivePlanForApp`, `*Repo.UpdateCredentialKey` (satisfying the Store interface).

- [ ] **Step 1: Write the failing DB tests**

In `internal/subscriptions/repo_test.go` (use the file's existing `testRepo(t) (ctx, *Repo, appID)` helper — it seeds a user + app; access `repo.pool` for same-package seeding; clean up with `t.Cleanup`):
```go
func TestActivePlanForApp(t *testing.T) {
	ctx, repo, appID := testRepo(t)
	// Seed a product, a plan, and an ACTIVE subscription for appID.
	var planID, prodID int64
	_ = repo.pool.QueryRow(ctx, `INSERT INTO plans(name,rate_limit_count,rate_limit_window_s) VALUES($1,$2,$3) RETURNING id`,
		"RotPlan-"+t.Name(), 123, 60).Scan(&planID)
	_ = repo.pool.QueryRow(ctx, `INSERT INTO api_products(name,slug,category,context_path,published) VALUES($1,$2,'C','/rp',true) RETURNING id`,
		"RotProd-"+t.Name(), "rotprod-"+t.Name()).Scan(&prodID)
	_, _ = repo.pool.Exec(ctx, `INSERT INTO subscriptions(application_id,api_product_id,plan_id,status) VALUES($1,$2,$3,'active')`, appID, prodID, planID)
	t.Cleanup(func() {
		_, _ = repo.pool.Exec(ctx, `DELETE FROM subscriptions WHERE application_id=$1`, appID)
		_, _ = repo.pool.Exec(ctx, `DELETE FROM api_products WHERE id=$1`, prodID)
		_, _ = repo.pool.Exec(ctx, `DELETE FROM plans WHERE id=$1`, planID)
	})

	p, err := repo.ActivePlanForApp(ctx, appID)
	if err != nil || p.Count != 123 || p.WindowSeconds != 60 {
		t.Fatalf("ActivePlanForApp = %+v, %v", p, err)
	}
}

func TestActivePlanForApp_NoneActive(t *testing.T) {
	ctx, repo, appID := testRepo(t)
	if _, err := repo.ActivePlanForApp(ctx, appID); !errors.Is(err, ErrNoActiveSubscription) {
		t.Fatalf("want ErrNoActiveSubscription, got %v", err)
	}
}

func TestUpdateCredentialKey(t *testing.T) {
	ctx, repo, appID := testRepo(t)
	// Create a credential, then rotate it.
	if _, err := repo.GetOrCreateCredential(ctx, appID, func() string { return "first" }); err != nil {
		t.Fatalf("seed credential: %v", err)
	}
	if err := repo.UpdateCredentialKey(ctx, appID, "second"); err != nil {
		t.Fatalf("UpdateCredentialKey: %v", err)
	}
	got, err := repo.GetCredential(ctx, appID)
	if err != nil || got.APIKey != "second" {
		t.Fatalf("after update GetCredential = %q, %v (want decrypted 'second')", got.APIKey, err)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/subscriptions/ -run 'TestActivePlanForApp|TestUpdateCredentialKey' -v`
Expected: FAIL — methods undefined.

- [ ] **Step 3: Implement the repo methods**

In `internal/subscriptions/repo.go`:
```go
// ActivePlanForApp returns the plan of the application's most-recent ACTIVE
// subscription (the "current" consumer limit under the last-write-wins model),
// or ErrNoActiveSubscription when the app has none.
func (r *Repo) ActivePlanForApp(ctx context.Context, appID int64) (PlanInfo, error) {
	var p PlanInfo
	err := r.pool.QueryRow(ctx,
		`SELECT p.id, p.rate_limit_count, p.rate_limit_window_s
		   FROM subscriptions s JOIN plans p ON p.id = s.plan_id
		 WHERE s.application_id=$1 AND s.status='active'
		 ORDER BY s.created_at DESC LIMIT 1`, appID).Scan(&p.ID, &p.Count, &p.WindowSeconds)
	if errors.Is(err, pgx.ErrNoRows) {
		return PlanInfo{}, ErrNoActiveSubscription
	}
	return p, err
}

// UpdateCredentialKey replaces the application's stored (encrypted) api key.
func (r *Repo) UpdateCredentialKey(ctx context.Context, appID int64, newKey string) error {
	enc, err := r.cipher.Encrypt(newKey)
	if err != nil {
		return err
	}
	ct, err := r.pool.Exec(ctx, `UPDATE credentials SET api_key=$2 WHERE application_id=$1`, appID, enc)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
```

- [ ] **Step 4: Run to verify they pass + full package build**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/subscriptions/ && go build ./... && go vet ./internal/subscriptions/`
Expected: PASS; the package now builds (Repo satisfies Store).

- [ ] **Step 5: Commit**

```bash
git add internal/subscriptions/repo.go internal/subscriptions/repo_test.go
git commit -m "feat(subscriptions): repo ActivePlanForApp + UpdateCredentialKey"
```

---

## Task 3: Rotate endpoint

**Files:**
- Modify: `internal/subscriptions/handler.go`
- Test: `internal/subscriptions/handler_test.go`

**Interfaces:**
- Consumes: `Service.RotateKey`, `ErrNoCredential`, `ErrNoActiveSubscription`, the handler's existing `authorize(w, r) (int64, bool)`.
- Produces: `POST /api/applications/{appID}/credentials/rotate` → `200 {"apiKey": "..."}`; 409 no-credential/no-active-sub; 403/404 non-owner (via `authorize`).

- [ ] **Step 1: Write the failing handler test**

In `internal/subscriptions/handler_test.go` (mirror the existing handler tests' setup — they build a `*Handler` with a service over a fake store + an `owns` func). Add:
```go
func TestRotateKeyEndpoint(t *testing.T) {
	// Build a handler whose service rotates successfully. Reuse the test setup
	// helper this file already uses for subscribe/unsubscribe handler tests;
	// configure the fake store with a credential + active plan so RotateKey returns a key.
	// (Adapt names to the file's existing helpers.)
	h := newTestHandler(t, /* store with cred app_7 + active plan, owns: user 7 owns app 7 */)
	req := httptest.NewRequest(http.MethodPost, "/api/applications/7/credentials/rotate", nil)
	req = req.WithContext(auth.WithUserID(req.Context(), 7))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct{ APIKey string `json:"apiKey"` }
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.APIKey == "" {
		t.Error("expected a new apiKey in the response")
	}
}

func TestRotateKeyEndpoint_NonOwner403(t *testing.T) {
	h := newTestHandler(t, /* owns returns false for user 9 */)
	req := httptest.NewRequest(http.MethodPost, "/api/applications/7/credentials/rotate", nil)
	req = req.WithContext(auth.WithUserID(req.Context(), 9))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
		t.Fatalf("non-owner status=%d (want 403/404)", rec.Code)
	}
}
```
NOTE: use the EXACT handler-test construction this file already has (the fake store type, the `owns` closure, and any `newTestHandler`-equivalent). If the existing fake store doesn't set a credential/active plan, give it ones so `RotateKey` succeeds. Match `authorize`'s owner semantics for the 403/404 case.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/subscriptions/ -run TestRotateKeyEndpoint -v`
Expected: FAIL — route returns 404/405 (not registered).

- [ ] **Step 3: Register the route + handler method**

In `internal/subscriptions/handler.go`, add the route inside `NewHandler` (after the subscribe/unsubscribe routes):
```go
	h.router.Post("/api/applications/{appID}/credentials/rotate", h.rotateKey)
```
Add the method:
```go
func (h *Handler) rotateKey(w http.ResponseWriter, r *http.Request) {
	appID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	newKey, err := h.svc.RotateKey(r.Context(), appID)
	if errors.Is(err, ErrNoCredential) || errors.Is(err, ErrNoActiveSubscription) {
		httpx.Error(w, http.StatusConflict, "no key to rotate — subscribe and get approved first")
		return
	}
	if err != nil {
		log.Printf("rotate key failed (app=%d): %v", appID, err)
		httpx.Error(w, http.StatusInternalServerError, "rotation failed")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"apiKey": newKey})
}
```

- [ ] **Step 4: Run to verify it passes + full backend suite**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/... ./cmd/... && go vet ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/subscriptions/handler.go internal/subscriptions/handler_test.go
git commit -m "feat(subscriptions): POST /api/applications/{id}/credentials/rotate"
```

---

## Task 4: Frontend client + event type + activity

**Files:**
- Modify: `web/src/api/client.ts` (`rotateKey`)
- Modify: `web/src/api/types.ts` (`AppEventKind` += `key_rotated`)
- Modify: `web/src/pages/application/activity.ts` (describe case)
- Test: `web/src/api/client.rotate.test.ts` (new), `web/src/pages/application/activity.test.ts`

**Interfaces:**
- Produces: `rotateKey(token, appId): Promise<{ apiKey: string }>` → `POST /api/applications/{id}/credentials/rotate`; `AppEventKind` includes `'key_rotated'`; `describe` renders a rotation feed item.

- [ ] **Step 1: Write the failing tests**

Create `web/src/api/client.rotate.test.ts`:
```ts
import { it, expect, vi, afterEach } from 'vitest'
import { rotateKey } from './client'

afterEach(() => vi.restoreAllMocks())

it('POSTs to the rotate endpoint with auth and returns the new key', async () => {
  const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
    new Response(JSON.stringify({ apiKey: 'newkey' }), { status: 200, headers: { 'Content-Type': 'application/json' } }),
  )
  const out = await rotateKey('jwt', 7)
  expect(out.apiKey).toBe('newkey')
  const [url, init] = fetchMock.mock.calls[0]
  expect(url).toBe('/api/applications/7/credentials/rotate')
  expect((init as RequestInit).method).toBe('POST')
  expect((init as RequestInit).headers).toMatchObject({ Authorization: 'Bearer jwt' })
})
```
Add to `web/src/pages/application/activity.test.ts` (match the file's existing test style):
```ts
it('describes a key_rotated event', () => {
  const item = describe({ kind: 'key_rotated', productName: '', planName: '', createdAt: '2026-06-27T10:00:00Z' }, new Date('2026-06-27T10:01:00Z'))
  expect(item.lead).toMatch(/Clé régénérée/)
  expect(item.icon).toBe('rotate')
})
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd web && pnpm exec vitest run src/api/client.rotate.test.ts src/pages/application/activity.test.ts`
Expected: FAIL — `rotateKey` not exported; `key_rotated` not a valid kind.

- [ ] **Step 3: Implement**

`web/src/api/types.ts` — add to the `AppEventKind` union:
```ts
  | 'key_rotated'
```
`web/src/api/client.ts` — add (near `subscribe`/`unsubscribe`):
```ts
export async function rotateKey(token: string, appId: number): Promise<{ apiKey: string }> {
  const url = `/api/applications/${appId}/credentials/rotate`
  return parse<{ apiKey: string }>(await fetch(url, { method: 'POST', headers: authHeaders(token) }), url)
}
```
`web/src/pages/application/activity.ts` — add a case in `describe`'s switch (before `default`):
```ts
    case 'key_rotated':
      return { icon: 'rotate', lead: 'Clé régénérée', rest: '', when }
```

- [ ] **Step 4: Run to verify they pass**

Run: `cd web && pnpm exec vitest run src/api/client.rotate.test.ts src/pages/application/activity.test.ts && pnpm exec tsc --noEmit`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/api/client.ts web/src/api/types.ts web/src/pages/application/activity.ts web/src/api/client.rotate.test.ts web/src/pages/application/activity.test.ts
git commit -m "feat(web): rotateKey client fn + key_rotated activity event"
```

---

## Task 5: CredentialsTab real rotation + remove demo Sandbox

**Files:**
- Modify: `web/src/pages/application/CredentialsTab.tsx`
- Modify: `web/src/pages/application/AppDetailPage.tsx` (pass `appId`/`token`/`lastRotatedAt`/`onRotated`)
- Modify: `web/src/pages/application/demo.ts` (remove sandbox/rotation demo constants)
- Test: `web/src/pages/application/CredentialsTab.test.tsx`

**Interfaces:**
- Consumes: `rotateKey` (Task 4); `formatRelative` from `./activity`; `AppDetailPage`'s `reloadDetail`.
- Produces: `CredentialsTab({ apiKey, appId, token, lastRotatedAt, notify, openModal, onRotated })` — one Production card, real rotation.

- [ ] **Step 1: Write the failing tests (rewrite the existing ones)**

Replace the body of `web/src/pages/application/CredentialsTab.test.tsx` (it currently asserts the demo Sandbox card + "Rotation à venir"; those are gone). New tests:
```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, act, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { CredentialsTab } from './CredentialsTab'
import type { ModalSpec } from '../../components/ConfirmModal'
import * as api from '../../api/client'

const KEY = 'ax_live_a3f9c1e7b240d8e5f6a1b9c4d7e2f8a0'

beforeEach(() => {
  Object.assign(navigator, { clipboard: { writeText: vi.fn().mockResolvedValue(undefined) } })
  vi.restoreAllMocks()
})

function setup() {
  const notify = vi.fn()
  const onRotated = vi.fn()
  let lastModal: ModalSpec | null = null
  const openModal = vi.fn((s: ModalSpec) => { lastModal = s })
  render(<CredentialsTab apiKey={KEY} appId={7} token="jwt" notify={notify} openModal={openModal} onRotated={onRotated} />)
  return { notify, openModal, onRotated, getModal: () => lastModal }
}

describe('CredentialsTab', () => {
  it('shows only the Production key (no Sandbox card)', () => {
    setup()
    expect(screen.getByTestId('key-prod')).toBeInTheDocument()
    expect(screen.queryByTestId('key-sbx')).not.toBeInTheDocument()
  })

  it('reveals on toggle and copies the real key', async () => {
    const { notify } = setup()
    const code = screen.getByTestId('key-prod')
    expect(code.textContent).toBe('ax_live_' + '•'.repeat(KEY.length - 10) + 'a0')
    await userEvent.click(screen.getAllByRole('button', { name: 'Afficher / masquer' })[0])
    expect(code.textContent).toBe(KEY)
    await userEvent.click(screen.getAllByRole('button', { name: 'Copier' })[0])
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(KEY)
    expect(notify).toHaveBeenCalledWith('Clé copiée dans le presse-papiers')
  })

  it('rotate confirms, calls the API, reveals the new key, and notifies', async () => {
    const spy = vi.spyOn(api, 'rotateKey').mockResolvedValue({ apiKey: 'ax_live_NEWKEY00000000000000000000000000' })
    const { openModal, getModal, onRotated, notify } = setup()
    await userEvent.click(screen.getAllByRole('button', { name: /Régénérer/ })[0])
    expect(openModal).toHaveBeenCalled()
    await act(async () => { await getModal()!.onConfirm() })
    expect(spy).toHaveBeenCalledWith('jwt', 7)
    await waitFor(() => expect(screen.getByTestId('key-prod').textContent).toBe('ax_live_NEWKEY00000000000000000000000000'))
    expect(notify).toHaveBeenCalledWith('Nouvelle clé générée')
    expect(onRotated).toHaveBeenCalled()
  })
})
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd web && pnpm exec vitest run src/pages/application/CredentialsTab.test.tsx`
Expected: FAIL — props/behaviour mismatch (no rotateKey wiring; sandbox still present).

- [ ] **Step 3: Rewrite `CredentialsTab.tsx`**

Replace the component. Remove the `demo` import, the Sandbox `KeyCard`, and sandbox state. New signature + Production card with real rotation:
```tsx
import { useEffect, useState } from 'react'
import { maskKey, copyText } from './helpers'
import { formatRelative } from './activity'
import { rotateKey } from '../../api/client'
import type { ModalSpec } from '../../components/ConfirmModal'
// (keep the EyeIcon / CopyIcon / RotateIcon / KeyCard definitions, but KeyCard
//  no longer needs the `kind` sandbox styling beyond 'prod')

export function CredentialsTab({ apiKey, appId, token, lastRotatedAt, notify, openModal, onRotated }: {
  apiKey: string
  appId: number
  token: string
  lastRotatedAt?: string
  notify: (msg: string) => void
  openModal: (spec: ModalSpec) => void
  onRotated: () => void
}) {
  const [shownKey, setShownKey] = useState(apiKey)
  const [revealed, setRevealed] = useState(false)
  // Keep the displayed key in sync when the prop changes (parent refetch / app switch).
  useEffect(() => { setShownKey(apiKey) }, [apiKey])

  function copy() {
    void copyText(shownKey).then(() => notify('Clé copiée dans le presse-papiers'))
  }

  function onRotate() {
    openModal({
      title: 'Régénérer la clé production ?',
      body: 'L’ancienne clé sera révoquée immédiatement dans APISIX (consumer key-auth). Les requêtes qui l’utilisent recevront un 401 — pensez à redéployer.',
      confirmLabel: 'Régénérer la clé',
      danger: true,
      onConfirm: async () => {
        try {
          const { apiKey: nk } = await rotateKey(token, appId)
          setShownKey(nk)
          setRevealed(true)        // reveal the fresh key once
          notify('Nouvelle clé générée')
          onRotated()              // refresh the detail (events / timestamp)
        } catch (e) {
          notify(e instanceof Error ? e.message : 'Échec de la rotation.')
        }
      },
    })
  }

  return (
    <section className="panel">
      <p className="section-title">Clés API · key-auth</p>
      <div className="keygrid">
        <div className="keycard prod">
          <div className="kh"><span className="env">Production <span className="envtag">live</span></span></div>
          <div className="kb">
            <div className="keyrow">
              <code data-testid="key-prod">{revealed ? shownKey : maskKey(shownKey)}</code>
              <button className="iconbtn" aria-label="Afficher / masquer" aria-pressed={revealed} onClick={() => setRevealed(r => !r)}><EyeIcon /></button>
              <button className="iconbtn" aria-label="Copier" onClick={copy}><CopyIcon /></button>
            </div>
            <div className="keymeta">
              <span>Dernière rotation · <span className="mono">{lastRotatedAt ? formatRelative(lastRotatedAt) : '—'}</span></span>
              <button className="rotate" onClick={onRotate}><RotateIcon />Régénérer</button>
            </div>
          </div>
        </div>
      </div>

      {/* keep the existing "Sécurité de la clé" dcard block as-is */}
    </section>
  )
}
```
Keep the existing `EyeIcon`/`CopyIcon`/`RotateIcon` and the "Sécurité de la clé" info card. Delete the `KeyCard` helper if it's now unused, or leave it only if still referenced (it isn't — the Production card is inlined above; remove `KeyCard` to avoid dead code).

- [ ] **Step 4: Update `AppDetailPage.tsx` to pass the new props**

Change the CredentialsTab render (currently `<CredentialsTab apiKey={detail.apiKey} notify={notify} openModal={setModal} />`) to:
```tsx
{tab === 'creds' && (
  <CredentialsTab
    apiKey={detail.apiKey}
    appId={appId}
    token={token}
    lastRotatedAt={detail.events.find(e => e.kind === 'key_rotated')?.createdAt}
    notify={notify}
    openModal={setModal}
    onRotated={reloadDetail}
  />
)}
```
(`token`, `appId`, `reloadDetail`, and `detail` are already in scope in `AppDetailPage`. `detail.events` is newest-first if the API returns it so; if not, sort/`find` the latest `key_rotated` — events come ordered `created_at DESC` from the events repo, so `.find` yields the most recent.)

- [ ] **Step 5: Remove demo constants**

In `web/src/pages/application/demo.ts`, delete `DEMO_SANDBOX_KEY`, `DEMO_ROTATION`, and `demoRotatedKey`. Keep anything still imported elsewhere (`demoBarWidth`, `demoRpm`, `DEMO_QUICKSTART`). Verify nothing else imports the removed names: `grep -rn "DEMO_SANDBOX_KEY\|DEMO_ROTATION\|demoRotatedKey" web/src` returns nothing.

- [ ] **Step 6: Run tests + full gate**

Run: `cd web && pnpm exec vitest run --exclude 'e2e/**' && pnpm exec tsc --noEmit && pnpm build`
Expected: all green.

- [ ] **Step 7: Commit**

```bash
git add web/src/pages/application/CredentialsTab.tsx web/src/pages/application/AppDetailPage.tsx web/src/pages/application/demo.ts web/src/pages/application/CredentialsTab.test.tsx
git commit -m "feat(web): real key rotation in CredentialsTab; drop demo Sandbox card"
```

---

## Task 6: Live verification

- [ ] **Step 1: Ensure stack + portal running**

Dev `docker compose` up (postgres + apisix + echo); portal on `:8090` (`PORTAL_ENV=dev UPSTREAM_ALLOW_PRIVATE=1 PORTAL_ADDR=:8090 go run ./cmd/portal`); Vite on `:5173`.

- [ ] **Step 2: Rotate against a real approved app**

Reuse the try-it/echo product (an app with an approved subscription, so a consumer + route exist). Capture the current key, then call:
```bash
curl -s -X POST http://localhost:8090/api/applications/<APPID>/credentials/rotate -H "Authorization: Bearer <DEV_TOKEN>"
```
Expected: `200 {"apiKey":"<new>"}` different from the old.

- [ ] **Step 3: Confirm the old key is dead and the new key works at the gateway**

```bash
# old key now rejected:
curl -s -o /dev/null -w '%{http_code}\n' http://localhost:9080/<context_path>/ping -H "apikey: <OLD_KEY>"   # expect 401
# new key accepted:
curl -s -o /dev/null -w '%{http_code}\n' http://localhost:9080/<context_path>/ping -H "apikey: <NEW_KEY>"   # expect 200
```

- [ ] **Step 4: Browser**

Open the app's Credentials tab: only the Production card shows (no Sandbox); click **Régénérer** → confirm → the new key is revealed and "Dernière rotation" updates; the Overview activity feed shows "Clé régénérée". **Look at the screenshot.**

---

## Self-Review notes

- **Spec coverage:** endpoint owner-scoped (T3) ✅; RotateKey gateway-before-DB + immediate revoke (T1) ✅; limit re-derived from most-recent active plan (T1 ActivePlanForApp + T2) ✅; 409 no-credential/no-active-sub (T1 sentinels + T3 mapping) ✅; encrypted key persistence (T2 UpdateCredentialKey) ✅; `key_rotated` event (T1 const + T4 type/activity) ✅; remove demo Sandbox card + constants (T5) ✅; real "Dernière rotation" timestamp (T5 lastRotatedAt) ✅; tests Go+vitest+live ✅.
- **Type consistency:** `RotateKey(ctx, appID) (string, error)`, `rotateKey(token, appId) → {apiKey}`, `ActivePlanForApp → PlanInfo{Count,WindowSeconds}`, `UpdateCredentialKey(appID, newKey)`, `KindKeyRotated="key_rotated"` consistent across tasks.
- **Implementer notes:** Task 1's `go build ./...` is expected to fail on the Store-interface gap until Task 2 lands (do T2's repo methods first if you want a green build at every checkpoint). Match the existing fake-store / handler-test construction in `service_test.go` / `handler_test.go` exactly (names differ per file). The Fake gateway test affordances (`ConsumerKey`, `FailEnsureConsumer`) are added in T1 only if not already present.
