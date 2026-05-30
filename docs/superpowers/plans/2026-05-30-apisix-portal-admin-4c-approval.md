# Plan 4c — Subscription Approval Workflow Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** New subscriptions start `pending` with NO gateway provisioning; an admin approves (→ `active`, then provision the consumer + route whitelist) or rejects (→ `rejected`). The developer's app detail shows each subscription's status. Admin endpoints live at `/api/admin/subscriptions`.

**Architecture:** This is the only sub-plan that changes existing subscribe behavior, so it is isolated. The data layer (`subscriptions.Repo` + the `memStore` test fake) is reworked to track subscriptions as status-carrying records (Task 1). Then `Subscribe` is flipped to persist `pending` without provisioning, and `Approve`/`Reject` are added to `subscriptions.Service` (Task 2). The admin HTTP surface lives in the `subscriptions` package (Task 3) — not `internal/admin` — because approve/reject orchestration and the pending-queue read are intrinsically subscription-domain and would otherwise force `internal/admin` to import `subscriptions` for a structured return type. It is mounted behind `auth.RequireAdmin` in main (Task 4).

**Tech Stack:** Go, chi v5, pgx v5, the `apisix.Gateway` interface + `Fake`, `httpx` helpers — all already in use. **No DB migration:** `subscriptions.status TEXT NOT NULL DEFAULT 'active'` already exists from migration `0003`.

---

## Context the implementer needs

- **Status column already exists.** `subscriptions` has `status TEXT NOT NULL DEFAULT 'active'` (migration `0003`). Existing rows are `active`. We introduce `pending` and `rejected`. No migration.
- **Whitelist already filters active.** `Repo.ConsumersForProduct` and `Repo.ConsumersForPlan` already filter `s.status='active'` (added in 4a/4b). So a `pending`/`rejected` subscription is automatically excluded from any route whitelist or plan re-provision — that is what makes "no provisioning until approved" work.
- **Provisioning model.** One APISIX consumer per application (`app_<id>`), key-auth key + one `limit-count`. Route per product (`prod_<id>`), key-auth + consumer-restriction whitelist rebuilt from active subscribers. `Service.ReprovisionRoute(productID)` rebuilds the route from the current upstream + active subscribers; `Service.ReprovisionPlan` re-applies limits to active subscribers' consumers.
- **`Subscribe` today** (`service.go`): validates product, gets plan, `GetOrCreateCredential`, `EnsureConsumer`, `SaveSubscription` (writes `active`), `ReprovisionRoute`. After 4c it must: validate product + plan, `GetOrCreateCredential` (so the dev still gets a key), `SaveSubscription` (writes `pending`), and **do no gateway calls**. The returned key won't pass the gateway until the route whitelist includes the app — which only happens on approve.
- **`memStore`** (`service_test.go`) currently stores `subs map[int64][]string` (productID→consumer names). Task 1 rewrites it to record-based storage so it can answer status queries; all existing service tests are updated in the same task to stay green (behavior is preserved in Task 1 — `Subscribe` still provisions+active until Task 2).
- **Key existing types:** `Credential{ApplicationID, APIKey, ConsumerUsername}`, `ProductInfo{ID, ContextPath, Upstream}`, `PlanInfo{ID, Count, WindowSeconds}`, `SubscriptionView{ProductID, ProductName, Version, ContextPath, PlanID, PlanName}` (+ `Status` added in Task 1), `AppDetail{APIKey, ConsumerUsername, Subscriptions}`. Helpers `consumerName(appID)` → `app_<id>`, `RouteID(productID)` → `prod_<id>`.
- **Handler pattern:** embed `chi.Router`, register full paths, `ServeHTTP`, narrow interface dependency, `httpx.JSON`/`httpx.Error`, `log.Printf` + generic 500. Admin handler does NOT check auth (mounted behind `RequireAdmin`).
- **Commands:** `go test ./internal/<pkg>/ -run X -v`, `go test ./internal/... ./cmd/...`, `go build ./...`, `gofmt -l <file>`. Commit trailer: `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`. Branch `master`.

## File structure (what this plan creates / modifies)

- **Modify** `internal/subscriptions/view.go` — add `Status` to `SubscriptionView`; add `SubscriptionRecord` + `AdminSubscriptionView` types.
- **Modify** `internal/subscriptions/service.go` — extend `Store` (3 new methods); flip `Subscribe`; add `Approve`, `Reject`, `AdminSubscriptions`.
- **Modify** `internal/subscriptions/repo.go` — `SaveSubscription` writes `pending`; `SubscriptionsForApp` returns all statuses incl. `status`; add `GetSubscription`, `SetSubscriptionStatus`, `AdminSubscriptions`.
- **Modify** `internal/subscriptions/service_test.go` — rewrite `memStore` to record-based; update existing tests; add approval tests.
- **Modify** `internal/subscriptions/handler_test.go` — update the subscribe test (no provisioning on subscribe).
- **Create** `internal/subscriptions/admin_handler.go` — `AdminHandler` for `/api/admin/subscriptions*`.
- **Create** `internal/subscriptions/admin_handler_test.go` — handler tests.
- **Modify** `cmd/portal/main.go` — construct + mount the admin subscription handler behind `RequireAdmin`.

