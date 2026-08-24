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

func oauthTestRepo(t *testing.T) (context.Context, *Repo, int64, int64) {
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
	pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,name) VALUES($1,'x','U') RETURNING id`, "oauth+"+suf+"@e.com").Scan(&uid)
	var teamID int64
	pool.QueryRow(ctx, `INSERT INTO teams(name,personal) VALUES('t',true) RETURNING id`).Scan(&teamID)
	pool.QueryRow(ctx, `INSERT INTO applications(owner_id,name,team_id) VALUES($1,'App',$2) RETURNING id`, uid, teamID).Scan(&appID)
	pool.QueryRow(ctx, `INSERT INTO api_products(name,slug,category,context_path,upstream_url,auth_type,published) VALUES($1,$2,'C','/oauth','echo:8080','oauth2',true) RETURNING id`, "OAuthProd "+suf, "oauthprod-"+suf).Scan(&pid)
	pool.QueryRow(ctx, `INSERT INTO plans(name,rate_limit_count,rate_limit_window_s) VALUES($1,5,60) RETURNING id`, "OAuthPlan "+suf).Scan(&planID)
	pool.Exec(ctx, `INSERT INTO subscriptions(application_id,api_product_id,plan_id,status) VALUES($1,$2,$3,'active')`, appID, pid, planID)
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM subscriptions WHERE application_id=$1`, appID)
		pool.Exec(ctx, `DELETE FROM credentials WHERE application_id=$1`, appID)
		pool.Exec(ctx, `DELETE FROM applications WHERE id=$1`, appID)
		pool.Exec(ctx, `DELETE FROM api_products WHERE id=$1`, pid)
		pool.Exec(ctx, `DELETE FROM plans WHERE id=$1`, planID)
		pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, uid)
	})
	return ctx, repo, appID, pid
}

func TestOAuthClientWhitelistAndProductsForApp(t *testing.T) {
	ctx, repo, appID, pid := oauthTestRepo(t)
	// no client id yet → not in whitelist
	if ids, err := repo.OAuthClientsForProduct(ctx, pid); err != nil || len(ids) != 0 {
		t.Fatalf("whitelist before = %v, %v", ids, err)
	}
	if err := repo.SetAppOIDCClientID(ctx, appID, "client-xyz"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if got, err := repo.GetAppOIDCClientID(ctx, appID); err != nil || got != "client-xyz" {
		t.Fatalf("get = %q, %v", got, err)
	}
	ids, err := repo.OAuthClientsForProduct(ctx, pid)
	if err != nil || len(ids) != 1 || ids[0] != "client-xyz" {
		t.Fatalf("whitelist after = %v, %v", ids, err)
	}
	prods, err := repo.OAuthProductsForApp(ctx, appID)
	if err != nil || len(prods) != 1 || prods[0].AuthType != "oauth2" {
		t.Fatalf("OAuthProductsForApp = %+v, %v", prods, err)
	}
}

// TestSubscriptionsForAppReportsAuthType is a regression test for #9: the
// Overview Quickstart card needs each subscription's auth type to render the
// right example (apikey header vs. OAuth2 bearer) per subscription, not just
// the most recent one.
func TestSubscriptionsForAppReportsAuthType(t *testing.T) {
	ctx, repo, appID, _ := oauthTestRepo(t)
	subs, err := repo.SubscriptionsForApp(ctx, appID)
	if err != nil {
		t.Fatalf("SubscriptionsForApp: %v", err)
	}
	if len(subs) != 1 || subs[0].AuthType != "oauth2" {
		t.Fatalf("SubscriptionsForApp = %+v, want one oauth2 subscription", subs)
	}
}
