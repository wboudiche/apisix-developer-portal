package billing_test

import (
	"context"
	"os"
	"testing"
	"time"

	"apisix-portal/internal/billing"
	"apisix-portal/internal/db"

	"github.com/jackc/pgx/v5/pgxpool"
)

// dial connects to the test database and applies migrations, matching the
// dial pattern used by internal/subscriptions and internal/notify. It skips
// the test (rather than failing) when DATABASE_URL is unset, so unit-test-only
// runs still pass.
func dial(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set; skipping DB-backed billing test")
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, url)
	if err != nil {
		t.Skipf("no database: %v", err)
	}
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// seedTeamAppSub seeds a user + personal team + an application (team_id set)
// + a product + a placeholder plan + a PENDING subscription tying them
// together. It returns (teamID, appID, subID). The subscription's plan_id
// initially points at the throwaway placeholder plan (subscriptions.plan_id
// is NOT NULL / FK'd) — tests call linkSubToPlan to repoint it at the plan
// under test.
func seedTeamAppSub(t *testing.T, pool *pgxpool.Pool) (teamID, appID, subID int64) {
	t.Helper()
	ctx := context.Background()
	suffix := time.Now().Format("150405.000000000")

	var uid int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users(email,password_hash,name) VALUES($1,'x','Dev') RETURNING id`,
		"billing-owner+"+suffix+"@example.com").Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO teams(name,personal) VALUES($1,true) RETURNING id`,
		"BillingTeam "+suffix).Scan(&teamID); err != nil {
		t.Fatalf("seed team: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO team_members(team_id,user_id,role) VALUES($1,$2,'owner')`,
		teamID, uid); err != nil {
		t.Fatalf("seed team member: %v", err)
	}

	var productID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO api_products(name,slug,category,context_path,published)
		 VALUES($1,$2,'Test','/billing-test',true) RETURNING id`,
		"BillingProduct+"+suffix, "billing-product-"+suffix).Scan(&productID); err != nil {
		t.Fatalf("seed product: %v", err)
	}

	var placeholderPlanID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO plans(name,rate_limit_count,rate_limit_window_s) VALUES($1,100,60) RETURNING id`,
		"BillingPlaceholderPlan+"+suffix).Scan(&placeholderPlanID); err != nil {
		t.Fatalf("seed placeholder plan: %v", err)
	}

	if err := pool.QueryRow(ctx,
		`INSERT INTO applications(owner_id,name,team_id) VALUES($1,$2,$3) RETURNING id`,
		uid, "BillingApp "+suffix, teamID).Scan(&appID); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	if err := pool.QueryRow(ctx,
		`INSERT INTO subscriptions(application_id,api_product_id,plan_id,status) VALUES($1,$2,$3,'pending') RETURNING id`,
		appID, productID, placeholderPlanID).Scan(&subID); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM invoices WHERE subscription_id=$1`, subID)
		_, _ = pool.Exec(ctx, `DELETE FROM subscriptions WHERE id=$1`, subID)
		_, _ = pool.Exec(ctx, `DELETE FROM billing_accounts WHERE team_id=$1`, teamID)
		_, _ = pool.Exec(ctx, `DELETE FROM applications WHERE id=$1`, appID)
		_, _ = pool.Exec(ctx, `DELETE FROM plans WHERE id=$1`, placeholderPlanID)
		_, _ = pool.Exec(ctx, `DELETE FROM api_products WHERE id=$1`, productID)
		_, _ = pool.Exec(ctx, `DELETE FROM team_members WHERE team_id=$1`, teamID)
		_, _ = pool.Exec(ctx, `DELETE FROM teams WHERE id=$1`, teamID)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, uid)
	})
	return teamID, appID, subID
}

// seedPlan inserts a plan with the given pricing and returns its id. The plan
// is cleaned up at the end of the test.
func seedPlan(t *testing.T, pool *pgxpool.Pool, name string, priceCents int, currency string) int64 {
	t.Helper()
	ctx := context.Background()
	var planID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO plans(name,rate_limit_count,rate_limit_window_s,price_cents,currency)
		 VALUES($1,1000,60,$2,$3) RETURNING id`,
		name, priceCents, currency).Scan(&planID); err != nil {
		t.Fatalf("seed plan %q: %v", name, err)
	}
	t.Cleanup(func() {
		// subscriptions.plan_id is NOT NULL / FK'd with no ON DELETE cascade, and
		// this cleanup can run before seedTeamAppSub's (t.Cleanup is LIFO, and
		// seedPlan is normally called after seedTeamAppSub in test bodies) — so
		// detach any subscription still pointing at this plan first. Deleting the
		// subscription cascade-deletes its invoices (invoices.subscription_id ON
		// DELETE CASCADE).
		_, _ = pool.Exec(ctx, `DELETE FROM subscriptions WHERE plan_id=$1`, planID)
		_, _ = pool.Exec(ctx, `DELETE FROM plans WHERE id=$1`, planID)
	})
	return planID
}

// linkSubToPlan repoints a seeded subscription's plan_id at the plan under test.
func linkSubToPlan(t *testing.T, pool *pgxpool.Pool, subID, planID int64) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`UPDATE subscriptions SET plan_id=$1 WHERE id=$2`, planID, subID); err != nil {
		t.Fatalf("link sub to plan: %v", err)
	}
}