---

## Task 1: Record-based data layer (behavior-preserving) + status reads

This reworks the persistence surface and the test fake to track subscriptions as status-carrying records, and surfaces `status` on the developer's app detail. **Behavior is unchanged this task:** `Subscribe` still writes `active` and still provisions; the new capabilities are wired but not yet used to change the flow (that is Task 2). This keeps the task reviewable and the suite green.

**Files:**
- Modify: `internal/subscriptions/view.go`
- Modify: `internal/subscriptions/service.go` (Store interface only)
- Modify: `internal/subscriptions/repo.go`
- Modify: `internal/subscriptions/service_test.go` (rewrite `memStore`, update tests)

- [ ] **Step 1: Add types to `view.go`**

In `internal/subscriptions/view.go`, add `Status` to `SubscriptionView` and add two new types:

```go
package subscriptions

import "time"

// SubscriptionView is one of an application's subscriptions, enriched with the
// product and plan names for display, including its approval status.
type SubscriptionView struct {
	ProductID   int64  `json:"productId"`
	ProductName string `json:"productName"`
	Version     string `json:"version"`
	ContextPath string `json:"contextPath"`
	PlanID      int64  `json:"planId"`
	PlanName    string `json:"planName"`
	Status      string `json:"status"`
}

// AppDetail is the response for GET /api/applications/{id}: the app's gateway
// key (empty until it has at least one subscription) and its subscriptions.
type AppDetail struct {
	APIKey           string             `json:"apiKey"`
	ConsumerUsername string             `json:"consumerUsername"`
	Subscriptions    []SubscriptionView `json:"subscriptions"`
}

// SubscriptionRecord is the minimal subscription identity used by the approval
// flow (look up a subscription by id to provision/transition it).
type SubscriptionRecord struct {
	ID        int64
	AppID     int64
	ProductID int64
	PlanID    int64
	Status    string
}

// AdminSubscriptionView is one row of the admin approval queue.
type AdminSubscriptionView struct {
	ID              int64     `json:"id"`
	ApplicationName string    `json:"applicationName"`
	OwnerEmail      string    `json:"ownerEmail"`
	ProductName     string    `json:"productName"`
	Version         string    `json:"version"`
	PlanName        string    `json:"planName"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"createdAt"`
}
```

- [ ] **Step 2: Extend the `Store` interface in `service.go`**

In `internal/subscriptions/service.go`, add to the `Store` interface (after `ConsumersForPlan`):

```go
	// GetSubscription returns a single subscription's identity + status by id.
	GetSubscription(ctx context.Context, subID int64) (SubscriptionRecord, error)
	// SetSubscriptionStatus transitions a subscription to the given status.
	SetSubscriptionStatus(ctx context.Context, subID int64, status string) error
	// AdminSubscriptions lists subscriptions for the admin queue. An empty
	// statusFilter returns all; otherwise only rows with that status.
	AdminSubscriptions(ctx context.Context, statusFilter string) ([]AdminSubscriptionView, error)
```

- [ ] **Step 3: Implement the new methods + status reads in `repo.go`**

In `internal/subscriptions/repo.go`:

(a) Change `SaveSubscription` to write `active` STILL (unchanged this task — Task 2 flips it). Leave it as-is.

(b) Change `SubscriptionsForApp` to return ALL statuses (drop the `AND s.status='active'` filter) and select `s.status`:

