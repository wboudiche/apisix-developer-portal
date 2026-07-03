# Monetization SP1 — Priced Plans + Billing Ledger (backend) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give plans a price and record an invoice against the subscriber's team when a paid subscription is approved, settled by a built-in manual billing provider.

**Architecture:** `plans` gains `price_cents`/`currency`. A new `internal/billing` package (Repo + `BillingProvider` interface + `ManualProvider` + `Service`) owns a team billing account + invoices. `subscriptions.Approve` calls a nil-safe `Biller` hook (mirrors the email `Notifier`) that creates a `pending` invoice for a paid plan. Admin + team-scoped HTTP over invoices.

**Tech Stack:** Go 1.25 (pgx/pgxpool, chi), Postgres. Reuses `internal/i18n` for validation error keys, `internal/httpx` for responses.

## Global Constraints

- Money is integer **minor units** (`price_cents INTEGER`, CHECK `>= 0`) + a 3-letter ISO `currency` (default `'EUR'`). **`price_cents = 0` = a free plan** (no billing).
- Billing entity = the **team**. One `billing_accounts` row per team (lazy). Invoices snapshot `plan_name`/`price_cents`/`currency`.
- Invoice status ∈ `'pending'|'paid'|'void'`; `MarkPaid` only from `pending`; illegal transitions → `ErrInvalidTransition` (→ 409).
- Bill-after-provision: the `Approve` hook runs AFTER activation; it is **synchronous + error-returning** and **idempotent** (one pending invoice per subscription).
- Go tests: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/... ./cmd/...`; `gofmt -w`; `go vet ./...`.
- Validation errors use `httpx.ErrorT(w, r, status, key)` with i18n keys; the `{"error":…}` shape is unchanged.

---

## Task 1: Plan price fields (migration + plans/admin packages)

**Files:**
- Create: `internal/db/migrations/0016_billing.sql`
- Modify: `internal/plans/plan.go`, `internal/plans/repo.go`, `internal/admin/plan.go`, `internal/admin/plan_repo.go`
- Test: `internal/plans/repo_test.go` (add), `internal/admin/plan_repo_test.go` or `internal/admin/repo_test.go` (add a case)

**Interfaces:**
- Produces: `plans.Plan.PriceCents int` (`json:"priceCents"`) + `.Currency string` (`json:"currency"`); `admin.Plan.PriceCents`/`.Currency` + `validate()` covering them. Migration `0016` also creates `billing_accounts` + `invoices` (used by Task 2).

- [ ] **Step 1: Write the migration**

Create `internal/db/migrations/0016_billing.sql`:
```sql
ALTER TABLE plans
  ADD COLUMN price_cents INTEGER NOT NULL DEFAULT 0 CHECK (price_cents >= 0),
  ADD COLUMN currency TEXT NOT NULL DEFAULT 'EUR';

