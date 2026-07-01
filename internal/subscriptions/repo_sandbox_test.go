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

func sandboxTestRepo(t *testing.T) (context.Context, *Repo, int64, int64, int64) {
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
	cipher, err := crypto.New(config.DevCredentialEncKey)
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	repo := NewRepo(pool, cipher)
	suf := time.Now().Format("150405.000000000")
	var uid, appID, pid, planID int64
	pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,name) VALUES($1,'x','U') RETURNING id`, "sb+"+suf+"@e.com").Scan(&uid)
	var teamID int64
	pool.QueryRow(ctx, `INSERT INTO teams(name,personal) VALUES('t',true) RETURNING id`).Scan(&teamID)
	pool.QueryRow(ctx, `INSERT INTO applications(owner_id,name,team_id) VALUES($1,'App',$2) RETURNING id`, uid, teamID).Scan(&appID)
	pool.QueryRow(ctx, `INSERT INTO api_products(name,slug,category,context_path,sandbox_upstream_url,published) VALUES($1,$2,'C','/sb','echo:8080',true) RETURNING id`, "SbProd "+suf, "sbprod-"+suf).Scan(&pid)
	pool.QueryRow(ctx, `INSERT INTO plans(name,rate_limit_count,rate_limit_window_s) VALUES($1,5,60) RETURNING id`, "SbPlan "+suf).Scan(&planID)
	pool.Exec(ctx, `INSERT INTO subscriptions(application_id,api_product_id,plan_id,status) VALUES($1,$2,$3,'active')`, appID, pid, planID)
	if _, err := repo.GetOrCreateCredential(ctx, appID, GenerateKey); err != nil {
		t.Fatalf("seed credential: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM subscriptions WHERE application_id=$1`, appID)
		pool.Exec(ctx, `DELETE FROM credentials WHERE application_id=$1`, appID)
		pool.Exec(ctx, `DELETE FROM applications WHERE id=$1`, appID)
		pool.Exec(ctx, `DELETE FROM api_products WHERE id=$1`, pid)
		pool.Exec(ctx, `DELETE FROM plans WHERE id=$1`, planID)
		pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, uid)
	})
	return ctx, repo, appID, pid, planID
}

func TestSandboxKeyRoundTripAndWhitelist(t *testing.T) {
	ctx, repo, appID, pid, planID := sandboxTestRepo(t)

	// No sandbox key yet → not in the product's sandbox whitelist.
	if k, err := repo.GetSandboxKey(ctx, appID); err != nil || k != "" {
		t.Fatalf("GetSandboxKey before = %q, %v (want empty)", k, err)
	}
	names, err := repo.SandboxConsumersForProduct(ctx, pid)
	if err != nil || len(names) != 0 {
		t.Fatalf("whitelist before = %v, %v (want empty)", names, err)
	}

	if err := repo.UpdateSandboxKey(ctx, appID, "sbkey-123"); err != nil {
		t.Fatalf("UpdateSandboxKey: %v", err)
	}
	if k, err := repo.GetSandboxKey(ctx, appID); err != nil || k != "sbkey-123" {
		t.Fatalf("GetSandboxKey after = %q, %v", k, err)
	}
	names, err = repo.SandboxConsumersForProduct(ctx, pid)
	if err != nil || len(names) != 1 {
		t.Fatalf("whitelist after = %v, %v (want 1)", names, err)
	}
	prods, err := repo.SandboxProductsForApp(ctx, appID)
	if err != nil || len(prods) != 1 || prods[0].SandboxUpstream != "echo:8080" {
		t.Fatalf("SandboxProductsForApp = %+v, %v", prods, err)
	}
	creds, err := repo.SandboxConsumersForPlan(ctx, planID)
	if err != nil {
		t.Fatalf("SandboxConsumersForPlan: %v", err)
	}
	if len(creds) != 1 {
		t.Fatalf("SandboxConsumersForPlan: want 1 credential, got %d", len(creds))
	}
	if creds[0].APIKey != "sbkey-123" {
		t.Fatalf("SandboxConsumersForPlan: APIKey = %q, want %q", creds[0].APIKey, "sbkey-123")
	}
	wantUsername := consumerName(appID)
	if creds[0].ConsumerUsername != wantUsername {
		t.Fatalf("SandboxConsumersForPlan: ConsumerUsername = %q, want %q", creds[0].ConsumerUsername, wantUsername)
	}
}
