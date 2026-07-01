package applications

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"apisix-portal/internal/db"
	"apisix-portal/internal/paging"
)

// appTestSetup connects to the test database, migrates it, seeds a user, and
// gives that user a personal team (a raw-SQL user insert does not create one
// on its own — that only happens via the auth registration flow). It returns
// the context, repo, the seeded user id, and their personal team id.
func appTestSetup(t *testing.T) (context.Context, *Repo, *pgxpool.Pool, int64, int64) {
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

	var teamID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO teams(name, personal) VALUES($1, true) RETURNING id`, email).Scan(&teamID); err != nil {
		t.Fatalf("seed personal team: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO team_members(team_id, user_id, role) VALUES($1,$2,'owner')`, teamID, uid); err != nil {
		t.Fatalf("seed team membership: %v", err)
	}

	return ctx, NewRepo(pool), pool, uid, teamID
}

func TestCreateUnderTeamAndListForUser(t *testing.T) {
	ctx, repo, pool, uid, teamID := appTestSetup(t)
	_ = pool
	app, err := repo.Create(ctx, uid, teamID, "TeamApp", "d")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if app.TeamID != teamID {
		t.Fatalf("app.TeamID=%d want %d", app.TeamID, teamID)
	}
	apps, total, err := repo.ListForUser(ctx, uid, paging.Params{Page: 1, Size: 20})
	if err != nil || total < 1 {
		t.Fatalf("list: total=%d err=%v", total, err)
	}
	var found bool
	for _, a := range apps {
		if a.ID == app.ID {
			found = true
			if a.TeamName == "" {
				t.Error("TeamName not populated in list")
			}
		}
	}
	if !found {
		t.Error("created app not in ListForUser")
	}
	got, err := repo.Get(ctx, app.ID)
	if err != nil || got.TeamID != teamID {
		t.Fatalf("get: %+v err=%v", got, err)
	}
}

func TestListForUserCountsSubscriptions(t *testing.T) {
	ctx, repo, pool, uid, teamID := appTestSetup(t)
	a, err := repo.Create(ctx, uid, teamID, "Counts App", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// A fresh application has no subscriptions.
	list, _, err := repo.ListForUser(ctx, uid, paging.Params{Page: 1, Size: 20})
	if err != nil || len(list) != 1 {
		t.Fatalf("ListForUser: %v len=%d", err, len(list))
	}
	if list[0].SubscriptionCount != 0 {
		t.Fatalf("fresh app: got count=%d, want 0", list[0].SubscriptionCount)
	}

	// Subscribe to two distinct products and assert the count reflects it.
	prodIDs := make([]int64, 0, 2)
	rows, err := pool.Query(ctx, `SELECT id FROM api_products ORDER BY id LIMIT 2`)
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
	if err := pool.QueryRow(ctx, `SELECT id FROM plans ORDER BY id LIMIT 1`).Scan(&planID); err != nil {
		t.Fatalf("query plan: %v", err)
	}
	for _, pid := range prodIDs {
		if _, err := pool.Exec(ctx,
			`INSERT INTO subscriptions(application_id, api_product_id, plan_id, status) VALUES($1,$2,$3,'active')`,
			a.ID, pid, planID); err != nil {
			t.Fatalf("seed subscription: %v", err)
		}
	}

	list, _, err = repo.ListForUser(ctx, uid, paging.Params{Page: 1, Size: 20})
	if err != nil || len(list) != 1 {
		t.Fatalf("ListForUser #2: %v len=%d", err, len(list))
	}
	if list[0].SubscriptionCount != 2 {
		t.Errorf("SubscriptionCount = %d, want 2", list[0].SubscriptionCount)
	}
}

func TestGetByID(t *testing.T) {
	ctx, repo, _, uid, teamID := appTestSetup(t)
	a, err := repo.Create(ctx, uid, teamID, "App", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repo.Get(ctx, a.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.TeamID != teamID || got.TeamName == "" {
		t.Fatalf("Get: %+v", got)
	}
	if _, err := repo.Get(ctx, a.ID+999_999_999); err != ErrNotFound {
		t.Fatalf("missing Get: want ErrNotFound, got %v", err)
	}
}
