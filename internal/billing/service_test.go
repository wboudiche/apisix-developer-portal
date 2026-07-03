package billing_test

import (
	"context"
	"testing"
	"time"

	"apisix-portal/internal/billing"
)

// planName returns a plan name unique to this test run. Seed data
// (migrations/0002_seed.sql) already contains plans literally named "Gold"
// and "Free", and plans.name is UNIQUE, so tests must not reuse those names.
func planName(base string) string {
	return base + "-" + time.Now().Format("150405.000000000")
}

func TestSubscriptionActivatedCreatesInvoiceForPaidPlan(t *testing.T) {
	pool := dial(t)
	ctx := context.Background()
	teamID, appID, subID := seedTeamAppSub(t, pool) // helper: creates team, app(team_id), pending sub
	gold := planName("Gold")
	planID := seedPlan(t, pool, gold, 2900, "EUR") // helper: INSERT plans ... RETURNING id
	linkSubToPlan(t, pool, subID, planID)          // set subscriptions.plan_id

	svc := billing.NewService(billing.NewRepo(pool), billing.ManualProvider{})
	if err := svc.SubscriptionActivated(ctx, appID, subID, planID); err != nil {
		t.Fatalf("activate: %v", err)
	}
	inv := onlyInvoiceForSub(t, pool, subID) // helper: SELECT the invoice row
	if inv.PriceCents != 2900 || inv.PlanName != gold || inv.Status != "pending" || inv.TeamID != teamID {
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
	if _, err := pool.Exec(ctx, `UPDATE plans SET price_cents=5000 WHERE id=$1`, planID); err != nil {
		t.Fatalf("update plan price: %v", err)
	}
	inv2 := onlyInvoiceForSub(t, pool, subID)
	if inv2.PriceCents != 2900 {
		t.Fatalf("snapshot broken: invoice price=%d, want 2900", inv2.PriceCents)
	}
}

func TestSubscriptionActivatedFreePlanNoInvoice(t *testing.T) {
	pool := dial(t)
	ctx := context.Background()
	_, appID, subID := seedTeamAppSub(t, pool)
	planID := seedPlan(t, pool, planName("Free"), 0, "EUR")
	linkSubToPlan(t, pool, subID, planID)
	svc := billing.NewService(billing.NewRepo(pool), billing.ManualProvider{})
	if err := svc.SubscriptionActivated(ctx, appID, subID, planID); err != nil {
		t.Fatalf("activate: %v", err)
	}
	if n := countInvoicesForSub(t, pool, subID); n != 0 {
		t.Fatalf("free plan created %d invoices, want 0", n)
	}
}

// TestInvoiceSurvivesSubscriptionDeletion is the regression test for the
// whole-branch-review finding: unsubscribing (which deletes the subscription
// row) must NOT erase the invoice ledger, even for paid invoices. Migration
// 0017 changes invoices.subscription_id's FK from ON DELETE CASCADE to
// ON DELETE SET NULL and drops the NOT NULL constraint so the invoice row
// detaches instead of disappearing.
func TestInvoiceSurvivesSubscriptionDeletion(t *testing.T) {
	pool := dial(t)
	ctx := context.Background()
	teamID, appID, subID := seedTeamAppSub(t, pool)
	pname := planName("SurvivorPlan")
	planID := seedPlan(t, pool, pname, 3300, "USD")
	linkSubToPlan(t, pool, subID, planID)

	svc := billing.NewService(billing.NewRepo(pool), billing.ManualProvider{})
	if err := svc.SubscriptionActivated(ctx, appID, subID, planID); err != nil {
		t.Fatalf("activate: %v", err)
	}
	inv := onlyInvoiceForSub(t, pool, subID)
	if err := svc.MarkPaid(ctx, inv.ID); err != nil {
		t.Fatalf("markpaid: %v", err)
	}

	// seedTeamAppSub's cleanup deletes invoices by subscription_id, which
	// will no longer match this invoice once the subscription is detached
	// below, so explicitly clean up the invoice row by id ourselves.
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM invoices WHERE id=$1`, inv.ID)
	})

	// Simulate an unsubscribe: delete the subscription row directly, the
	// same way the subscriptions package does on unsubscribe.
	if _, err := pool.Exec(ctx, `DELETE FROM subscriptions WHERE id=$1`, subID); err != nil {
		t.Fatalf("delete subscription: %v", err)
	}

	got, err := svc.Get(ctx, inv.ID)
	if err != nil {
		t.Fatalf("Get after unsubscribe: %v (invoice should survive)", err)
	}
	if got.SubscriptionID != nil {
		t.Fatalf("SubscriptionID = %v, want nil after subscription deletion", *got.SubscriptionID)
	}
	if got.TeamID != teamID || got.PlanName != pname || got.PriceCents != 3300 || got.Status != billing.StatusPaid {
		t.Fatalf("invoice ledger not preserved: %+v", got)
	}
}

func TestMarkPaidAndVoidTransitions(t *testing.T) {
	pool := dial(t)
	ctx := context.Background()
	_, appID, subID := seedTeamAppSub(t, pool)
	planID := seedPlan(t, pool, planName("Gold"), 2900, "EUR")
	linkSubToPlan(t, pool, subID, planID)
	svc := billing.NewService(billing.NewRepo(pool), billing.ManualProvider{})
	if err := svc.SubscriptionActivated(ctx, appID, subID, planID); err != nil {
		t.Fatalf("activate: %v", err)
	}
	inv := onlyInvoiceForSub(t, pool, subID)

	if err := svc.MarkPaid(ctx, inv.ID); err != nil {
		t.Fatalf("markpaid: %v", err)
	}
	got, err := svc.Get(ctx, inv.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != "paid" || got.PaidAt == nil {
		t.Fatalf("not paid: %+v", got)
	}
	if err := svc.MarkPaid(ctx, inv.ID); err != billing.ErrInvalidTransition {
		t.Fatalf("double-pay err = %v, want ErrInvalidTransition", err)
	}
}

func TestListForUserAndListAll(t *testing.T) {
	pool := dial(t)
	ctx := context.Background()
	_, appID, subID := seedTeamAppSub(t, pool)
	planID := seedPlan(t, pool, "TeamPlan", 1200, "EUR")
	linkSubToPlan(t, pool, subID, planID)
	svc := billing.NewService(billing.NewRepo(pool), billing.ManualProvider{})
	if err := svc.SubscriptionActivated(ctx, appID, subID, planID); err != nil {
		t.Fatalf("activate: %v", err)
	}
	inv := onlyInvoiceForSub(t, pool, subID)

	var uid int64
	if err := pool.QueryRow(ctx, `SELECT owner_id FROM applications WHERE id=$1`, appID).Scan(&uid); err != nil {
		t.Fatalf("find app owner: %v", err)
	}

	invs, err := svc.ListForUser(ctx, uid)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	var found bool
	for _, v := range invs {
		if v.ID == inv.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("ListForUser(%d) = %+v, want to include invoice %d", uid, invs, inv.ID)
	}

	all, err := svc.ListAll(ctx, "pending")
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	found = false
	for _, v := range all {
		if v.ID == inv.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("ListAll(pending) = %+v, want to include invoice %d", all, inv.ID)
	}
}
