package events_test

import (
	"context"
	"os"
	"testing"
	"time"

	"apisix-portal/internal/db"
	"apisix-portal/internal/events"
)

// testRepo connects to the dev DB (skips when unavailable) and seeds an owner +
// application to satisfy the app_events FK, returning the app id.
func testRepo(t *testing.T) (context.Context, *events.Repo, int64) {
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
	suffix := time.Now().Format("150405.000000000")
	var uid, appID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users(email,password_hash,name) VALUES($1,'x','U') RETURNING id`,
		"evt+"+suffix+"@example.com").Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	var teamID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO teams(name,personal) VALUES('t',true) RETURNING id`).Scan(&teamID); err != nil {
		t.Fatalf("seed team: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO applications(owner_id,name,team_id) VALUES($1,'EvtApp',$2) RETURNING id`, uid, teamID).Scan(&appID); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	return ctx, events.NewRepo(pool), appID
}

func TestLogAndRecentNewestFirst(t *testing.T) {
	ctx, repo, appID := testRepo(t)

	if err := repo.Log(ctx, appID, events.KindAppCreated, nil, nil); err != nil {
		t.Fatalf("log app_created: %v", err)
	}
	if err := repo.Log(ctx, appID, events.KindSubscribed, nil, nil); err != nil {
		t.Fatalf("log subscribed: %v", err)
	}

	got, err := repo.Recent(ctx, appID, 10)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 events, got %d", len(got))
	}
	// Newest first: the second insert (subscribed) must lead.
	if got[0].Kind != events.KindSubscribed || got[1].Kind != events.KindAppCreated {
		t.Fatalf("wrong order: %s then %s", got[0].Kind, got[1].Kind)
	}
}

func TestRecentResolvesProductAndPlanNames(t *testing.T) {
	ctx, repo, appID := testRepo(t)

	// The Recent JOIN should surface current product/plan names, and tolerate
	// null references (resolving to "").
	if err := repo.Log(ctx, appID, events.KindUnsubscribed, nil, nil); err != nil {
		t.Fatalf("log: %v", err)
	}
	got, err := repo.Recent(ctx, appID, 10)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if len(got) != 1 || got[0].ProductName != "" || got[0].PlanName != "" {
		t.Fatalf("null refs must resolve to empty names, got %+v", got)
	}
}

func TestRecentLimitAndIsolation(t *testing.T) {
	ctx, repo, appID := testRepo(t)
	for i := 0; i < 5; i++ {
		if err := repo.Log(ctx, appID, events.KindSubscribed, nil, nil); err != nil {
			t.Fatalf("log %d: %v", i, err)
		}
	}
	got, err := repo.Recent(ctx, appID, 3)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("limit not honored: want 3, got %d", len(got))
	}
	// A different app id must see none of these.
	other, err := repo.Recent(ctx, appID+1_000_000, 10)
	if err != nil {
		t.Fatalf("recent other: %v", err)
	}
	if len(other) != 0 {
		t.Fatalf("events must be isolated per app; got %d for a foreign app", len(other))
	}
}