```go
// SubscriptionsForApp returns the application's subscriptions for display,
// including pending/rejected ones so the developer can see their status.
func (r *Repo) SubscriptionsForApp(ctx context.Context, appID int64) ([]SubscriptionView, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT s.api_product_id, p.name, p.version, p.context_path, s.plan_id, pl.name, s.status
		 FROM subscriptions s
		 JOIN api_products p ON p.id = s.api_product_id
		 JOIN plans pl ON pl.id = s.plan_id
		 WHERE s.application_id=$1
		 ORDER BY p.name`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SubscriptionView
	for rows.Next() {
		var v SubscriptionView
		if err := rows.Scan(&v.ProductID, &v.ProductName, &v.Version, &v.ContextPath, &v.PlanID, &v.PlanName, &v.Status); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
```

(c) Add the three new methods (place after `SubscriptionsForApp`):

```go
// GetSubscription returns the subscription's identity + status, or ErrNotFound.
func (r *Repo) GetSubscription(ctx context.Context, subID int64) (SubscriptionRecord, error) {
	var s SubscriptionRecord
	err := r.pool.QueryRow(ctx,
		`SELECT id, application_id, api_product_id, plan_id, status FROM subscriptions WHERE id=$1`, subID,
	).Scan(&s.ID, &s.AppID, &s.ProductID, &s.PlanID, &s.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return SubscriptionRecord{}, ErrNotFound
	}
	return s, err
}

// SetSubscriptionStatus transitions a subscription to the given status.
func (r *Repo) SetSubscriptionStatus(ctx context.Context, subID int64, status string) error {
	tag, err := r.pool.Exec(ctx, `UPDATE subscriptions SET status=$2 WHERE id=$1`, subID, status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// AdminSubscriptions lists subscriptions for the admin queue, newest first. An
// empty statusFilter returns all rows; otherwise only those with that status.
func (r *Repo) AdminSubscriptions(ctx context.Context, statusFilter string) ([]AdminSubscriptionView, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT s.id, a.name, u.email, p.name, p.version, pl.name, s.status, s.created_at
		 FROM subscriptions s
		 JOIN applications a ON a.id = s.application_id
		 JOIN users u ON u.id = a.owner_id
		 JOIN api_products p ON p.id = s.api_product_id
		 JOIN plans pl ON pl.id = s.plan_id
		 WHERE ($1 = '' OR s.status = $1)
		 ORDER BY s.created_at DESC`, statusFilter)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AdminSubscriptionView
	for rows.Next() {
		var v AdminSubscriptionView
		if err := rows.Scan(&v.ID, &v.ApplicationName, &v.OwnerEmail, &v.ProductName, &v.Version, &v.PlanName, &v.Status, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
```

> `errors` and `pgx` are already imported in `repo.go`.

- [ ] **Step 4: Rewrite `memStore` in `service_test.go` to be record-based**

Replace the `memStore` struct, `newMemStore`, and its methods in `internal/subscriptions/service_test.go` with the version below. Storage is now per-subscription records, so it can answer status queries while still deriving the route whitelist from active records.

```go
type memStore struct {
	creds    map[int64]Credential
	products map[int64]ProductInfo
	plans    map[int64]PlanInfo
	records  map[int64]*SubscriptionRecord // subID -> record
	nextID   int64
}

func newMemStore() *memStore {
	return &memStore{
		creds:    map[int64]Credential{},
		products: map[int64]ProductInfo{3: {ID: 3, ContextPath: "/pizzashack", Upstream: "echo:8080"}},
		plans:    map[int64]PlanInfo{2: {ID: 2, Count: 100, WindowSeconds: 60}},
		records:  map[int64]*SubscriptionRecord{},
	}
}

func (m *memStore) GetOrCreateCredential(_ context.Context, appID int64, genKey func() string) (Credential, error) {
	if c, ok := m.creds[appID]; ok {
		return c, nil
	}
	c := Credential{ApplicationID: appID, APIKey: genKey(), ConsumerUsername: consumerName(appID)}
	m.creds[appID] = c
	return c, nil
}
func (m *memStore) GetProduct(_ context.Context, id int64) (ProductInfo, error) { return m.products[id], nil }
func (m *memStore) GetPlan(_ context.Context, id int64) (PlanInfo, error)       { return m.plans[id], nil }

func (m *memStore) findRecord(appID, productID int64) *SubscriptionRecord {
	for _, r := range m.records {
		if r.AppID == appID && r.ProductID == productID {
			return r
		}
	}
	return nil
}

// SaveSubscription upserts a record. The status it writes mirrors the real repo
// (active in Task 1; Task 2 changes both to pending).
func (m *memStore) SaveSubscription(_ context.Context, appID, productID, planID int64) error {
	if r := m.findRecord(appID, productID); r != nil {
		r.PlanID = planID
		r.Status = "active"
		return nil
	}
	m.nextID++
	m.records[m.nextID] = &SubscriptionRecord{ID: m.nextID, AppID: appID, ProductID: productID, PlanID: planID, Status: "active"}
	return nil
}
func (m *memStore) DeleteSubscription(_ context.Context, appID, productID int64) error {
	if r := m.findRecord(appID, productID); r != nil {
		delete(m.records, r.ID)
	}
	return nil
}
func (m *memStore) ConsumersForProduct(_ context.Context, productID int64) ([]string, error) {
	var out []string
	for _, r := range m.records {
		if r.ProductID == productID && r.Status == "active" {
			out = append(out, consumerName(r.AppID))
		}
	}
	return out, nil
}
func (m *memStore) ConsumersForPlan(_ context.Context, planID int64) ([]Credential, error) {
	var out []Credential
	for _, r := range m.records {
		if r.PlanID == planID && r.Status == "active" {
			if c, ok := m.creds[r.AppID]; ok {
				out = append(out, c)
			} else {
				out = append(out, Credential{ApplicationID: r.AppID, ConsumerUsername: consumerName(r.AppID)})
			}
		}
	}
	return out, nil
}
func (m *memStore) GetSubscription(_ context.Context, subID int64) (SubscriptionRecord, error) {
	if r, ok := m.records[subID]; ok {
		return *r, nil
	}
	return SubscriptionRecord{}, ErrNotFound
}
func (m *memStore) SetSubscriptionStatus(_ context.Context, subID int64, status string) error {
	r, ok := m.records[subID]
	if !ok {
		return ErrNotFound
	}
	r.Status = status
	return nil
}
func (m *memStore) AdminSubscriptions(_ context.Context, statusFilter string) ([]AdminSubscriptionView, error) {
	out := []AdminSubscriptionView{}
	for _, r := range m.records {
		if statusFilter == "" || r.Status == statusFilter {
			out = append(out, AdminSubscriptionView{ID: r.ID, Status: r.Status})
		}
	}
	return out, nil
}
```

- [ ] **Step 5: Update the existing service tests that seeded the old fake**

In `internal/subscriptions/service_test.go`, the `TestReprovisionRoute` and `TestReprovisionPlanUpdatesEachConsumerLimit` tests seeded the removed `subs`/`planConsumers` fields. Replace their seeding with record-based seeding.

`TestReprovisionRoute` — replace its body's store setup so it seeds two active records on product 7:

```go
func TestReprovisionRoute(t *testing.T) {
	store := newMemStore()
	store.products[7] = ProductInfo{ID: 7, ContextPath: "/seven", Upstream: "echo:8080"}
	store.records[101] = &SubscriptionRecord{ID: 101, AppID: 1, ProductID: 7, PlanID: 2, Status: "active"}
	store.records[102] = &SubscriptionRecord{ID: 102, AppID: 2, ProductID: 7, PlanID: 2, Status: "active"}
	gw := apisix.NewFake()
	svc := NewService(store, gw, GenerateKey)

	if err := svc.ReprovisionRoute(context.Background(), 7); err != nil {
		t.Fatalf("reprovision: %v", err)
	}
	r, ok := gw.Routes[RouteID(7)]
	if !ok {
		t.Fatalf("route %s not created", RouteID(7))
	}
	if r.Upstream != "echo:8080" || r.URI != "/seven/*" {
		t.Fatalf("unexpected route: %+v", r)
	}
	if len(r.Allowed) != 2 {
		t.Fatalf("want 2 allowed consumers, got %v", r.Allowed)
	}
}
```

`TestReprovisionPlanUpdatesEachConsumerLimit` — seed creds + two active records on plan 2:

```go
func TestReprovisionPlanUpdatesEachConsumerLimit(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
	store.creds[1] = Credential{ApplicationID: 1, APIKey: "k1", ConsumerUsername: "app_1"}
	store.creds[2] = Credential{ApplicationID: 2, APIKey: "k2", ConsumerUsername: "app_2"}
	store.records[201] = &SubscriptionRecord{ID: 201, AppID: 1, ProductID: 3, PlanID: 2, Status: "active"}
	store.records[202] = &SubscriptionRecord{ID: 202, AppID: 2, ProductID: 3, PlanID: 2, Status: "active"}
	gw := apisix.NewFake()
	svc := NewService(store, gw, GenerateKey)

	if err := svc.ReprovisionPlan(ctx, 2); err != nil {
		t.Fatalf("ReprovisionPlan: %v", err)
	}
	for _, name := range []string{"app_1", "app_2"} {
		c, ok := gw.Consumers[name]
		if !ok {
			t.Fatalf("consumer %s not provisioned", name)
		}
		if c.Limit.Count != 100 || c.Limit.WindowSeconds != 60 {
			t.Fatalf("consumer %s limit = %+v, want {100,60}", name, c.Limit)
		}
	}
	if gw.Consumers["app_1"].APIKey != "k1" {
		t.Fatalf("app_1 key not preserved: %q", gw.Consumers["app_1"].APIKey)
	}
}
```

> `TestSubscribeProvisionsConsumerAndRoute`, `TestUnsubscribeRemovesFromWhitelist`, and `TestDeprovisionRoute` should continue to pass unchanged: in Task 1, `SaveSubscription` still writes `active`, so `Subscribe` still results in an active record and a provisioned consumer/route. If `TestDeprovisionRoute` references any removed field, it only uses `store.products[7]` and the gateway — leave it as-is.

- [ ] **Step 6: Run tests + build**

Run: `go test ./internal/subscriptions/ -v`
Expected: PASS — all existing tests still green with the record-based fake.

Run: `go build ./...` and `gofmt -l internal/subscriptions/*.go` (no output).

- [ ] **Step 7: Commit**

```bash
git add internal/subscriptions/
git commit -m "refactor(subscriptions): record-based store; expose subscription status + admin reads

Subscription status now flows through SubscriptionsForApp; adds GetSubscription,
SetSubscriptionStatus, and AdminSubscriptions to the store. Behavior unchanged
(Subscribe still provisions active) — the approval flip lands next.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: Flip `Subscribe` to pending + add `Approve`/`Reject`

**Files:**
- Modify: `internal/subscriptions/repo.go` (`SaveSubscription` → pending)
- Modify: `internal/subscriptions/service.go` (`Subscribe`, `Approve`, `Reject`, `AdminSubscriptions`)
- Modify: `internal/subscriptions/service_test.go` (`memStore.SaveSubscription` → pending; update/add tests)
- Modify: `internal/subscriptions/handler_test.go` (subscribe no longer provisions)

- [ ] **Step 1: Write the failing tests**

In `internal/subscriptions/service_test.go`, REPLACE `TestSubscribeProvisionsConsumerAndRoute` with a pending test, update `TestUnsubscribeRemovesFromWhitelist` to approve first, and add approve/reject tests:

```go
func TestSubscribeIsPendingAndDoesNotProvision(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
	gw := apisix.NewFake()
	svc := NewService(store, gw, func() string { return "fixed-key" })

	cred, err := svc.Subscribe(ctx, 42, 3, 2)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	// The credential (key) is issued immediately so the dev can see it...
	if cred.APIKey != "fixed-key" || cred.ConsumerUsername != "app_42" {
		t.Fatalf("bad cred: %+v", cred)
	}
	// ...but nothing is provisioned into the gateway until approval.
	if len(gw.Consumers) != 0 {
		t.Fatalf("expected no consumer provisioned on subscribe, got %v", gw.Consumers)
	}
	if len(gw.Routes) != 0 {
		t.Fatalf("expected no route provisioned on subscribe, got %v", gw.Routes)
	}
	// The subscription exists as pending.
	r := store.findRecord(42, 3)
	if r == nil || r.Status != "pending" {
		t.Fatalf("expected a pending record, got %+v", r)
	}
}

func TestApproveProvisionsConsumerAndRoute(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
	gw := apisix.NewFake()
	svc := NewService(store, gw, func() string { return "fixed-key" })

	if _, err := svc.Subscribe(ctx, 42, 3, 2); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	rec := store.findRecord(42, 3)
	if err := svc.Approve(ctx, rec.ID); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if store.records[rec.ID].Status != "active" {
		t.Fatalf("status = %q, want active", store.records[rec.ID].Status)
	}
	c, ok := gw.Consumers["app_42"]
	if !ok || c.Limit.Count != 100 {
		t.Fatalf("consumer not provisioned with plan limits: %+v", c)
	}
	r := gw.Routes["prod_3"]
	if len(r.Allowed) != 1 || r.Allowed[0] != "app_42" {
		t.Fatalf("route whitelist after approve: %+v", r.Allowed)
	}
}

func TestRejectSetsStatusAndDoesNotProvision(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
	gw := apisix.NewFake()
	svc := NewService(store, gw, func() string { return "fixed-key" })

	if _, err := svc.Subscribe(ctx, 42, 3, 2); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	rec := store.findRecord(42, 3)
	if err := svc.Reject(ctx, rec.ID); err != nil {
		t.Fatalf("Reject: %v", err)
	}
	if store.records[rec.ID].Status != "rejected" {
		t.Fatalf("status = %q, want rejected", store.records[rec.ID].Status)
	}
	if _, ok := gw.Consumers["app_42"]; ok {
		t.Fatal("rejected subscription must not provision a consumer")
	}
	// A route may be (re)written with an empty whitelist, but must not allow the app.
	if r, ok := gw.Routes["prod_3"]; ok {
		for _, a := range r.Allowed {
			if a == "app_42" {
				t.Fatal("rejected app must not be in the route whitelist")
			}
		}
	}
}

func TestUnsubscribeRemovesFromWhitelist(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
	gw := apisix.NewFake()
	svc := NewService(store, gw, func() string { return "k" })
	// Subscribe + approve two apps so both are active in the whitelist.
	_, _ = svc.Subscribe(ctx, 42, 3, 2)
	_, _ = svc.Subscribe(ctx, 43, 3, 2)
	if err := svc.Approve(ctx, store.findRecord(42, 3).ID); err != nil {
		t.Fatalf("approve 42: %v", err)
	}
	if err := svc.Approve(ctx, store.findRecord(43, 3).ID); err != nil {
		t.Fatalf("approve 43: %v", err)
	}
	if err := svc.Unsubscribe(ctx, 42, 3); err != nil {
		t.Fatalf("Unsubscribe: %v", err)
	}
	r := gw.Routes["prod_3"]
	if len(r.Allowed) != 1 || r.Allowed[0] != "app_43" {
		t.Fatalf("whitelist after unsubscribe: %+v", r.Allowed)
	}
}
```

Also update `memStore.SaveSubscription` to write `"pending"` in BOTH the update and insert branches (replace the two `"active"` literals with `"pending"`).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/subscriptions/ -run 'TestSubscribeIsPending|TestApprove|TestReject' -v`
Expected: compile failure — `svc.Approve` / `svc.Reject` undefined.

- [ ] **Step 3: Flip `SaveSubscription` (real repo) to pending**

In `internal/subscriptions/repo.go`, change `SaveSubscription`:

```go
func (r *Repo) SaveSubscription(ctx context.Context, appID, productID, planID int64) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO subscriptions(application_id, api_product_id, plan_id, status) VALUES($1,$2,$3,'pending')
		 ON CONFLICT (application_id, api_product_id)
		 DO UPDATE SET plan_id=EXCLUDED.plan_id, status='pending'`,
		appID, productID, planID)
	return err
}
```

- [ ] **Step 4: Flip `Subscribe` and add `Approve`/`Reject`/`AdminSubscriptions` in `service.go`**

Replace `Subscribe`:

```go
// Subscribe records a PENDING subscription and issues the application's gateway
// credential, but performs NO provisioning — the key will not pass the gateway
// until an admin approves the subscription. Returns the credential.
func (s *Service) Subscribe(ctx context.Context, appID, productID, planID int64) (Credential, error) {
	if _, err := s.store.GetProduct(ctx, productID); err != nil {
		return Credential{}, err
	}
	if _, err := s.store.GetPlan(ctx, planID); err != nil {
		return Credential{}, err
	}
	cred, err := s.store.GetOrCreateCredential(ctx, appID, s.genKey)
	if err != nil {
		return Credential{}, err
	}
	if err := s.store.SaveSubscription(ctx, appID, productID, planID); err != nil {
		return Credential{}, err
	}
	return cred, nil
}
```

Add (after `Unsubscribe`):

```go
// Approve activates a pending subscription: it provisions the application's
// consumer with the plan's limits and rebuilds the product route whitelist to
// include it. Idempotent — approving an already-active subscription re-asserts
// the gateway state.
func (s *Service) Approve(ctx context.Context, subID int64) error {
	rec, err := s.store.GetSubscription(ctx, subID)
	if err != nil {
		return err
	}
	plan, err := s.store.GetPlan(ctx, rec.PlanID)
	if err != nil {
		return err
	}
	cred, err := s.store.GetOrCreateCredential(ctx, rec.AppID, s.genKey)
	if err != nil {
		return err
	}
	if err := s.gw.EnsureConsumer(ctx, cred.ConsumerUsername, cred.APIKey,
		apisix.RateLimit{Count: plan.Count, WindowSeconds: plan.WindowSeconds}); err != nil {
		return err
	}
	if err := s.store.SetSubscriptionStatus(ctx, subID, "active"); err != nil {
		return err
	}
	return s.ReprovisionRoute(ctx, rec.ProductID)
}

// Reject marks a subscription rejected and rebuilds the product route whitelist
// so the application is excluded (a no-op for a still-pending subscription,
// which was never in the whitelist).
func (s *Service) Reject(ctx context.Context, subID int64) error {
	rec, err := s.store.GetSubscription(ctx, subID)
	if err != nil {
		return err
	}
	if err := s.store.SetSubscriptionStatus(ctx, subID, "rejected"); err != nil {
		return err
	}
	return s.ReprovisionRoute(ctx, rec.ProductID)
}

// AdminSubscriptions lists subscriptions for the admin queue (see Store).
func (s *Service) AdminSubscriptions(ctx context.Context, statusFilter string) ([]AdminSubscriptionView, error) {
	return s.store.AdminSubscriptions(ctx, statusFilter)
}
```

- [ ] **Step 5: Run tests + build**

Run: `go test ./internal/subscriptions/ -v`
Expected: PASS — new pending/approve/reject tests pass; updated unsubscribe test passes; the others remain green.

Run: `go build ./...` (note: the subscribe HANDLER test in `handler_test.go` may now fail — fix it in Step 6 before relying on a green `go test ./...`).

- [ ] **Step 6: Fix the subscribe handler test**

In `internal/subscriptions/handler_test.go`, `TestSubscribeEndpointProvisionsAndReturnsKey` asserts the consumer is provisioned on subscribe — that is no longer true. Rename and update it to assert the key is returned but NO consumer is provisioned:

```go
func TestSubscribeEndpointReturnsKeyWithoutProvisioning(t *testing.T) {
	h, gw := newTestHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/applications/1/subscriptions", strings.NewReader(`{"productId":3,"planId":2}`))
	req = req.WithContext(auth.WithUserID(req.Context(), 5))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"apiKey":"key-xyz"`) {
		t.Fatalf("missing api key in body: %s", rec.Body)
	}
	if len(gw.Consumers) != 0 {
		t.Fatalf("subscribe must not provision a consumer (pending), got %v", gw.Consumers)
	}
}
```

`TestUnsubscribeEndpoint` subscribes then unsubscribes; unsubscribe deletes the record regardless of status and rebuilds the (empty) route, returning 204 — it stays green. Leave it.

- [ ] **Step 7: Run the whole module**

Run: `go test ./internal/... ./cmd/...`
Expected: all packages pass.

Run: `gofmt -l internal/subscriptions/*.go` (no output).

- [ ] **Step 8: Commit**

```bash
git add internal/subscriptions/
git commit -m "feat(subscriptions): approval flow — subscribe is pending, add Approve/Reject

New subscriptions persist as pending with no gateway provisioning; the issued
key works only after an admin Approve (consumer + route whitelist). Reject marks
the subscription and rebuilds the whitelist to exclude it.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: Admin subscription handler (`/api/admin/subscriptions*`)

**Files:**
- Create: `internal/subscriptions/admin_handler.go`
- Create: `internal/subscriptions/admin_handler_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/subscriptions/admin_handler_test.go`:

```go
package subscriptions

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeAdminSvc struct {
	list       []AdminSubscriptionView
	gotFilter  string
	approved   []int64
	rejected   []int64
	approveErr error
	rejectErr  error
}

func (f *fakeAdminSvc) AdminSubscriptions(_ context.Context, statusFilter string) ([]AdminSubscriptionView, error) {
	f.gotFilter = statusFilter
	return f.list, nil
}
func (f *fakeAdminSvc) Approve(_ context.Context, id int64) error {
	if f.approveErr != nil {
		return f.approveErr
	}
	f.approved = append(f.approved, id)
	return nil
}
func (f *fakeAdminSvc) Reject(_ context.Context, id int64) error {
	if f.rejectErr != nil {
		return f.rejectErr
	}
	f.rejected = append(f.rejected, id)
	return nil
}

func TestAdminListPassesStatusFilter(t *testing.T) {
	svc := &fakeAdminSvc{list: []AdminSubscriptionView{{ID: 1, Status: "pending"}}}
	h := NewAdminHandler(svc)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/subscriptions?status=pending", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}
	if svc.gotFilter != "pending" {
		t.Fatalf("status filter = %q, want pending", svc.gotFilter)
	}
}

func TestAdminApprove(t *testing.T) {
	svc := &fakeAdminSvc{}
	h := NewAdminHandler(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/subscriptions/7/approve", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status=%d want 204 (body %s)", rec.Code, rec.Body)
	}
	if len(svc.approved) != 1 || svc.approved[0] != 7 {
		t.Fatalf("approved = %v, want [7]", svc.approved)
	}
}

func TestAdminReject(t *testing.T) {
	svc := &fakeAdminSvc{}
	h := NewAdminHandler(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/subscriptions/9/reject", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status=%d want 204", rec.Code)
	}
	if len(svc.rejected) != 1 || svc.rejected[0] != 9 {
		t.Fatalf("rejected = %v, want [9]", svc.rejected)
	}
}

func TestAdminApproveNotFoundReturns404(t *testing.T) {
	svc := &fakeAdminSvc{approveErr: ErrNotFound}
	h := NewAdminHandler(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/subscriptions/99/approve", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", rec.Code)
	}
}