CREATE TABLE billing_accounts (
  id         BIGSERIAL PRIMARY KEY,
  team_id    BIGINT NOT NULL UNIQUE REFERENCES teams(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE invoices (
  id                 BIGSERIAL PRIMARY KEY,
  billing_account_id BIGINT NOT NULL REFERENCES billing_accounts(id) ON DELETE CASCADE,
  team_id            BIGINT NOT NULL,
  subscription_id    BIGINT NOT NULL REFERENCES subscriptions(id) ON DELETE CASCADE,
  plan_name          TEXT NOT NULL,
  price_cents        INTEGER NOT NULL,
  currency           TEXT NOT NULL,
  status             TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','paid','void')),
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  paid_at            TIMESTAMPTZ NULL
);
CREATE INDEX idx_invoices_team_created ON invoices(team_id, created_at DESC);
```

- [ ] **Step 2: Write the failing plans-repo test**

Add to `internal/plans/repo_test.go` (create if absent; match the DB-test dialing pattern used by other repo tests — `pgxpool.New(ctx, os.Getenv("DATABASE_URL"))`, skip if unset):
```go
func TestListExposesPrice(t *testing.T) {
	pool := dialTestPool(t) // match the existing helper/inline dial in this package's tests
	// seed a priced plan
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO plans(name, rate_limit_count, rate_limit_window_s, price_cents, currency)
		 VALUES('PriceTest', 10, 60, 2900, 'EUR') RETURNING id`).Scan(&id)
	if err != nil { t.Fatalf("seed: %v", err) }
	defer pool.Exec(context.Background(), `DELETE FROM plans WHERE id=$1`, id)

	p, err := plans.NewRepo(pool).GetByID(context.Background(), id)
	if err != nil { t.Fatalf("getbyid: %v", err) }
	if p.PriceCents != 2900 || p.Currency != "EUR" {
		t.Fatalf("price=%d cur=%q, want 2900/EUR", p.PriceCents, p.Currency)
	}
}
```

- [ ] **Step 3: Run to verify it fails**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/plans/ -run TestListExposesPrice -v`
Expected: FAIL — `PriceCents`/`Currency` undefined.

- [ ] **Step 4: Add the fields to `plans.Plan` + repo**

`internal/plans/plan.go`:
```go
type Plan struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	RateLimit     int    `json:"rateLimit"`
	WindowSeconds int    `json:"windowSeconds"`
	PriceCents    int    `json:"priceCents"`
	Currency      string `json:"currency"`
}
```
In `internal/plans/repo.go`, extend BOTH SELECTs (`List` and `GetByID`) and their `Scan`s:
```go
// List:
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, rate_limit_count, rate_limit_window_s, price_cents, currency FROM plans
		 ORDER BY rate_limit_count ASC LIMIT $1 OFFSET $2`, p.Limit, p.Offset)
	// ... in the scan loop:
	//   err := rows.Scan(&pl.ID, &pl.Name, &pl.RateLimit, &pl.WindowSeconds, &pl.PriceCents, &pl.Currency)

// GetByID:
	err := r.pool.QueryRow(ctx,
		`SELECT id, name, rate_limit_count, rate_limit_window_s, price_cents, currency FROM plans WHERE id=$1`, id,
	).Scan(&p.ID, &p.Name, &p.RateLimit, &p.WindowSeconds, &p.PriceCents, &p.Currency)
```
(Preserve the exact surrounding code — only the column list + Scan args change.)

- [ ] **Step 5: Add price to the admin plan (struct + validate + repo)**

`internal/admin/plan.go` — add fields + validation:
```go
type Plan struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	RateLimit     int    `json:"rateLimit"`
	WindowSeconds int    `json:"windowSeconds"`
	PriceCents    int    `json:"priceCents"`
	Currency      string `json:"currency"`
}

func (p Plan) validate() string {
	if strings.TrimSpace(p.Name) == "" {
		return "common.nameRequired"
	}
	if p.RateLimit <= 0 {
		return "admin.plan.badRateLimit"
	}
	if p.WindowSeconds <= 0 {
		return "admin.plan.badWindowSeconds"
	}
	if p.PriceCents < 0 {
		return "admin.plan.badPrice"
	}
	if !validCurrency(p.Currency) {
		return "admin.plan.badCurrency"
	}
	return ""
}

// validCurrency accepts exactly three ASCII uppercase letters (ISO 4217 shape).
func validCurrency(c string) bool {
	if len(c) != 3 {
		return false
	}
	for _, r := range c {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}
```
Add the two i18n keys to BOTH catalogs — `internal/i18n/catalog_en.go`: `"admin.plan.badPrice": "price must be zero or positive",` + `"admin.plan.badCurrency": "currency must be a 3-letter code",`; `internal/i18n/catalog_fr.go`: `"admin.plan.badPrice": "le prix doit être positif ou nul",` + `"admin.plan.badCurrency": "la devise doit être un code à 3 lettres",`.

In `internal/admin/plan_repo.go`, extend `planCols`, `scanPlan`, INSERT, UPDATE:
```go
const planCols = `id, name, rate_limit_count, rate_limit_window_s, price_cents, currency`

func scanPlan(row pgx.Row) (Plan, error) {
	var p Plan
	err := row.Scan(&p.ID, &p.Name, &p.RateLimit, &p.WindowSeconds, &p.PriceCents, &p.Currency)
	return p, err
}
// Create:
	`INSERT INTO plans(name, rate_limit_count, rate_limit_window_s, price_cents, currency)
	 VALUES($1,$2,$3,$4,$5) RETURNING `+planCols
	// args: p.Name, p.RateLimit, p.WindowSeconds, p.PriceCents, p.Currency
// Update:
	`UPDATE plans SET name=$2, rate_limit_count=$3, rate_limit_window_s=$4, price_cents=$5, currency=$6
	 WHERE id=$1 RETURNING `+planCols
	// args: id, p.Name, p.RateLimit, p.WindowSeconds, p.PriceCents, p.Currency
```
(Match the exact `scanPlan` signature already in the file — it may take `pgx.Row`; keep it.)

- [ ] **Step 6: Add an admin plan-repo test case**

Add a test that creates a plan with `PriceCents: 2900, Currency: "EUR"` via the admin `PlanRepo.Create`, reads it back, and asserts the price/currency round-trip; and a `validate()` unit test asserting `badPrice` for `-1` and `badCurrency` for `"eur"`/`"EURO"`. Match the existing admin test harness.

- [ ] **Step 7: Run tests + build**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/plans/ ./internal/admin/ ./internal/i18n/ && go build ./... && go vet ./internal/plans/ ./internal/admin/`
Expected: PASS (i18n parity test still green with the 2 new keys).

- [ ] **Step 8: Commit**

```bash
gofmt -w internal/plans/ internal/admin/ internal/i18n/
git add internal/db/migrations/0016_billing.sql internal/plans/ internal/admin/ internal/i18n/
git commit -m "feat(billing): plan price_cents/currency + billing_accounts/invoices schema"
```

---

## Task 2: The `internal/billing` package (domain)

**Files:**
- Create: `internal/billing/invoice.go`, `internal/billing/repo.go`, `internal/billing/provider.go`, `internal/billing/service.go`
- Test: `internal/billing/service_test.go`, `internal/billing/repo_test.go`

**Interfaces:**
- Consumes: migration `0016` tables (Task 1); `plans`/`subscriptions`/`applications`/`team_members` tables.
- Produces: `billing.Invoice`; `billing.NewRepo(pool)`; `billing.ManualProvider{}`; `billing.NewService(repo, provider)`; `Service.SubscriptionActivated(ctx, appID, subID, planID int64) error`; `Service.MarkPaid(ctx, id)`; `Service.Void(ctx, id)`; `Service.ListForUser(ctx, userID)`; `Service.ListAll(ctx, status)`; `Service.Get(ctx, id)`; `billing.ErrInvalidTransition`, `billing.ErrNotFound`.

- [ ] **Step 1: Write the failing service/repo tests**

Create `internal/billing/service_test.go` (DB-backed; dial `DATABASE_URL`, skip if unset; seed a team + app + plan + a pending subscription with helper SQL):
```go
func TestSubscriptionActivatedCreatesInvoiceForPaidPlan(t *testing.T) {
	pool := dial(t); ctx := context.Background()
	teamID, appID, subID := seedTeamAppSub(t, pool)      // helper: creates team, app(team_id), pending sub
	planID := seedPlan(t, pool, "Gold", 2900, "EUR")     // helper: INSERT plans ... RETURNING id
	linkSubToPlan(t, pool, subID, planID)                // set subscriptions.plan_id

	svc := billing.NewService(billing.NewRepo(pool), billing.ManualProvider{})
	if err := svc.SubscriptionActivated(ctx, appID, subID, planID); err != nil {
		t.Fatalf("activate: %v", err)
	}
	inv := onlyInvoiceForSub(t, pool, subID)             // helper: SELECT the invoice row
	if inv.PriceCents != 2900 || inv.PlanName != "Gold" || inv.Status != "pending" || inv.TeamID != teamID {
		t.Fatalf("bad invoice: %+v", inv)
	}

	// idempotent: a second activation does not duplicate the pending invoice
	if err := svc.SubscriptionActivated(ctx, appID, subID, planID); err != nil {
		t.Fatalf("activate2: %v", err)
	}
	if n := countInvoicesForSub(t, pool, subID); n != 1 {
		t.Fatalf("invoice count = %d, want 1 (idempotent)", n)
	}

	// snapshot: changing the plan price does NOT change the invoice
	pool.Exec(ctx, `UPDATE plans SET price_cents=5000 WHERE id=$1`, planID)
	inv2 := onlyInvoiceForSub(t, pool, subID)
	if inv2.PriceCents != 2900 {
		t.Fatalf("snapshot broken: invoice price=%d, want 2900", inv2.PriceCents)
	}
}

func TestSubscriptionActivatedFreePlanNoInvoice(t *testing.T) {
	pool := dial(t); ctx := context.Background()
	_, appID, subID := seedTeamAppSub(t, pool)
	planID := seedPlan(t, pool, "Free", 0, "EUR")
	linkSubToPlan(t, pool, subID, planID)
	svc := billing.NewService(billing.NewRepo(pool), billing.ManualProvider{})
	if err := svc.SubscriptionActivated(ctx, appID, subID, planID); err != nil {
		t.Fatalf("activate: %v", err)
	}
	if n := countInvoicesForSub(t, pool, subID); n != 0 {
		t.Fatalf("free plan created %d invoices, want 0", n)
	}
}

func TestMarkPaidAndVoidTransitions(t *testing.T) {
	pool := dial(t); ctx := context.Background()
	_, appID, subID := seedTeamAppSub(t, pool)
	planID := seedPlan(t, pool, "Gold", 2900, "EUR")
	linkSubToPlan(t, pool, subID, planID)
	svc := billing.NewService(billing.NewRepo(pool), billing.ManualProvider{})
	svc.SubscriptionActivated(ctx, appID, subID, planID)
	inv := onlyInvoiceForSub(t, pool, subID)

	if err := svc.MarkPaid(ctx, inv.ID); err != nil { t.Fatalf("markpaid: %v", err) }
	got, _ := svc.Get(ctx, inv.ID)
	if got.Status != "paid" || got.PaidAt == nil { t.Fatalf("not paid: %+v", got) }
	if err := svc.MarkPaid(ctx, inv.ID); err != billing.ErrInvalidTransition {
		t.Fatalf("double-pay err = %v, want ErrInvalidTransition", err)
	}
}
```
(Write the small `seed*`/`only*`/`count*` helpers in the test file with plain SQL. The seed must satisfy existing NOT NULL columns — an app needs `team_id`; a subscription needs its columns. Look at `internal/subscriptions`/`internal/notify` test seeds for the exact insert shape.)

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/billing/ -v`
Expected: FAIL — package/exports missing.

- [ ] **Step 3: Implement `invoice.go`**

`internal/billing/invoice.go`:
```go
// Package billing records plan pricing and invoices for paid subscriptions.
package billing

import (
	"errors"
	"time"
)

const (
	StatusPending = "pending"
	StatusPaid    = "paid"
	StatusVoid    = "void"
)

var (
	ErrInvalidTransition = errors.New("billing: invalid invoice status transition")
	ErrNotFound          = errors.New("billing: invoice not found")
)

type Invoice struct {
	ID               int64      `json:"id"`
	BillingAccountID int64      `json:"-"`
	TeamID           int64      `json:"teamId"`
	SubscriptionID   int64      `json:"subscriptionId"`
	PlanName         string     `json:"planName"`
	PriceCents       int        `json:"priceCents"`
	Currency         string     `json:"currency"`
	Status           string     `json:"status"`
	CreatedAt        time.Time  `json:"createdAt"`
	PaidAt           *time.Time `json:"paidAt"`
}
```

- [ ] **Step 4: Implement `provider.go`**

`internal/billing/provider.go`:
```go
package billing

import "context"

// BillingProvider settles an invoice with a payment backend. A real PSP creates
// a payment intent / checkout and returns its reference; the built-in
// ManualProvider records nothing external (the invoice stays pending until an
// admin marks it paid).
type BillingProvider interface {
	Charge(ctx context.Context, inv Invoice) (ref string, err error)
}

type ManualProvider struct{}

func (ManualProvider) Charge(ctx context.Context, inv Invoice) (string, error) { return "", nil }
```

- [ ] **Step 5: Implement `repo.go`**

`internal/billing/repo.go`:
```go
package billing

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

const invoiceCols = `id, billing_account_id, team_id, subscription_id, plan_name, price_cents, currency, status, created_at, paid_at`

func scanInvoice(row pgx.Row) (Invoice, error) {
	var v Invoice
	err := row.Scan(&v.ID, &v.BillingAccountID, &v.TeamID, &v.SubscriptionID,
		&v.PlanName, &v.PriceCents, &v.Currency, &v.Status, &v.CreatedAt, &v.PaidAt)
	return v, err
}

// PlanPricing returns the plan's snapshot pricing.
func (r *Repo) PlanPricing(ctx context.Context, planID int64) (name string, priceCents int, currency string, err error) {
	err = r.pool.QueryRow(ctx, `SELECT name, price_cents, currency FROM plans WHERE id=$1`, planID).
		Scan(&name, &priceCents, &currency)
	return
}

// TeamForApp returns the team that owns the app.
func (r *Repo) TeamForApp(ctx context.Context, appID int64) (int64, error) {
	var teamID int64
	err := r.pool.QueryRow(ctx, `SELECT team_id FROM applications WHERE id=$1`, appID).Scan(&teamID)
	return teamID, err
}

// EnsureAccount returns the team's billing account id, creating it if absent.
func (r *Repo) EnsureAccount(ctx context.Context, teamID int64) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx,
		`INSERT INTO billing_accounts(team_id) VALUES($1)
		 ON CONFLICT (team_id) DO UPDATE SET team_id=EXCLUDED.team_id
		 RETURNING id`, teamID).Scan(&id)
	return id, err
}

// PendingInvoiceExists reports whether a non-void invoice already exists for the
// subscription (idempotency guard for re-approval).
func (r *Repo) PendingInvoiceExists(ctx context.Context, subID int64) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM invoices WHERE subscription_id=$1 AND status <> 'void')`, subID).Scan(&exists)
	return exists, err
}

