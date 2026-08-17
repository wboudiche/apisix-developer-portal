package subscriptions

import (
	"context"
	"errors"
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
	var teamID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO teams(name,personal) VALUES('t',true) RETURNING id`).Scan(&teamID); err != nil {
		t.Fatalf("seed team: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO team_members(team_id,user_id,role) VALUES($1,$2,'owner')`, teamID, uid); err != nil {
		t.Fatalf("seed team membership: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO applications(owner_id,name,team_id) VALUES($1,'CredApp',$2) RETURNING id`, uid, teamID).Scan(&appID); err != nil {
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

// TestDeleteApplicationDeletesRowAndCascades proves the DB-level cascade this
// repo method relies on: removing the application row also removes its
// credential and subscriptions (ON DELETE CASCADE), without needing explicit
// child deletes here.
func TestDeleteApplicationDeletesRowAndCascades(t *testing.T) {
	ctx, repo, appID := testRepo(t)
	suffix := time.Now().Format("150405.000000000")

	if _, err := repo.GetOrCreateCredential(ctx, appID, GenerateKey); err != nil {
		t.Fatalf("seed credential: %v", err)
	}

	var productID int64
	if err := repo.pool.QueryRow(ctx,
		`INSERT INTO api_products(name,slug,category,context_path,published)
		 VALUES($1,$2,'Test','/delapp-test',true) RETURNING id`,
		"DelAppTestProduct+"+suffix, "delapp-test-product-"+suffix,
	).Scan(&productID); err != nil {
		t.Fatalf("seed product: %v", err)
	}
	var planID int64
	if err := repo.pool.QueryRow(ctx,
		`INSERT INTO plans(name,rate_limit_count,rate_limit_window_s) VALUES($1,100,60) RETURNING id`,
		"DelAppPlan+"+suffix,
	).Scan(&planID); err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	t.Cleanup(func() {
		_, _ = repo.pool.Exec(ctx, `DELETE FROM plans WHERE id=$1`, planID)
		_, _ = repo.pool.Exec(ctx, `DELETE FROM api_products WHERE id=$1`, productID)
	})
	if _, err := repo.pool.Exec(ctx,
		`INSERT INTO subscriptions(application_id,api_product_id,plan_id,status) VALUES($1,$2,$3,'active')`,
		appID, productID, planID); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}

	if err := repo.DeleteApplication(ctx, appID); err != nil {
		t.Fatalf("DeleteApplication: %v", err)
	}

	var n int
	if err := repo.pool.QueryRow(ctx, `SELECT count(*) FROM applications WHERE id=$1`, appID).Scan(&n); err != nil {
		t.Fatalf("count applications: %v", err)
	} else if n != 0 {
		t.Errorf("application row still present after delete")
	}
	if err := repo.pool.QueryRow(ctx, `SELECT count(*) FROM credentials WHERE application_id=$1`, appID).Scan(&n); err != nil {
		t.Fatalf("count credentials: %v", err)
	} else if n != 0 {
		t.Errorf("credential row still present after delete (should cascade)")
	}
	if err := repo.pool.QueryRow(ctx, `SELECT count(*) FROM subscriptions WHERE application_id=$1`, appID).Scan(&n); err != nil {
		t.Fatalf("count subscriptions: %v", err)
	} else if n != 0 {
		t.Errorf("subscription row still present after delete (should cascade)")
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

	// Seed a team to satisfy applications.team_id, then a second application
	// owned by the same user.
	var team2ID int64
	if err := repo.pool.QueryRow(ctx,
		`INSERT INTO teams(name,personal) VALUES('t',true) RETURNING id`,
	).Scan(&team2ID); err != nil {
		t.Fatalf("seed team2: %v", err)
	}
	var app2ID int64
	if err := repo.pool.QueryRow(ctx,
		`INSERT INTO applications(owner_id,name,team_id) VALUES($1,$2,$3) RETURNING id`,
		ownerID, "TryitApp2+"+suffix, team2ID,
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

// TestApprovedAppsForProductTeamMember proves that a non-owner team member
// (not the application's owner_id) sees the team's approved app in their
// try-it context dropdown, since applications are team-owned.
func TestApprovedAppsForProductTeamMember(t *testing.T) {
	ctx, repo, _ := testRepo(t)
	suffix := time.Now().Format("150405.000000000")

	// Seed a product.
	var productID int64
	if err := repo.pool.QueryRow(ctx,
		`INSERT INTO api_products(name,slug,category,context_path,published)
		 VALUES($1,$2,'Test','/tryit-member-test',true) RETURNING id`,
		"TryitMemberProduct+"+suffix, "tryit-member-product-"+suffix,
	).Scan(&productID); err != nil {
		t.Fatalf("seed product: %v", err)
	}

	// Seed a team owner and a second, non-owner team member.
	var ownerID, memberID int64
	if err := repo.pool.QueryRow(ctx,
		`INSERT INTO users(email,password_hash,name) VALUES($1,'x','Owner') RETURNING id`,
		"teamowner+"+suffix+"@example.com").Scan(&ownerID); err != nil {
		t.Fatalf("seed owner: %v", err)
	}
	if err := repo.pool.QueryRow(ctx,
		`INSERT INTO users(email,password_hash,name) VALUES($1,'x','Member') RETURNING id`,
		"teammember+"+suffix+"@example.com").Scan(&memberID); err != nil {
		t.Fatalf("seed member: %v", err)
	}

	// Seed the team (must exist before applications.team_id, which is NOT NULL)
	// and add both users as members: the owner and a plain 'member'.
	var teamID int64
	if err := repo.pool.QueryRow(ctx,
		`INSERT INTO teams(name,personal) VALUES('Acme',false) RETURNING id`,
	).Scan(&teamID); err != nil {
		t.Fatalf("seed team: %v", err)
	}
	if _, err := repo.pool.Exec(ctx,
		`INSERT INTO team_members(team_id,user_id,role) VALUES($1,$2,'owner')`, teamID, ownerID); err != nil {
		t.Fatalf("seed owner membership: %v", err)
	}
	if _, err := repo.pool.Exec(ctx,
		`INSERT INTO team_members(team_id,user_id,role) VALUES($1,$2,'member')`, teamID, memberID); err != nil {
		t.Fatalf("seed member membership: %v", err)
	}

	// The app is owned (owner_id) by the team owner, but belongs to the team.
	var appID int64
	if err := repo.pool.QueryRow(ctx,
		`INSERT INTO applications(owner_id,name,team_id) VALUES($1,$2,$3) RETURNING id`,
		ownerID, "TeamApp+"+suffix, teamID,
	).Scan(&appID); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	var planID int64
	if err := repo.pool.QueryRow(ctx,
		`INSERT INTO plans(name,rate_limit_count,rate_limit_window_s)
		 VALUES($1,100,60) RETURNING id`, "TryitMemberPlan+"+suffix,
	).Scan(&planID); err != nil {
		t.Fatalf("seed plan: %v", err)
	}

	if _, err := repo.pool.Exec(ctx,
		`INSERT INTO subscriptions(application_id,api_product_id,plan_id,status) VALUES($1,$2,$3,'active')`,
		appID, productID, planID); err != nil {
		t.Fatalf("seed active sub: %v", err)
	}

	t.Cleanup(func() {
		_, _ = repo.pool.Exec(ctx, `DELETE FROM subscriptions WHERE api_product_id=$1`, productID)
		_, _ = repo.pool.Exec(ctx, `DELETE FROM applications WHERE id=$1`, appID)
		_, _ = repo.pool.Exec(ctx, `DELETE FROM plans WHERE id=$1`, planID)
		_, _ = repo.pool.Exec(ctx, `DELETE FROM api_products WHERE id=$1`, productID)
		_, _ = repo.pool.Exec(ctx, `DELETE FROM team_members WHERE team_id=$1`, teamID)
		_, _ = repo.pool.Exec(ctx, `DELETE FROM teams WHERE id=$1`, teamID)
		_, _ = repo.pool.Exec(ctx, `DELETE FROM users WHERE id IN ($1,$2)`, ownerID, memberID)
	})

	// The non-owner member must see the team's approved app.
	refs, err := repo.ApprovedAppsForProduct(ctx, memberID, productID)
	if err != nil {
		t.Fatalf("ApprovedAppsForProduct: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1 approved app for team member, got %d", len(refs))
	}
	if refs[0].ID != appID {
		t.Fatalf("expected appID=%d, got %d", appID, refs[0].ID)
	}
}
