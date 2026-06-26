package subscriptions

import (
	"context"
	"os"
	"testing"
	"time"

	"apisix-portal/internal/config"
	"apisix-portal/internal/crypto"
	"apisix-portal/internal/db"
)

func testRepo(t *testing.T) (context.Context, *Repo, int64) {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://portal:portal@localhost:5432/portal?sslmode=disable"
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
	// seed a user and an application to own the credential (FK targets)
	suffix := time.Now().Format("150405.000000000")
	var uid, appID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users(email,password_hash,name) VALUES($1,'x','U') RETURNING id`,
		"credowner+"+suffix+"@example.com").Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO applications(owner_id,name) VALUES($1,'CredApp') RETURNING id`, uid).Scan(&appID); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	cipher, err := crypto.New(config.DevCredentialEncKey)
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	return ctx, NewRepo(pool, cipher), appID
}

func TestGetOrCreateCredentialIsIdempotent(t *testing.T) {
	ctx, repo, appID := testRepo(t)
	calls := 0
	gen := func() string { calls++; return GenerateKey() }

	first, err := repo.GetOrCreateCredential(ctx, appID, gen)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if len(first.APIKey) != 32 {
		t.Fatalf("api key should be 32 hex chars, got %q", first.APIKey)
	}
	if first.ConsumerUsername != consumerName(appID) {
		t.Fatalf("consumer username = %q want %q", first.ConsumerUsername, consumerName(appID))
	}

	second, err := repo.GetOrCreateCredential(ctx, appID, gen)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	// same key on repeat — one credential per application, even though gen was called again
	if second.APIKey != first.APIKey {
		t.Fatalf("expected the same key on repeat, got %q then %q", first.APIKey, second.APIKey)
	}
}

func TestGenerateKeyIs32Hex(t *testing.T) {
	k := GenerateKey()
	if len(k) != 32 {
		t.Fatalf("len=%d want 32", len(k))
	}
	for _, c := range k {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Fatalf("non-hex char %q in %q", c, k)
		}
	}
}

func TestApprovedAppsForProduct(t *testing.T) {
	ctx, repo, appID := testRepo(t)

	// Retrieve the owner of the seeded application (same-package pool access).
	var ownerID int64
	if err := repo.pool.QueryRow(ctx,
		`SELECT owner_id FROM applications WHERE id=$1`, appID).Scan(&ownerID); err != nil {
		t.Fatalf("get owner: %v", err)
	}

	// Use a unique suffix so re-runs on a persistent DB don't clash.
	suffix := time.Now().Format("150405.000000000")

	// Seed a product.
	var productID int64
	if err := repo.pool.QueryRow(ctx,
		`INSERT INTO api_products(name,slug,category,context_path,published)
		 VALUES($1,$2,'Test','/tryit-test',true) RETURNING id`,
		"TryitTestProduct+"+suffix, "tryit-test-product-"+suffix,
	).Scan(&productID); err != nil {
		t.Fatalf("seed product: %v", err)
	}

	// Seed a second application owned by the same user.
	var app2ID int64
	if err := repo.pool.QueryRow(ctx,
		`INSERT INTO applications(owner_id,name) VALUES($1,$2) RETURNING id`,
		ownerID, "TryitApp2+"+suffix,
	).Scan(&app2ID); err != nil {
		t.Fatalf("seed app2: %v", err)
	}

	// Seed a default plan required by subscriptions.plan_id FK.
	var planID int64
	if err := repo.pool.QueryRow(ctx,
		`INSERT INTO plans(name,rate_limit_count,rate_limit_window_s)
		 VALUES($1,100,60) RETURNING id`, "TryitPlan+"+suffix,
	).Scan(&planID); err != nil {
		t.Fatalf("seed plan: %v", err)
	}

	// Seed an ACTIVE subscription for appID and a PENDING one for app2ID.
	if _, err := repo.pool.Exec(ctx,
		`INSERT INTO subscriptions(application_id,api_product_id,plan_id,status) VALUES($1,$2,$3,'active')`,
		appID, productID, planID); err != nil {
		t.Fatalf("seed active sub: %v", err)
	}
	if _, err := repo.pool.Exec(ctx,
		`INSERT INTO subscriptions(application_id,api_product_id,plan_id,status) VALUES($1,$2,$3,'pending')`,
		app2ID, productID, planID); err != nil {
		t.Fatalf("seed pending sub: %v", err)
	}

	t.Cleanup(func() {
		_, _ = repo.pool.Exec(ctx,
			`DELETE FROM subscriptions WHERE api_product_id=$1`, productID)
		_, _ = repo.pool.Exec(ctx,
			`DELETE FROM applications WHERE id=$1`, app2ID)
		_, _ = repo.pool.Exec(ctx,
			`DELETE FROM plans WHERE id=$1`, planID)
		_, _ = repo.pool.Exec(ctx,
			`DELETE FROM api_products WHERE id=$1`, productID)
	})

	refs, err := repo.ApprovedAppsForProduct(ctx, ownerID, productID)
	if err != nil {
		t.Fatalf("ApprovedAppsForProduct: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1 approved app, got %d", len(refs))
	}
	if refs[0].ID != appID {
		t.Fatalf("expected appID=%d, got %d", appID, refs[0].ID)
	}
}