// onlyInvoiceForSub fetches the single invoice row for a subscription,
// failing the test if there isn't exactly one.
func onlyInvoiceForSub(t *testing.T, pool *pgxpool.Pool, subID int64) billing.Invoice {
	t.Helper()
	ctx := context.Background()
	rows, err := pool.Query(ctx,
		`SELECT id, billing_account_id, team_id, subscription_id, plan_name, price_cents, currency, status, created_at, paid_at
		 FROM invoices WHERE subscription_id=$1`, subID)
	if err != nil {
		t.Fatalf("query invoice: %v", err)
	}
	defer rows.Close()
	var out []billing.Invoice
	for rows.Next() {
		var v billing.Invoice
		if err := rows.Scan(&v.ID, &v.BillingAccountID, &v.TeamID, &v.SubscriptionID,
			&v.PlanName, &v.PriceCents, &v.Currency, &v.Status, &v.CreatedAt, &v.PaidAt); err != nil {
			t.Fatalf("scan invoice: %v", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows err: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected exactly 1 invoice for subscription %d, got %d", subID, len(out))
	}
	return out[0]
}

// countInvoicesForSub counts the invoice rows for a subscription.
func countInvoicesForSub(t *testing.T, pool *pgxpool.Pool, subID int64) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM invoices WHERE subscription_id=$1`, subID).Scan(&n); err != nil {
		t.Fatalf("count invoices: %v", err)
	}
	return n
}

func TestRepoEnsureAccountIsIdempotent(t *testing.T) {
	pool := dial(t)
	teamID, _, _ := seedTeamAppSub(t, pool)
	repo := billing.NewRepo(pool)
	ctx := context.Background()

	id1, err := repo.EnsureAccount(ctx, teamID)
	if err != nil {
		t.Fatalf("EnsureAccount: %v", err)
	}
	id2, err := repo.EnsureAccount(ctx, teamID)
	if err != nil {
		t.Fatalf("EnsureAccount second call: %v", err)
	}
	if id1 != id2 {
		t.Fatalf("EnsureAccount returned different ids: %d vs %d", id1, id2)
	}
}

func TestRepoGetNotFound(t *testing.T) {
	pool := dial(t)
	repo := billing.NewRepo(pool)
	if _, err := repo.Get(context.Background(), -1); err != billing.ErrNotFound {
		t.Fatalf("Get(missing) err = %v, want ErrNotFound", err)
	}
}

func TestRepoVoidThenMarkPaidIsInvalidTransition(t *testing.T) {
	pool := dial(t)
	teamID, appID, subID := seedTeamAppSub(t, pool)
	planID := seedPlan(t, pool, "VoidPlan", 1500, "EUR")
	linkSubToPlan(t, pool, subID, planID)
	repo := billing.NewRepo(pool)
	ctx := context.Background()

	accountID, err := repo.EnsureAccount(ctx, teamID)
	if err != nil {
		t.Fatalf("EnsureAccount: %v", err)
	}
	inv, err := repo.CreateInvoice(ctx, accountID, teamID, subID, "VoidPlan", 1500, "EUR")
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}
	_ = appID

	if err := repo.Void(ctx, inv.ID); err != nil {
		t.Fatalf("Void: %v", err)
	}
	if err := repo.MarkPaid(ctx, inv.ID); err != billing.ErrInvalidTransition {
		t.Fatalf("MarkPaid after Void err = %v, want ErrInvalidTransition", err)
	}
	if err := repo.Void(ctx, inv.ID); err != billing.ErrInvalidTransition {
		t.Fatalf("double-void err = %v, want ErrInvalidTransition", err)
	}
}

func TestRepoListByTeamsAndTeamsForUser(t *testing.T) {
	pool := dial(t)
	teamID, _, subID := seedTeamAppSub(t, pool)
	planID := seedPlan(t, pool, "ListPlan", 900, "EUR")
	linkSubToPlan(t, pool, subID, planID)
	repo := billing.NewRepo(pool)
	ctx := context.Background()

	accountID, err := repo.EnsureAccount(ctx, teamID)
	if err != nil {
		t.Fatalf("EnsureAccount: %v", err)
	}
	if _, err := repo.CreateInvoice(ctx, accountID, teamID, subID, "ListPlan", 900, "EUR"); err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}

	invs, err := repo.ListByTeams(ctx, []int64{teamID})
	if err != nil {
		t.Fatalf("ListByTeams: %v", err)
	}
	if len(invs) != 1 || invs[0].TeamID != teamID {
		t.Fatalf("ListByTeams = %+v", invs)
	}

	if empty, err := repo.ListByTeams(ctx, nil); err != nil || len(empty) != 0 {
		t.Fatalf("ListByTeams(nil) = %v, %v, want empty/nil", empty, err)
	}

	var uid int64
	if err := pool.QueryRow(ctx, `SELECT user_id FROM team_members WHERE team_id=$1 LIMIT 1`, teamID).Scan(&uid); err != nil {
		t.Fatalf("find team member: %v", err)
	}
	teamIDs, err := repo.TeamsForUser(ctx, uid)
	if err != nil {
		t.Fatalf("TeamsForUser: %v", err)
	}
	var found bool
	for _, id := range teamIDs {
		if id == teamID {
			found = true
		}
	}
	if !found {
		t.Fatalf("TeamsForUser(%d) = %v, want to include %d", uid, teamIDs, teamID)
	}
}
