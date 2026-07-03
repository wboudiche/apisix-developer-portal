package admin

import (
	"testing"
	"time"
)

// TestPlanRepoPriceRoundTrip guards the price_cents/currency columns added by
// migration 0016: a plan created via PlanRepo.Create must read back the same
// price and currency it was given (see internal/db/migrations/0016_billing.sql).
func TestPlanRepoPriceRoundTrip(t *testing.T) {
	ctx, productRepo := adminTestRepo(t)
	repo := NewPlanRepo(productRepo.pool)
	name := "PriceRoundTrip-" + time.Now().Format("150405.000000000")
	created, err := repo.CreatePlan(ctx, Plan{
		Name: name, RateLimit: 100, WindowSeconds: 60,
		PriceCents: 2900, Currency: "EUR",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { _, _ = repo.pool.Exec(ctx, `DELETE FROM plans WHERE id=$1`, created.ID) })

	if created.PriceCents != 2900 || created.Currency != "EUR" {
		t.Fatalf("create round-trip: price=%d currency=%q, want 2900/EUR", created.PriceCents, created.Currency)
	}

	got, err := repo.GetPlan(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.PriceCents != 2900 || got.Currency != "EUR" {
		t.Fatalf("get round-trip: price=%d currency=%q, want 2900/EUR", got.PriceCents, got.Currency)
	}
}