func TestAdminApproveBadIDReturns400(t *testing.T) {
	svc := &fakeAdminSvc{}
	h := NewAdminHandler(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/subscriptions/abc/approve", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", rec.Code)
	}
}

var _ = errors.Is // keep errors import if unused after edits
```

> Remove the `var _ = errors.Is` line if `errors` ends up used; it is only there to avoid an unused-import error if you copy incrementally. The final file should not import anything it doesn't use.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/subscriptions/ -run TestAdmin -v`
Expected: compile failure — `NewAdminHandler` / `AdminHandler` undefined.

- [ ] **Step 3: Write the handler**

Create `internal/subscriptions/admin_handler.go`:

```go
package subscriptions

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"apisix-portal/internal/httpx"
)

// AdminService is the surface the admin subscription handler needs (satisfied by *Service).
type AdminService interface {
	AdminSubscriptions(ctx context.Context, statusFilter string) ([]AdminSubscriptionView, error)
	Approve(ctx context.Context, subID int64) error
	Reject(ctx context.Context, subID int64) error
}

// AdminHandler serves the admin approval surface. Mount behind RequireAdmin.
type AdminHandler struct {
	svc    AdminService
	router chi.Router
}

func NewAdminHandler(svc AdminService) *AdminHandler {
	h := &AdminHandler{svc: svc, router: chi.NewRouter()}
	h.router.Get("/api/admin/subscriptions", h.list)
	h.router.Post("/api/admin/subscriptions/{id}/approve", h.approve)
	h.router.Post("/api/admin/subscriptions/{id}/reject", h.reject)
	return h
}

func (h *AdminHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.router.ServeHTTP(w, r) }

func (h *AdminHandler) list(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.AdminSubscriptions(r.Context(), r.URL.Query().Get("status"))
	if err != nil {
		log.Printf("admin list subscriptions: %v", err)
		httpx.Error(w, http.StatusInternalServerError, "failed to list subscriptions")
		return
	}
	if items == nil {
		items = []AdminSubscriptionView{}
	}
	httpx.JSON(w, http.StatusOK, items)
}

func (h *AdminHandler) approve(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, h.svc.Approve, "approve")
}

func (h *AdminHandler) reject(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, h.svc.Reject, "reject")
}

func (h *AdminHandler) transition(w http.ResponseWriter, r *http.Request, act func(context.Context, int64) error, name string) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "bad subscription id")
		return
	}
	if err := act(r.Context(), id); err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "subscription not found")
			return
		}
		log.Printf("admin %s subscription %d: %v", name, id, err)
		httpx.Error(w, http.StatusInternalServerError, name+" failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 4: Run tests + build**

Run: `go test ./internal/subscriptions/ -v` (all pass, incl. the 5 new admin tests).
Run: `go build ./...` and `gofmt -l internal/subscriptions/admin_handler.go` (no output).

- [ ] **Step 5: Commit**

```bash
git add internal/subscriptions/admin_handler.go internal/subscriptions/admin_handler_test.go
git commit -m "feat(subscriptions): admin approval handler (/api/admin/subscriptions)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: Wire the admin subscription handler into `cmd/portal/main.go`