func (r *Repo) CreateInvoice(ctx context.Context, accountID, teamID, subID int64, planName string, priceCents int, currency string) (Invoice, error) {
	return scanInvoice(r.pool.QueryRow(ctx,
		`INSERT INTO invoices(billing_account_id, team_id, subscription_id, plan_name, price_cents, currency, status)
		 VALUES($1,$2,$3,$4,$5,$6,'pending') RETURNING `+invoiceCols,
		accountID, teamID, subID, planName, priceCents, currency))
}

func (r *Repo) Get(ctx context.Context, id int64) (Invoice, error) {
	v, err := scanInvoice(r.pool.QueryRow(ctx, `SELECT `+invoiceCols+` FROM invoices WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Invoice{}, ErrNotFound
	}
	return v, err
}

// MarkPaid flips a pending invoice to paid; any other current status → ErrInvalidTransition.
func (r *Repo) MarkPaid(ctx context.Context, id int64) error {
	ct, err := r.pool.Exec(ctx,
		`UPDATE invoices SET status='paid', paid_at=now() WHERE id=$1 AND status='pending'`, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return r.transitionError(ctx, id)
	}
	return nil
}

// Void flips a pending invoice to void; any other current status → ErrInvalidTransition.
func (r *Repo) Void(ctx context.Context, id int64) error {
	ct, err := r.pool.Exec(ctx, `UPDATE invoices SET status='void' WHERE id=$1 AND status='pending'`, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return r.transitionError(ctx, id)
	}
	return nil
}

// transitionError distinguishes "no such invoice" from "wrong current status".
func (r *Repo) transitionError(ctx context.Context, id int64) error {
	if _, err := r.Get(ctx, id); errors.Is(err, ErrNotFound) {
		return ErrNotFound
	}
	return ErrInvalidTransition
}

func (r *Repo) list(ctx context.Context, where string, args ...any) ([]Invoice, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+invoiceCols+` FROM invoices `+where+` ORDER BY created_at DESC, id DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Invoice
	for rows.Next() {
		v, err := scanInvoice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ListByTeams returns invoices for the given team ids (newest first).
func (r *Repo) ListByTeams(ctx context.Context, teamIDs []int64) ([]Invoice, error) {
	if len(teamIDs) == 0 {
		return nil, nil
	}
	return r.list(ctx, `WHERE team_id = ANY($1)`, teamIDs)
}

// ListAll returns every invoice, optionally filtered by status ("" = all).
func (r *Repo) ListAll(ctx context.Context, status string) ([]Invoice, error) {
	if status == "" {
		return r.list(ctx, ``)
	}
	return r.list(ctx, `WHERE status=$1`, status)
}

// TeamsForUser returns the ids of the teams the user belongs to.
func (r *Repo) TeamsForUser(ctx context.Context, userID int64) ([]int64, error) {
	rows, err := r.pool.Query(ctx, `SELECT team_id FROM team_members WHERE user_id=$1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
```

- [ ] **Step 6: Implement `service.go`**

`internal/billing/service.go`:
```go
package billing

import "context"

type Store interface {
	PlanPricing(ctx context.Context, planID int64) (string, int, string, error)
	TeamForApp(ctx context.Context, appID int64) (int64, error)
	EnsureAccount(ctx context.Context, teamID int64) (int64, error)
	PendingInvoiceExists(ctx context.Context, subID int64) (bool, error)
	CreateInvoice(ctx context.Context, accountID, teamID, subID int64, planName string, priceCents int, currency string) (Invoice, error)
	Get(ctx context.Context, id int64) (Invoice, error)
	MarkPaid(ctx context.Context, id int64) error
	Void(ctx context.Context, id int64) error
	ListByTeams(ctx context.Context, teamIDs []int64) ([]Invoice, error)
	ListAll(ctx context.Context, status string) ([]Invoice, error)
	TeamsForUser(ctx context.Context, userID int64) ([]int64, error)
}

type Service struct {
	store    Store
	provider BillingProvider
}

func NewService(store Store, provider BillingProvider) *Service {
	return &Service{store: store, provider: provider}
}

// SubscriptionActivated records a pending invoice for a newly-activated PAID
// subscription. Free plans (price 0) are a no-op. Idempotent per subscription.
// This IS the subscriptions.Biller method.
func (s *Service) SubscriptionActivated(ctx context.Context, appID, subID, planID int64) error {
	name, priceCents, currency, err := s.store.PlanPricing(ctx, planID)
	if err != nil {
		return err
	}
	if priceCents == 0 {
		return nil // free plan → no billing
	}
	exists, err := s.store.PendingInvoiceExists(ctx, subID)
	if err != nil {
		return err
	}
	if exists {
		return nil // idempotent: already invoiced
	}
	teamID, err := s.store.TeamForApp(ctx, appID)
	if err != nil {
		return err
	}
	accountID, err := s.store.EnsureAccount(ctx, teamID)
	if err != nil {
		return err
	}
	inv := Invoice{TeamID: teamID, SubscriptionID: subID, PlanName: name, PriceCents: priceCents, Currency: currency}
	if _, err := s.provider.Charge(ctx, inv); err != nil {
		return err
	}
	_, err = s.store.CreateInvoice(ctx, accountID, teamID, subID, name, priceCents, currency)
	return err
}

func (s *Service) MarkPaid(ctx context.Context, id int64) error { return s.store.MarkPaid(ctx, id) }
func (s *Service) Void(ctx context.Context, id int64) error     { return s.store.Void(ctx, id) }
func (s *Service) Get(ctx context.Context, id int64) (Invoice, error) { return s.store.Get(ctx, id) }
func (s *Service) ListAll(ctx context.Context, status string) ([]Invoice, error) {
	return s.store.ListAll(ctx, status)
}

// ListForUser returns invoices across all teams the user belongs to.
func (s *Service) ListForUser(ctx context.Context, userID int64) ([]Invoice, error) {
	teamIDs, err := s.store.TeamsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.store.ListByTeams(ctx, teamIDs)
}
```
(`*Repo` satisfies `Store` structurally.)

- [ ] **Step 7: Run to verify pass + build/vet**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/billing/ -v && go build ./... && go vet ./internal/billing/`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
gofmt -w internal/billing/
git add internal/billing/
git commit -m "feat(billing): billing domain — invoices, manual provider, service"
```

---

## Task 3: The `Approve` Biller hook + wiring

**Files:**
- Modify: `internal/subscriptions/service.go`, `internal/server/server.go`
- Test: `internal/subscriptions/service_test.go` (add) or the existing approve test file

**Interfaces:**
- Consumes: `billing.NewService`/`billing.NewRepo`/`billing.ManualProvider` (Task 2); `billing.Service.SubscriptionActivated` satisfies the new `Biller`.
- Produces: `subscriptions.Biller` interface + `(*Service).SetBiller`; the hook in `Approve`.

- [ ] **Step 1: Write the failing hook test**

Add to the subscriptions service test (a `fakeBiller` records the call args; drive an approval of a pending sub through the existing test harness — reuse whatever fake `Store`/gateway the approve tests already use):
```go
type fakeBiller struct{ calls [][3]int64 } // {appID, subID, planID}
func (f *fakeBiller) SubscriptionActivated(_ context.Context, appID, subID, planID int64) error {
	f.calls = append(f.calls, [3]int64{appID, subID, planID}); return nil
}

func TestApproveFiresBiller(t *testing.T) {
	svc, store := newApproveTestService(t) // existing helper that yields a Service + its fake store
	fb := &fakeBiller{}
	svc.SetBiller(fb)
	subID := store.seedPendingSub(t, /*appID*/ 11, /*productID*/ 22, /*planID*/ 33) // match existing seed helpers
	if err := svc.Approve(context.Background(), subID); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if len(fb.calls) != 1 || fb.calls[0] != [3]int64{11, subID, 33} {
		t.Fatalf("biller calls = %+v, want one {11,%d,33}", fb.calls, subID)
	}
}
```
(If the existing approve tests use a different construction, mirror it exactly; the point is: a `SetBiller` fake receives one call with `(rec.AppID, subID, rec.PlanID)` after a successful approve, for BOTH auth types. If both a key-auth and an oauth2 fixture exist, assert the fire in each.)

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/subscriptions/ -run TestApproveFiresBiller -v`
Expected: FAIL — `SetBiller`/`Biller` undefined.

- [ ] **Step 3: Add the `Biller` interface + setter**

In `internal/subscriptions/service.go`, next to the `Notifier` interface:
```go
// Biller records billing for a newly-activated paid subscription. Left unset
// (nil) = disabled. Synchronous; a returned error fails the approval.
type Biller interface {
	SubscriptionActivated(ctx context.Context, appID, subID, planID int64) error
}
```
Add a `biller Biller` field to the `Service` struct and:
```go
// SetBiller wires billing. Left unset (nil) = disabled.
func (s *Service) SetBiller(b Biller) { s.biller = b }
```

- [ ] **Step 4: Call the hook in BOTH `Approve` branches**

`Approve` has two success paths (oauth2 early-return; key-auth fall-through). In the **oauth2 branch**, immediately AFTER the `if s.notifier != nil { s.notifier.SubscriptionApproved(...) }` block and BEFORE `return nil`, insert:
```go
		if s.biller != nil {
			if err := s.biller.SubscriptionActivated(ctx, rec.AppID, subID, rec.PlanID); err != nil {
				return err
			}
		}
```
Insert the SAME block in the **key-auth branch**, after its `if s.notifier != nil { ... }` block and before that branch's final `return nil`. (Both branches already have `rec` and `subID` in scope.)

- [ ] **Step 5: Wire billing in `server.go`**

In `internal/server/server.go`, right after `subSvc := subscriptions.NewService(...)` (and its OIDC/notifier wiring, ~line 69-73), add:
```go
	subSvc.SetBiller(billing.NewService(billing.NewRepo(pool), billing.ManualProvider{}))
```
Add the import `apisix-portal/internal/billing`. (Billing is always on — the manual provider needs no config. Free plans make it inert on the common path.)

- [ ] **Step 6: Run tests + build**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/subscriptions/ && go build ./... && go vet ./internal/subscriptions/ ./internal/server/`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
gofmt -w internal/subscriptions/ internal/server/
git add internal/subscriptions/service.go internal/server/server.go internal/subscriptions/*_test.go
git commit -m "feat(billing): invoice paid subscriptions on approval via nil-safe Biller hook"
```

---

## Task 4: Billing HTTP (team + admin handlers + routes)

**Files:**
- Create: `internal/billing/handler.go`, `internal/billing/handler_test.go`
- Modify: `internal/server/server.go`

**Interfaces:**
- Consumes: `billing.Service` (Task 2), `auth.UserID`, `auth.RequireAuth`/`RequireAdmin`, `httpx`.
- Produces: `billing.NewTeamHandler(svc)` → `GET /api/billing/invoices`; `billing.NewAdminHandler(svc)` → `GET /api/admin/invoices`, `POST /api/admin/invoices/{id}/pay`, `POST /api/admin/invoices/{id}/void`.

- [ ] **Step 1: Write the failing handler tests**

Create `internal/billing/handler_test.go`. Cover: team list is scoped to the caller's teams (a user with no membership sees `[]`; the seeded team's member sees its invoice); admin list returns all + `?status=` filters; admin `pay` → 200 + status paid; a second `pay` → 409; `void` → 200; `pay` on a nonexistent id → 404. Use `httptest`, `auth.WithUserID` to inject the caller, and chi URL params via the router's ServeHTTP (mount the handler and issue real request paths so `{id}` resolves). Match the DB-backed seed helpers from Task 2.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/billing/ -run Handler -v`
Expected: FAIL — handlers undefined.

- [ ] **Step 3: Implement `handler.go`**

`internal/billing/handler.go`:
```go
package billing

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"apisix-portal/internal/auth"
	"apisix-portal/internal/httpx"
)

// TeamHandler serves the authenticated user's own team invoices.
type TeamHandler struct {
	svc    *Service
	router chi.Router
}

func NewTeamHandler(svc *Service) *TeamHandler {
	h := &TeamHandler{svc: svc, router: chi.NewRouter()}
	h.router.Get("/api/billing/invoices", h.listMine)
	return h
}
func (h *TeamHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.router.ServeHTTP(w, r) }

func (h *TeamHandler) listMine(w http.ResponseWriter, r *http.Request) {
	uid := auth.UserID(r.Context())
	if uid == 0 {
		httpx.ErrorT(w, r, http.StatusUnauthorized, "auth.middleware.missingToken")
		return
	}
	invoices, err := h.svc.ListForUser(r.Context(), uid)
	if err != nil {
		httpx.ErrorT(w, r, http.StatusInternalServerError, "billing.listFailed")
		return
	}
	if invoices == nil {
		invoices = []Invoice{}
	}
	httpx.JSON(w, http.StatusOK, invoices)
}

// AdminHandler serves all invoices + settlement actions (mounted behind requireAdmin).
type AdminHandler struct {
	svc    *Service
	router chi.Router
}

func NewAdminHandler(svc *Service) *AdminHandler {
	h := &AdminHandler{svc: svc, router: chi.NewRouter()}
	h.router.Get("/api/admin/invoices", h.listAll)
	h.router.Post("/api/admin/invoices/{id}/pay", h.pay)
	h.router.Post("/api/admin/invoices/{id}/void", h.void)
	return h
}
func (h *AdminHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.router.ServeHTTP(w, r) }

func (h *AdminHandler) listAll(w http.ResponseWriter, r *http.Request) {
	invoices, err := h.svc.ListAll(r.Context(), r.URL.Query().Get("status"))
	if err != nil {
		httpx.ErrorT(w, r, http.StatusInternalServerError, "billing.listFailed")
		return
	}
	if invoices == nil {
		invoices = []Invoice{}
	}
	httpx.JSON(w, http.StatusOK, invoices)
}

func (h *AdminHandler) pay(w http.ResponseWriter, r *http.Request)  { h.transition(w, r, h.svc.MarkPaid) }
func (h *AdminHandler) void(w http.ResponseWriter, r *http.Request) { h.transition(w, r, h.svc.Void) }

func (h *AdminHandler) transition(w http.ResponseWriter, r *http.Request, fn func(ctx context.Context, id int64) error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.ErrorT(w, r, http.StatusBadRequest, "billing.badInvoiceID")
		return
	}
	switch err := fn(r.Context(), id); {
	case err == nil:
		w.WriteHeader(http.StatusOK)
	case errors.Is(err, ErrNotFound):
		httpx.ErrorT(w, r, http.StatusNotFound, "billing.invoiceNotFound")
	case errors.Is(err, ErrInvalidTransition):
		httpx.ErrorT(w, r, http.StatusConflict, "billing.invalidTransition")
	default:
		httpx.ErrorT(w, r, http.StatusInternalServerError, "billing.actionFailed")
	}
}
```

- [ ] **Step 4: Add the billing i18n keys**

Add these keys to BOTH catalogs (`internal/i18n/catalog_en.go` / `catalog_fr.go`):
- `billing.listFailed`: `"could not list invoices"` / `"impossible de lister les factures"`
- `billing.badInvoiceID`: `"bad invoice id"` / `"identifiant de facture invalide"`
- `billing.invoiceNotFound`: `"invoice not found"` / `"facture introuvable"`
- `billing.invalidTransition`: `"invoice cannot change to that status"` / `"la facture ne peut pas passer à ce statut"`
- `billing.actionFailed`: `"invoice action failed"` / `"l'action sur la facture a échoué"`

- [ ] **Step 5: Mount the routes in `server.go`**

In `internal/server/server.go`, construct one billing service reused for the hook + the handlers (replace the inline `SetBiller` from Task 3 Step 5 with a named var):
```go
	billingSvc := billing.NewService(billing.NewRepo(pool), billing.ManualProvider{})
	subSvc.SetBiller(billingSvc)
	billingTeamH := billing.NewTeamHandler(billingSvc)
	billingAdminH := billing.NewAdminHandler(billingSvc)
```
Mount (next to the other `mux.Handle` lines):
```go
	mux.Handle("/api/billing/invoices", requireAuth(billingTeamH))
	mux.Handle("/api/admin/invoices", requireAdmin(billingAdminH))
	mux.Handle("/api/admin/invoices/", requireAdmin(billingAdminH))
```

- [ ] **Step 6: Run tests + build**

Run: `DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/billing/ ./internal/i18n/ && go build ./... && go vet ./internal/billing/ ./internal/server/`
Expected: PASS (i18n parity green with the new billing.* keys).

- [ ] **Step 7: Commit**

```bash
gofmt -w internal/billing/ internal/server/ internal/i18n/
git add internal/billing/ internal/server/server.go internal/i18n/
git commit -m "feat(billing): team + admin invoice HTTP endpoints"
```

---

## Task 5: Full suite + live verification

**Files:** none (verification).

- [ ] **Step 1: Full backend suite**

Run:
```bash
DATABASE_URL='postgres://portal:portal@localhost:5432/portal?sslmode=disable' go test ./internal/... ./cmd/... && go vet ./...
```
Expected: green.

- [ ] **Step 2: Live**

Bring up the stack (`make up`; `make run` on `PORTAL_ADDR` if `:8080` is held). As admin:
1. Create a **paid** plan: `POST /api/admin/plans` `{"name":"Gold","rateLimit":100,"windowSeconds":60,"priceCents":2900,"currency":"EUR"}` → 201 with `priceCents:2900`.
2. Confirm `GET /api/plans` now returns `priceCents`/`currency`.
3. As a developer: create an app, `POST /api/applications/{appId}/subscriptions {"productId":<published>,"planId":<gold>}` → pending.
4. As admin: `POST /api/admin/subscriptions/{subId}/approve` → 200.
5. `GET /api/admin/invoices?status=pending` → the invoice, `priceCents:2900`, `planName:"Gold"`, `status:"pending"`, right `teamId`.
6. `GET /api/billing/invoices` as that developer → sees the invoice; as an unrelated user → does NOT.
7. `POST /api/admin/invoices/{id}/pay` → 200; re-list → `paid` + `paidAt`; `pay` again → 409.
8. Repeat with a **free** plan (`priceCents:0`) → approve → NO invoice created.
Also validate: `POST /api/admin/plans {... "currency":"EURO"}` → 400; `{... "priceCents":-1}` → 400.

- [ ] **Step 3: No commit** (verification; note results in the ledger).

---

## Self-Review notes

- **Spec coverage:** plan `price_cents`/`currency` + free=0 (T1) ✅; `billing_accounts`/`invoices` with snapshot (T1 schema, T2 writes) ✅; `internal/billing` domain + `BillingProvider`/`ManualProvider` + `Service` (T2) ✅; nil-safe synchronous idempotent `Biller` hook in `Approve` for both auth types (T3) ✅; admin plan price validation + `GET /api/admin/invoices` + pay/void + team `GET /api/billing/invoices` + plans expose price (T1/T4) ✅; team-scoping + transitions + snapshot + free-plan tests (T2/T4) + live (T5) ✅.
- **Type consistency:** `Service.SubscriptionActivated(ctx, appID, subID, planID) error` IS the `Biller` method (T2 defines, T3's interface matches, server wires `billingSvc` as both hook + handler backend). `Repo` satisfies `Store`; `*Service` backs both handlers. Invoice JSON (`priceCents`/`teamId`/`status`/`paidAt`) is the shape SP2 will consume.
- **Implementer notes:** Approve has NO single convergence point — the hook MUST be inserted in both the oauth2 and key-auth success paths (Task 3 Step 4). `EnsureAccount` uses `ON CONFLICT (team_id)` so concurrent first-activations don't error. The idempotency guard treats any non-void invoice as "already billed" (a re-approve won't double-invoice; a voided invoice would allow a fresh one). Keep `price_cents`/`plan_name`/`currency` snapshotted on the invoice — never join back to `plans` for historical amounts.
