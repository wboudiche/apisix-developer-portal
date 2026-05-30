# Plan 4b — Admin Plan Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let admins create/edit/delete rate-limit plans via `/api/admin/plans`, re-provisioning every active subscriber's APISIX consumer when a plan's rate limits change, and refusing to delete a plan still referenced by any subscription.

**Architecture:** Plan writes live in the existing `internal/admin` package (alongside product admin from 4a), behind `auth.RequireAdmin`. The "apply new limits to live consumers" provisioning is added to `subscriptions.Service` as `ReprovisionPlan` (mirroring 4a's `ReprovisionRoute`), so the admin layer triggers gateway changes without duplicating provisioning code. The public `internal/plans` package keeps its read-only `GET /api/plans`.

**Tech Stack:** Go, chi v5, pgx v5, the `apisix.Gateway` interface + `Fake`, `httpx` helpers — all already in use.

---

## Context the implementer needs

- **Provisioning model recap.** The portal uses **one APISIX consumer per application** (`app_<id>`), carrying that app's key-auth key and a single `limit-count`. `subscriptions.Service.Subscribe` sets the consumer's limit to the plan being subscribed (`EnsureConsumer(username, apiKey, RateLimit{Count, WindowSeconds})`). An app with several subscriptions has one consumer whose limit is "last write wins". Therefore **re-provisioning a plan** = for every application with an *active* subscription on that plan, call `EnsureConsumer` again with the plan's new limits. This is an accepted V1 simplification (an app subscribed to two plans ends up with whichever was applied last); do not try to reconcile multi-plan apps in this plan.
- **`plans` table** (migration `0001`): `id BIGSERIAL`, `name TEXT UNIQUE`, `rate_limit_count INT`, `rate_limit_window_s INT`. No migration is needed in 4b.
- **`subscriptions` table** has `plan_id BIGINT NOT NULL REFERENCES plans(id)` (no `ON DELETE CASCADE`). So deleting a plan with any referencing subscription would fail at the DB with a foreign-key violation regardless — 4b turns that into a clean `409` by checking first.
- **`credentials` table**: `application_id` (unique), `api_key`, `consumer_username`. Join `subscriptions → credentials` on `application_id` to get the consumer identity for re-provisioning.
- **Existing `subscriptions` types/methods:**
  - `Credential{ApplicationID int64, APIKey string, ConsumerUsername string}` (`view.go`/`service.go`).
  - `PlanInfo{ID int64, Count int, WindowSeconds int}` and `Store.GetPlan(ctx, id) (PlanInfo, error)` (already exists).
  - `Store` interface (in `service.go`) is implemented by `*Repo` (`repo.go`) and by `memStore` in `service_test.go`. **Adding a method to `Store` requires updating BOTH.**
  - `apisix.RateLimit{Count, WindowSeconds}` and `Gateway.EnsureConsumer(ctx, username, apiKey, RateLimit) error`.
- **Existing `admin` package (from 4a):** `product.go`, `repo.go` (`Repo` wraps `*pgxpool.Pool`, has `isUniqueViolation`), `service.go`, `handler.go`. Sentinels `ErrNotFound`, `ErrSlugTaken`, `ErrHasSubscriptions`. Handler pattern: embed `chi.Router`, register full paths, `ServeHTTP`, depend on a narrow interface, `httpx.JSON`/`httpx.Error`, `log.Printf` + generic 500.
- **Existing `plans.Plan`** (`internal/plans/plan.go`) JSON shape — match it so the frontend reuses one type:
  ```go
  type Plan struct {
      ID            int64  `json:"id"`
      Name          string `json:"name"`
      RateLimit     int    `json:"rateLimit"`
      WindowSeconds int    `json:"windowSeconds"`
  }
  ```
- **Commands:** single test `go test ./internal/<pkg>/ -run TestName -v`; package `go test ./internal/<pkg>/`; all `go test ./internal/... ./cmd/...`. Build `go build ./...`. Format check `gofmt -l <file>`.
- **Commit trailer:** end every commit message with `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`. Work on branch `master`.

## File structure (what this plan creates / modifies)

- **Modify** `internal/subscriptions/service.go` — add `ReprovisionPlan`; add `ConsumersForPlan` to the `Store` interface.
- **Modify** `internal/subscriptions/repo.go` — implement `ConsumersForPlan`.
- **Modify** `internal/subscriptions/service_test.go` — extend `memStore` with `ConsumersForPlan`; add a `ReprovisionPlan` test.
- **Create** `internal/admin/plan.go` — admin `Plan` type + validation.
- **Create** `internal/admin/plan_test.go` — validation tests.
- **Create** `internal/admin/plan_repo.go` — `PlanRepo` SQL (CRUD + subscription count) + plan sentinels.
- **Create** `internal/admin/plan_service.go` — `PlanService` (reprovision on rate change, block delete in use).
- **Create** `internal/admin/plan_service_test.go` — service logic with fakes.
- **Create** `internal/admin/plan_handler.go` — `PlanHandler` for `/api/admin/plans*`.
- **Create** `internal/admin/plan_handler_test.go` — handler status/validation/error mapping with a fake service.
- **Modify** `cmd/portal/main.go` — construct and mount the plan admin handler behind `RequireAdmin`.

---

## Task 1: `ReprovisionPlan` in the subscriptions service

**Files:**
- Modify: `internal/subscriptions/service.go`
- Modify: `internal/subscriptions/repo.go`
- Test: `internal/subscriptions/service_test.go`

- [ ] **Step 1: Write the failing test**

First, the existing `memStore` in `service_test.go` must gain a `ConsumersForPlan` method (the `Store` interface will require it). Add this method to `memStore` (place it next to the other `memStore` methods), plus a field to hold per-plan consumers. At the top of `service_test.go`, the `memStore` struct currently has fields `creds, subs, products, plans`. Add a field `planConsumers map[int64][]Credential` and initialize it in `newMemStore()` as `planConsumers: map[int64][]Credential{}`. Then add:

```go
func (m *memStore) ConsumersForPlan(_ context.Context, planID int64) ([]Credential, error) {
	return m.planConsumers[planID], nil
}
```

Now add the test:

```go
func TestReprovisionPlanUpdatesEachConsumerLimit(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
	// Plan 2 exists in newMemStore with Count=100, Window=60. Two apps are on it.
	store.planConsumers[2] = []Credential{
		{ApplicationID: 1, APIKey: "k1", ConsumerUsername: "app_1"},
		{ApplicationID: 2, APIKey: "k2", ConsumerUsername: "app_2"},
	}
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

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/subscriptions/ -run TestReprovisionPlan -v`
Expected: compile failure — `svc.ReprovisionPlan` undefined (and, once you add the method, the `Store` interface will need `ConsumersForPlan`).

- [ ] **Step 3: Add `ConsumersForPlan` to the `Store` interface and `ReprovisionPlan` to the service**

In `internal/subscriptions/service.go`, add to the `Store` interface (after `ConsumersForProduct`):

```go
	// ConsumersForPlan returns the credential (consumer identity + key) of every
	// application with an active subscription on the plan.
	ConsumersForPlan(ctx context.Context, planID int64) ([]Credential, error)
```

Add the service method (after `DeprovisionRoute`):

```go
// ReprovisionPlan applies the plan's current rate limits to every active
// subscriber's APISIX consumer. Used when an admin edits a plan's limits.
func (s *Service) ReprovisionPlan(ctx context.Context, planID int64) error {
	plan, err := s.store.GetPlan(ctx, planID)
	if err != nil {
		return err
	}
	consumers, err := s.store.ConsumersForPlan(ctx, planID)
	if err != nil {
		return err
	}
	for _, c := range consumers {
		if err := s.gw.EnsureConsumer(ctx, c.ConsumerUsername, c.APIKey,
			apisix.RateLimit{Count: plan.Count, WindowSeconds: plan.WindowSeconds}); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 4: Implement `ConsumersForPlan` on the real `Repo`**

In `internal/subscriptions/repo.go`, add (after `ConsumersForProduct`):

```go
// ConsumersForPlan returns the credentials of every application with an active
// subscription on the plan (used to re-apply new rate limits to live consumers).
func (r *Repo) ConsumersForPlan(ctx context.Context, planID int64) ([]Credential, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT DISTINCT c.application_id, c.api_key, c.consumer_username
		 FROM subscriptions s
		 JOIN credentials c ON c.application_id = s.application_id
		 WHERE s.plan_id=$1 AND s.status='active'`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Credential
	for rows.Next() {
		var c Credential
		if err := rows.Scan(&c.ApplicationID, &c.APIKey, &c.ConsumerUsername); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/subscriptions/ -v`
Expected: PASS — new test passes; all existing subscriptions tests still pass; `memStore` and `*Repo` both satisfy the extended `Store` (the `var _ Store = (*Repo)(nil)` assertion compiles).

Also run `go build ./...` (the admin package's existing `Provisioner` interface is unaffected; nothing else implements `subscriptions.Store`).

- [ ] **Step 6: Commit**

```bash
git add internal/subscriptions/
git commit -m "feat(subscriptions): ReprovisionPlan to re-apply plan limits to live consumers

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: Admin `Plan` type + validation

**Files:**
- Create: `internal/admin/plan.go`
- Test: `internal/admin/plan_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/admin/plan_test.go`:

```go
package admin

import "testing"

func TestPlanValidate(t *testing.T) {
	base := Plan{Name: "Silver", RateLimit: 100, WindowSeconds: 60}

	cases := []struct {
		name    string
		mutate  func(p *Plan)
		wantErr bool
	}{
		{"valid", func(p *Plan) {}, false},
		{"missing name", func(p *Plan) { p.Name = "" }, true},
		{"blank name", func(p *Plan) { p.Name = "   " }, true},
		{"zero rate", func(p *Plan) { p.RateLimit = 0 }, true},
		{"negative rate", func(p *Plan) { p.RateLimit = -5 }, true},
		{"zero window", func(p *Plan) { p.WindowSeconds = 0 }, true},
		{"negative window", func(p *Plan) { p.WindowSeconds = -1 }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := base
			tc.mutate(&p)
			msg := p.validate()
			if tc.wantErr && msg == "" {
				t.Fatal("expected validation error, got none")
			}
			if !tc.wantErr && msg != "" {
				t.Fatalf("unexpected validation error: %s", msg)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/admin/ -run TestPlanValidate -v`
Expected: failure — `Plan` undefined.

- [ ] **Step 3: Write the type + validation**

Create `internal/admin/plan.go`:

```go
package admin

import "strings"

// Plan is a rate-limit tier as managed by an admin. Its JSON shape matches
// plans.Plan so the frontend can reuse one type across read and admin APIs.
type Plan struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	RateLimit     int    `json:"rateLimit"`
	WindowSeconds int    `json:"windowSeconds"`
}

// validate returns "" when the plan is valid, otherwise a human-readable reason.
func (p Plan) validate() string {
	if strings.TrimSpace(p.Name) == "" {
		return "name is required"
	}
	if p.RateLimit <= 0 {
		return "rateLimit must be greater than zero"
	}
	if p.WindowSeconds <= 0 {
		return "windowSeconds must be greater than zero"
	}
	return ""
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/admin/ -run TestPlanValidate -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/admin/plan.go internal/admin/plan_test.go
git commit -m "feat(admin): Plan type + validation

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: Admin `PlanRepo` (SQL)

Thin SQL repo (no hermetic DB test; logic is unit-tested at the service layer in Task 4).

**Files:**
- Create: `internal/admin/plan_repo.go`

- [ ] **Step 1: Write the repo**

Create `internal/admin/plan_repo.go`:

```go
package admin

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Plan-specific sentinels (product sentinels live in repo.go).
var (
	ErrPlanNotFound = errors.New("admin: plan not found")
	ErrPlanNameTaken = errors.New("admin: plan name already exists")
	ErrPlanInUse     = errors.New("admin: plan is referenced by subscriptions")
)

// PlanRepo is the SQL store for admin plan management.
type PlanRepo struct{ pool *pgxpool.Pool }

func NewPlanRepo(pool *pgxpool.Pool) *PlanRepo { return &PlanRepo{pool: pool} }

const planCols = `id, name, rate_limit_count, rate_limit_window_s`

func scanPlan(row pgx.Row) (Plan, error) {
	var p Plan
	err := row.Scan(&p.ID, &p.Name, &p.RateLimit, &p.WindowSeconds)
	return p, err
}

func (r *PlanRepo) ListPlans(ctx context.Context) ([]Plan, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+planCols+` FROM plans ORDER BY rate_limit_count ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Plan
	for rows.Next() {
		p, err := scanPlan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *PlanRepo) GetPlan(ctx context.Context, id int64) (Plan, error) {
	p, err := scanPlan(r.pool.QueryRow(ctx, `SELECT `+planCols+` FROM plans WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Plan{}, ErrPlanNotFound
	}
	return p, err
}

func (r *PlanRepo) CreatePlan(ctx context.Context, p Plan) (Plan, error) {
	created, err := scanPlan(r.pool.QueryRow(ctx,
		`INSERT INTO plans(name, rate_limit_count, rate_limit_window_s)
		 VALUES($1,$2,$3) RETURNING `+planCols,
		p.Name, p.RateLimit, p.WindowSeconds))
	if isUniqueViolation(err) {
		return Plan{}, ErrPlanNameTaken
	}
	return created, err
}

func (r *PlanRepo) UpdatePlan(ctx context.Context, p Plan) (Plan, error) {
	updated, err := scanPlan(r.pool.QueryRow(ctx,
		`UPDATE plans SET name=$2, rate_limit_count=$3, rate_limit_window_s=$4
		 WHERE id=$1 RETURNING `+planCols,
		p.ID, p.Name, p.RateLimit, p.WindowSeconds))
	if errors.Is(err, pgx.ErrNoRows) {
		return Plan{}, ErrPlanNotFound
	}
	if isUniqueViolation(err) {
		return Plan{}, ErrPlanNameTaken
	}
	return updated, err
}

func (r *PlanRepo) DeletePlan(ctx context.Context, id int64) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM plans WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrPlanNotFound
	}
	return nil
}

func (r *PlanRepo) CountSubscriptionsForPlan(ctx context.Context, planID int64) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM subscriptions WHERE plan_id=$1`, planID).Scan(&n)
	return n, err
}
```

> **Note:** `isUniqueViolation` is already defined in `internal/admin/repo.go` (same package) — reuse it, do not redefine. `CountSubscriptionsForPlan` counts ALL subscriptions (any status), because `subscriptions.plan_id` has a foreign key with no cascade — any referencing row (active, pending, or rejected) blocks deletion at the DB, so the 409 guard must match that.

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/admin/` then `go vet ./internal/admin/`
Expected: clean. Run `go test ./internal/admin/` — existing tests still pass.

- [ ] **Step 3: Commit**

```bash
git add internal/admin/plan_repo.go
git commit -m "feat(admin): plan repo (CRUD + subscription-reference count)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: Admin `PlanService`

**Files:**
- Create: `internal/admin/plan_service.go`
- Test: `internal/admin/plan_service_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/admin/plan_service_test.go`:

```go
package admin

import (
	"context"
	"errors"
	"testing"
)

type fakePlanStore struct {
	plans   map[int64]Plan
	counts  map[int64]int
	deleted map[int64]bool
}

func newFakePlanStore() *fakePlanStore {
	return &fakePlanStore{plans: map[int64]Plan{}, counts: map[int64]int{}, deleted: map[int64]bool{}}
}

func (f *fakePlanStore) ListPlans(_ context.Context) ([]Plan, error) {
	out := []Plan{}
	for _, p := range f.plans {
		out = append(out, p)
	}
	return out, nil
}
func (f *fakePlanStore) GetPlan(_ context.Context, id int64) (Plan, error) {
	p, ok := f.plans[id]
	if !ok {
		return Plan{}, ErrPlanNotFound
	}
	return p, nil
}
func (f *fakePlanStore) CreatePlan(_ context.Context, p Plan) (Plan, error) {
	p.ID = int64(len(f.plans) + 1)
	f.plans[p.ID] = p
	return p, nil
}
func (f *fakePlanStore) UpdatePlan(_ context.Context, p Plan) (Plan, error) {
	if _, ok := f.plans[p.ID]; !ok {
		return Plan{}, ErrPlanNotFound
	}
	f.plans[p.ID] = p
	return p, nil
}
func (f *fakePlanStore) DeletePlan(_ context.Context, id int64) error {
	if _, ok := f.plans[id]; !ok {
		return ErrPlanNotFound
	}
	delete(f.plans, id)
	f.deleted[id] = true
	return nil
}
func (f *fakePlanStore) CountSubscriptionsForPlan(_ context.Context, id int64) (int, error) {
	return f.counts[id], nil
}

type fakePlanProv struct{ reprovisioned []int64 }

func (f *fakePlanProv) ReprovisionPlan(_ context.Context, id int64) error {
	f.reprovisioned = append(f.reprovisioned, id)
	return nil
}

func TestPlanUpdateReprovisionsWhenLimitsChange(t *testing.T) {
	store := newFakePlanStore()
	store.plans[1] = Plan{ID: 1, Name: "Silver", RateLimit: 100, WindowSeconds: 60}
	prov := &fakePlanProv{}
	svc := NewPlanService(store, prov)

	upd := Plan{ID: 1, Name: "Silver", RateLimit: 200, WindowSeconds: 60}
	if _, err := svc.Update(context.Background(), upd); err != nil {
		t.Fatal(err)
	}
	if len(prov.reprovisioned) != 1 || prov.reprovisioned[0] != 1 {
		t.Fatalf("expected reprovision of plan 1, got %v", prov.reprovisioned)
	}
}

func TestPlanUpdateNoReprovisionWhenOnlyNameChanges(t *testing.T) {
	store := newFakePlanStore()
	store.plans[1] = Plan{ID: 1, Name: "Silver", RateLimit: 100, WindowSeconds: 60}
	prov := &fakePlanProv{}
	svc := NewPlanService(store, prov)

	upd := Plan{ID: 1, Name: "Argent", RateLimit: 100, WindowSeconds: 60}
	if _, err := svc.Update(context.Background(), upd); err != nil {
		t.Fatal(err)
	}
	if len(prov.reprovisioned) != 0 {
		t.Fatalf("expected no reprovision on name-only change, got %v", prov.reprovisioned)
	}
}

func TestPlanDeleteBlockedWhenInUse(t *testing.T) {
	store := newFakePlanStore()
	store.plans[1] = Plan{ID: 1, Name: "Silver", RateLimit: 100, WindowSeconds: 60}
	store.counts[1] = 3
	prov := &fakePlanProv{}
	svc := NewPlanService(store, prov)

	err := svc.Delete(context.Background(), 1)
	if !errors.Is(err, ErrPlanInUse) {
		t.Fatalf("err = %v, want ErrPlanInUse", err)
	}
	if store.deleted[1] {
		t.Fatal("plan should not have been deleted")
	}
}

func TestPlanDeleteSucceedsWhenUnused(t *testing.T) {
	store := newFakePlanStore()
	store.plans[1] = Plan{ID: 1, Name: "Silver", RateLimit: 100, WindowSeconds: 60}
	store.counts[1] = 0
	prov := &fakePlanProv{}
	svc := NewPlanService(store, prov)

	if err := svc.Delete(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if !store.deleted[1] {
		t.Fatal("plan should have been deleted")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/admin/ -run TestPlan -v`
Expected: compile failure — `NewPlanService` / `PlanService` undefined (`TestPlanValidate` from Task 2 also matches `-run TestPlan` and will fail to compile alongside).

- [ ] **Step 3: Write the service**

Create `internal/admin/plan_service.go`:

```go
package admin

import "context"

// PlanStore is the persistence surface the plan service needs (satisfied by *PlanRepo).
type PlanStore interface {
	ListPlans(ctx context.Context) ([]Plan, error)
	GetPlan(ctx context.Context, id int64) (Plan, error)
	CreatePlan(ctx context.Context, p Plan) (Plan, error)
	UpdatePlan(ctx context.Context, p Plan) (Plan, error)
	DeletePlan(ctx context.Context, id int64) error
	CountSubscriptionsForPlan(ctx context.Context, planID int64) (int, error)
}

// PlanProvisioner re-applies a plan's limits to live consumers (satisfied by
// *subscriptions.Service via ReprovisionPlan).
type PlanProvisioner interface {
	ReprovisionPlan(ctx context.Context, planID int64) error
}

// PlanService applies admin plan operations and keeps APISIX consumers in sync.
type PlanService struct {
	store PlanStore
	prov  PlanProvisioner
}

func NewPlanService(store PlanStore, prov PlanProvisioner) *PlanService {
	return &PlanService{store: store, prov: prov}
}

func (s *PlanService) List(ctx context.Context) ([]Plan, error) { return s.store.ListPlans(ctx) }

func (s *PlanService) Create(ctx context.Context, p Plan) (Plan, error) {
	return s.store.CreatePlan(ctx, p)
}

// Update persists changes and, when the rate limits changed, re-applies them to
// every active subscriber's consumer. A name-only edit triggers no provisioning.
func (s *PlanService) Update(ctx context.Context, p Plan) (Plan, error) {
	old, err := s.store.GetPlan(ctx, p.ID)
	if err != nil {
		return Plan{}, err
	}
	updated, err := s.store.UpdatePlan(ctx, p)
	if err != nil {
		return Plan{}, err
	}
	if updated.RateLimit != old.RateLimit || updated.WindowSeconds != old.WindowSeconds {
		if err := s.prov.ReprovisionPlan(ctx, p.ID); err != nil {
			return Plan{}, err
		}
	}
	return updated, nil
}

// Delete refuses (ErrPlanInUse) while any subscription references the plan;
// otherwise it removes the plan.
func (s *PlanService) Delete(ctx context.Context, id int64) error {
	n, err := s.store.CountSubscriptionsForPlan(ctx, id)
	if err != nil {
		return err
	}
	if n > 0 {
		return ErrPlanInUse
	}
	return s.store.DeletePlan(ctx, id)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/admin/ -v`
Expected: PASS (product tests from 4a + plan validation + the 4 new plan service tests).

Also `go build ./...`.

- [ ] **Step 5: Commit**

```bash
git add internal/admin/plan_service.go internal/admin/plan_service_test.go
git commit -m "feat(admin): plan service (reprovision on limit change, block delete when in use)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: Admin `PlanHandler` (`/api/admin/plans*`)

**Files:**
- Create: `internal/admin/plan_handler.go`
- Test: `internal/admin/plan_handler_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/admin/plan_handler_test.go`:

```go
package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakePlanService struct {
	plans     map[int64]Plan
	createErr error
	updateErr error
	deleteErr error
}

func (f *fakePlanService) List(_ context.Context) ([]Plan, error) {
	out := []Plan{}
	for _, p := range f.plans {
		out = append(out, p)
	}
	return out, nil
}
func (f *fakePlanService) Create(_ context.Context, p Plan) (Plan, error) {
	if f.createErr != nil {
		return Plan{}, f.createErr
	}
	p.ID = 1
	return p, nil
}
func (f *fakePlanService) Update(_ context.Context, p Plan) (Plan, error) {
	if f.updateErr != nil {
		return Plan{}, f.updateErr
	}
	return p, nil
}
func (f *fakePlanService) Delete(_ context.Context, id int64) error { return f.deleteErr }

func doPlan(h *PlanHandler, method, target string, body any) *httptest.ResponseRecorder {
	var rdr *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, target, rdr)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestPlanCreateValid(t *testing.T) {
	h := NewPlanHandler(&fakePlanService{plans: map[int64]Plan{}})
	rec := doPlan(h, http.MethodPost, "/api/admin/plans",
		Plan{Name: "Gold", RateLimit: 500, WindowSeconds: 60})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestPlanCreateInvalidReturns400(t *testing.T) {
	h := NewPlanHandler(&fakePlanService{plans: map[int64]Plan{}})
	rec := doPlan(h, http.MethodPost, "/api/admin/plans", Plan{Name: "Bad", RateLimit: 0, WindowSeconds: 60})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestPlanList(t *testing.T) {
	h := NewPlanHandler(&fakePlanService{plans: map[int64]Plan{1: {ID: 1, Name: "Free", RateLimit: 10, WindowSeconds: 60}}})
	rec := doPlan(h, http.MethodGet, "/api/admin/plans", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got []Plan
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body not a JSON array: %v", err)
	}
}

func TestPlanCreateNameTakenReturns409(t *testing.T) {
	h := NewPlanHandler(&fakePlanService{plans: map[int64]Plan{}, createErr: ErrPlanNameTaken})
	rec := doPlan(h, http.MethodPost, "/api/admin/plans",
		Plan{Name: "Silver", RateLimit: 100, WindowSeconds: 60})
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
}

func TestPlanUpdateNotFoundReturns404(t *testing.T) {
	h := NewPlanHandler(&fakePlanService{plans: map[int64]Plan{}, updateErr: ErrPlanNotFound})
	rec := doPlan(h, http.MethodPut, "/api/admin/plans/9",
		Plan{Name: "Silver", RateLimit: 100, WindowSeconds: 60})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestPlanDeleteInUseReturns409(t *testing.T) {
	h := NewPlanHandler(&fakePlanService{plans: map[int64]Plan{1: {ID: 1}}, deleteErr: ErrPlanInUse})
	rec := doPlan(h, http.MethodDelete, "/api/admin/plans/1", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
}

func TestPlanDeleteSuccessReturns204(t *testing.T) {
	h := NewPlanHandler(&fakePlanService{plans: map[int64]Plan{1: {ID: 1}}})
	rec := doPlan(h, http.MethodDelete, "/api/admin/plans/1", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/admin/ -run TestPlan -v`
Expected: compile failure — `PlanHandler`, `NewPlanHandler` undefined.

- [ ] **Step 3: Write the handler**

Create `internal/admin/plan_handler.go`:

```go
package admin

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"apisix-portal/internal/httpx"
)

// PlanAdminService is the surface the plan handler needs (satisfied by *PlanService).
type PlanAdminService interface {
	List(ctx context.Context) ([]Plan, error)
	Create(ctx context.Context, p Plan) (Plan, error)
	Update(ctx context.Context, p Plan) (Plan, error)
	Delete(ctx context.Context, id int64) error
}

type PlanHandler struct {
	svc    PlanAdminService
	router chi.Router
}

func NewPlanHandler(svc PlanAdminService) *PlanHandler {
	h := &PlanHandler{svc: svc, router: chi.NewRouter()}
	h.router.Get("/api/admin/plans", h.list)
	h.router.Post("/api/admin/plans", h.create)
	h.router.Put("/api/admin/plans/{id}", h.update)
	h.router.Delete("/api/admin/plans/{id}", h.delete)
	return h
}

func (h *PlanHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.router.ServeHTTP(w, r) }

func (h *PlanHandler) list(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List(r.Context())
	if err != nil {
		log.Printf("admin list plans: %v", err)
		httpx.Error(w, http.StatusInternalServerError, "failed to list plans")
		return
	}
	if items == nil {
		items = []Plan{}
	}
	httpx.JSON(w, http.StatusOK, items)
}

func (h *PlanHandler) create(w http.ResponseWriter, r *http.Request) {
	p, ok := decodePlan(w, r)
	if !ok {
		return
	}
	created, err := h.svc.Create(r.Context(), p)
	if errors.Is(err, ErrPlanNameTaken) {
		httpx.Error(w, http.StatusConflict, "plan name already exists")
		return
	}
	if err != nil {
		log.Printf("admin create plan: %v", err)
		httpx.Error(w, http.StatusInternalServerError, "failed to create plan")
		return
	}
	httpx.JSON(w, http.StatusCreated, created)
}

func (h *PlanHandler) update(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePlanID(w, r)
	if !ok {
		return
	}
	p, ok := decodePlan(w, r)
	if !ok {
		return
	}
	p.ID = id
	updated, err := h.svc.Update(r.Context(), p)
	if errors.Is(err, ErrPlanNotFound) {
		httpx.Error(w, http.StatusNotFound, "plan not found")
		return
	}
	if errors.Is(err, ErrPlanNameTaken) {
		httpx.Error(w, http.StatusConflict, "plan name already exists")
		return
	}
	if err != nil {
		log.Printf("admin update plan %d: %v", id, err)
		httpx.Error(w, http.StatusInternalServerError, "failed to update plan")
		return
	}
	httpx.JSON(w, http.StatusOK, updated)
}

func (h *PlanHandler) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePlanID(w, r)
	if !ok {
		return
	}
	err := h.svc.Delete(r.Context(), id)
	if errors.Is(err, ErrPlanNotFound) {
		httpx.Error(w, http.StatusNotFound, "plan not found")
		return
	}
	if errors.Is(err, ErrPlanInUse) {
		httpx.Error(w, http.StatusConflict, "plan is referenced by subscriptions")
		return
	}
	if err != nil {
		log.Printf("admin delete plan %d: %v", id, err)
		httpx.Error(w, http.StatusInternalServerError, "failed to delete plan")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parsePlanID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "bad plan id")
		return 0, false
	}
	return id, true
}

func decodePlan(w http.ResponseWriter, r *http.Request) (Plan, bool) {
	var p Plan
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid body")
		return Plan{}, false
	}
	if msg := p.validate(); msg != "" {
		httpx.Error(w, http.StatusBadRequest, msg)
		return Plan{}, false
	}
	return p, true
}
```

> **Note:** the product handler in `handler.go` already defines `parseID` and `decodeProduct`. This file uses distinct names (`parsePlanID`, `decodePlan`) to avoid redeclaration in the same package.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/admin/ -v`
Expected: PASS (all product + plan tests).

Also `go build ./...` and `gofmt -l internal/admin/plan_handler.go` (must print nothing).

- [ ] **Step 5: Commit**

```bash
git add internal/admin/plan_handler.go internal/admin/plan_handler_test.go
git commit -m "feat(admin): plan CRUD handler (/api/admin/plans)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 6: Wire plan admin into `cmd/portal/main.go`

**Files:**
- Modify: `cmd/portal/main.go`

- [ ] **Step 1: Construct the plan admin handler**

In `cmd/portal/main.go`, after the existing admin product wiring:
```go
	adminSvc := admin.NewService(admin.NewRepo(pool), subSvc)
	adminH := admin.NewHandler(adminSvc)
```
add:
```go
	planAdminSvc := admin.NewPlanService(admin.NewPlanRepo(pool), subSvc)
	planAdminH := admin.NewPlanHandler(planAdminSvc)
```

> `subSvc` (a `*subscriptions.Service`) satisfies `admin.PlanProvisioner` because Task 1 added `ReprovisionPlan` to it.

- [ ] **Step 2: Mount the plan admin routes**

After the existing product admin mounts:
```go
	mux.Handle("/api/admin/products", requireAdmin(adminH))
	mux.Handle("/api/admin/products/", requireAdmin(adminH))
```
add:
```go
	mux.Handle("/api/admin/plans", requireAdmin(planAdminH))
	mux.Handle("/api/admin/plans/", requireAdmin(planAdminH))
```

- [ ] **Step 3: Verify the whole module builds and all tests pass**

Run: `go build ./...`
Expected: success. If you get "*subscriptions.Service does not implement admin.PlanProvisioner", Task 1's `ReprovisionPlan` is missing — STOP and report.

Run: `go test ./internal/... ./cmd/...`
Expected: all packages pass.

Run: `gofmt -l cmd/portal/main.go` (must print nothing) and `go vet ./cmd/portal/`.

- [ ] **Step 4: Commit**

```bash
git add cmd/portal/main.go
git commit -m "feat(admin): wire plan admin behind RequireAdmin

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 7: Live smoke (optional, requires running stack)

Manual end-to-end check; skip if the Docker stack / Postgres is not running (hermetic tests cover the logic). Assumes you have an admin token `$TOKEN` (see Plan 4a Task 11 for obtaining one).

- [ ] **Step 1: List, create, edit, delete a plan**

```bash
# List (200, array of the 3 seeded plans)
curl -s -w '\n%{http_code}\n' localhost:8080/api/admin/plans -H "authorization: Bearer $TOKEN"

# Create (201)
curl -s -w '\n%{http_code}\n' localhost:8080/api/admin/plans \
  -H "authorization: Bearer $TOKEN" -H 'content-type: application/json' \
  -d '{"name":"Platinum","rateLimit":1000,"windowSeconds":60}'

# Invalid create (400): zero rate
curl -s -o /dev/null -w '%{http_code}\n' localhost:8080/api/admin/plans \
  -H "authorization: Bearer $TOKEN" -H 'content-type: application/json' \
  -d '{"name":"Bad","rateLimit":0,"windowSeconds":60}'

# Edit the new plan (200) — use its returned id
curl -s -w '\n%{http_code}\n' -X PUT localhost:8080/api/admin/plans/<id> \
  -H "authorization: Bearer $TOKEN" -H 'content-type: application/json' \
  -d '{"name":"Platinum","rateLimit":2000,"windowSeconds":60}'

# Delete the unused new plan (204)
curl -s -o /dev/null -w '%{http_code}\n' -X DELETE localhost:8080/api/admin/plans/<id> \
  -H "authorization: Bearer $TOKEN"

# Try deleting a seeded plan that has subscriptions → 409
curl -s -o /dev/null -w '%{http_code}\n' -X DELETE localhost:8080/api/admin/plans/1 \
  -H "authorization: Bearer $TOKEN"
```

Expected: `200`, `201`, `400`, `200`, `204`, and `409` for the in-use plan (if plan 1 has subscriptions; otherwise it returns `204`).

---

## Self-review notes (already applied)

- **Spec coverage:** 4b implements `GET/POST /api/admin/plans` + `PUT/DELETE /api/admin/plans/{id}` (Tasks 5–6); re-provision affected consumers on rate-limit edit (Tasks 1, 4); `409` delete-when-in-use (Tasks 3, 4, 5). Matches the admin spec's Plan-management section. Approval/UI remain later sub-plans.
- **Decision applied:** re-provision triggers only when `rate_limit_count` or `rate_limit_window_s` changes (name-only edits skip it) — tested in Task 4.
- **Delete guard scope:** counts ALL subscriptions referencing the plan (not just active), matching the DB foreign-key constraint that actually blocks deletion. Documented in Task 3.
- **Type consistency:** `Plan` fields/JSON (`rateLimit`, `windowSeconds`) match `plans.Plan`; `PlanStore`/`PlanProvisioner`/`PlanAdminService` method sets match `*PlanRepo`, `*subscriptions.Service` (`ReprovisionPlan`), and `*PlanService`; `ConsumersForPlan` returns `subscriptions.Credential`. `isUniqueViolation` reused from `repo.go` (not redefined). Handler helpers named `parsePlanID`/`decodePlan` to avoid clashing with the product handler's `parseID`/`decodeProduct`.
- **No placeholders:** every code step is complete and compilable with exact run commands and expected results.
