package applications

import (
	"context"
	"os"
	"testing"
	"time"

	"apisix-portal/internal/db"
	"apisix-portal/internal/paging"
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
	var uid int64
	email := "appowner+" + time.Now().Format("150405.000000000") + "@example.com"
	if err := pool.QueryRow(ctx,
		`INSERT INTO users(email,password_hash,name) VALUES($1,'x','U') RETURNING id`, email).Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return ctx, NewRepo(pool), uid
}

func TestCreateAndListByOwner(t *testing.T) {
	ctx, repo, uid := testRepo(t)
	a, err := repo.Create(ctx, uid, "My App", "desc")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if a.ID == 0 || a.OwnerID != uid {
		t.Fatalf("bad app: %+v", a)
	}
	list, total, err := repo.ListByOwner(ctx, uid, paging.Params{Page: 1, Size: 20})
	if err != nil || len(list) != 1 || total != 1 {
		t.Fatalf("ListByOwner: %v len=%d total=%d", err, len(list), total)
	}
}

func TestListByOwnerCountsSubsAndKey(t *testing.T) {
	ctx, repo, uid := testRepo(t)
	a, err := repo.Create(ctx, uid, "Counts App", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// A fresh application has no subscriptions and no key.
	list, _, err := repo.ListByOwner(ctx, uid, paging.Params{Page: 1, Size: 20})
	if err != nil || len(list) != 1 {
		t.Fatalf("ListByOwner: %v len=%d", err, len(list))
	}
	if list[0].SubscriptionCount != 0 || list[0].HasKey {
		t.Fatalf("fresh app: got count=%d hasKey=%v, want 0/false", list[0].SubscriptionCount, list[0].HasKey)
	}

	// Issue a credential (→ HasKey) and two subscriptions to distinct products.
	suffix := time.Now().Format("150405.000000000")
	if _, err := repo.pool.Exec(ctx,
		`INSERT INTO credentials(application_id, api_key, consumer_username) VALUES($1,$2,$3)`,
		a.ID, "key-"+suffix, "consumer-"+suffix); err != nil {
		t.Fatalf("seed credential: %v", err)
	}
	prodIDs := make([]int64, 0, 2)
	rows, err := repo.pool.Query(ctx, `SELECT id FROM api_products ORDER BY id LIMIT 2`)
	if err != nil {
		t.Fatalf("query products: %v", err)
	}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan product: %v", err)
		}
		prodIDs = append(prodIDs, id)
	}
	rows.Close()
	if len(prodIDs) < 2 {
		t.Skipf("need >=2 seeded products, have %d", len(prodIDs))
	}
	var planID int64
	if err := repo.pool.QueryRow(ctx, `SELECT id FROM plans ORDER BY id LIMIT 1`).Scan(&planID); err != nil {
		t.Fatalf("query plan: %v", err)
	}
	for _, pid := range prodIDs {
		if _, err := repo.pool.Exec(ctx,
			`INSERT INTO subscriptions(application_id, api_product_id, plan_id, status) VALUES($1,$2,$3,'active')`,
			a.ID, pid, planID); err != nil {
			t.Fatalf("seed subscription: %v", err)
		}
	}

	list, _, err = repo.ListByOwner(ctx, uid, paging.Params{Page: 1, Size: 20})
	if err != nil || len(list) != 1 {
		t.Fatalf("ListByOwner #2: %v len=%d", err, len(list))
	}
	if list[0].SubscriptionCount != 2 {
		t.Errorf("SubscriptionCount = %d, want 2", list[0].SubscriptionCount)
	}
	if !list[0].HasKey {
		t.Errorf("HasKey = false, want true")
	}
}

func TestGetEnforcesOwnership(t *testing.T) {
	ctx, repo, uid := testRepo(t)
	a, _ := repo.Create(ctx, uid, "App", "")
	if _, err := repo.Get(ctx, a.ID, uid); err != nil {
		t.Fatalf("owner Get: %v", err)
	}
	if _, err := repo.Get(ctx, a.ID, uid+999); err != ErrNotFound {
		t.Fatalf("non-owner Get: want ErrNotFound, got %v", err)
	}
}