**Files:**
- Modify: `cmd/portal/main.go`

- [ ] **Step 1: Construct the handler**

In `cmd/portal/main.go`, after the existing plan-admin wiring:
```go
	planAdminSvc := admin.NewPlanService(admin.NewPlanRepo(pool), subSvc)
	planAdminH := admin.NewPlanHandler(planAdminSvc)
```
add:
```go
	subAdminH := subscriptions.NewAdminHandler(subSvc)
```

> `subSvc` (a `*subscriptions.Service`) satisfies `subscriptions.AdminService` because Task 2 added `AdminSubscriptions`, `Approve`, and `Reject` to it.

- [ ] **Step 2: Mount the routes**

After the existing plan-admin mounts:
```go
	mux.Handle("/api/admin/plans", requireAdmin(planAdminH))
	mux.Handle("/api/admin/plans/", requireAdmin(planAdminH))
```
add:
```go
	mux.Handle("/api/admin/subscriptions", requireAdmin(subAdminH))
	mux.Handle("/api/admin/subscriptions/", requireAdmin(subAdminH))
```

- [ ] **Step 3: Verify**

Run: `go build ./...` (clean). If "*subscriptions.Service does not implement subscriptions.AdminService", Task 2's methods are missing — STOP and report.
Run: `go test ./internal/... ./cmd/...` (all pass).
Run: `gofmt -l cmd/portal/main.go` (no output) and `go vet ./cmd/portal/` (clean).

- [ ] **Step 4: Commit**

```bash
git add cmd/portal/main.go
git commit -m "feat(subscriptions): wire admin approval behind RequireAdmin

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: Live smoke (optional, requires running stack)

Manual end-to-end check; skip if the Docker stack / Postgres is not running (hermetic tests cover the logic). Needs an admin token `$TOKEN` (Plan 4a Task 11) and a developer token `$DEV` for a registered non-admin user, plus a product with a real upstream (e.g. the `echo-smoke` product from 4a Task 11 with `upstreamUrl=echo:8080`).

- [ ] **Step 1: Developer subscribes (pending, key issued, gateway rejects)**

```bash
# As developer: create an app, then subscribe to the echo product (id <P>) on a plan (id <PL>)
APP=$(curl -s localhost:8080/api/applications -H "authorization: Bearer $DEV" -H 'content-type: application/json' -d '{"name":"SmokeApp"}' | jq -r .id)
KEY=$(curl -s localhost:8080/api/applications/$APP/subscriptions -H "authorization: Bearer $DEV" -H 'content-type: application/json' -d "{\"productId\":<P>,\"planId\":<PL>}" | jq -r .apiKey)
# Gateway call with the issued key BEFORE approval → 401 (app not yet in whitelist)
curl -s -o /dev/null -w 'pre-approve: %{http_code}\n' localhost:9080/echosmoke/x -H "apikey: $KEY"   # expect 401
```

- [ ] **Step 2: Admin sees it pending, approves, gateway now allows**

```bash
# Admin pending queue includes it
curl -s -w '\n%{http_code}\n' "localhost:8080/api/admin/subscriptions?status=pending" -H "authorization: Bearer $TOKEN"
# Approve by subscription id <S> from the queue
curl -s -o /dev/null -w 'approve: %{http_code}\n' -X POST localhost:8080/api/admin/subscriptions/<S>/approve -H "authorization: Bearer $TOKEN"   # expect 204
sleep 1
# Gateway call now succeeds
curl -s -o /dev/null -w 'post-approve: %{http_code}\n' localhost:9080/echosmoke/x -H "apikey: $KEY"   # expect 200
```

- [ ] **Step 3: Reject path on a second subscription**

```bash
# Reject some pending subscription id <S2> → 204; developer's app detail shows status "rejected"
curl -s -o /dev/null -w 'reject: %{http_code}\n' -X POST localhost:8080/api/admin/subscriptions/<S2>/reject -H "authorization: Bearer $TOKEN"
curl -s localhost:8080/api/applications/$APP -H "authorization: Bearer $DEV" | jq '.subscriptions[] | {productId, status}'
```

Expected: pre-approve `401`, approve `204`, post-approve `200`, reject `204`, and the app detail shows the rejected subscription's status.

---

## Self-review notes (already applied)

- **Spec coverage:** subscribe → `pending` with no provisioning (Task 2); admin `GET /api/admin/subscriptions?status=pending`, `POST .../{id}/approve`, `POST .../{id}/reject` (Tasks 3–4); approve provisions consumer + route, reject excludes (Task 2); whitelist only includes active (pre-existing `ConsumersForProduct` filter); developer app detail shows status (Task 1 `SubscriptionView.Status`). No migration needed — `status` exists from `0003`.
- **Decision applied:** universal approval-required (every new subscription is pending; no per-product auto-approve flag).
- **Placement decision:** admin subscription handler lives in `subscriptions` (not `internal/admin`) so the package owning the approve/reject orchestration and the join query also owns the thin HTTP layer, avoiding an `internal/admin → subscriptions` import for a structured return type. Routes are still `/api/admin/...` and mounted behind `RequireAdmin`, consistent with 4a/4b.
- **Idempotency:** `Approve` re-asserts gateway state (EnsureConsumer + ReprovisionRoute) so re-approving is safe. `Reject` rebuilds the whitelist so a previously-active→rejected transition drops the app.
- **Type consistency:** `SubscriptionRecord`, `AdminSubscriptionView`, the 3 new `Store` methods, and the `AdminService` interface are used identically across repo, service, fake, and handler. `memStore` rewritten to records and all dependent tests updated in the same task that changes it.
- **No placeholders:** every code step is complete and compilable with exact commands and expected results.
