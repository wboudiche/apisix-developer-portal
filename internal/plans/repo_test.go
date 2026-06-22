package plans

import (
	"context"
	"os"
	"testing"

	"apisix-portal/internal/db"
	"apisix-portal/internal/paging"
)

func testRepo(t *testing.T) (context.Context, *Repo) {
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
	return ctx, NewRepo(pool)
}

func TestListReturnsThreeSeededPlans(t *testing.T) {
	ctx, repo := testRepo(t)
	all, _, err := repo.List(ctx, paging.Params{Page: 1, Size: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// Assert the three seeded plans are present rather than an exact total: the
	// E2E suite (internal/e2e) creates extra plans against the same shared DB,
	// so an exact-count check is brittle. Presence of the seeds is the invariant.
	names := make(map[string]bool, len(all))
	for _, p := range all {
		names[p.Name] = true
	}
	for _, want := range []string{"Free", "Silver", "Gold"} {
		if !names[want] {
			t.Fatalf("seeded plan %q missing; got %d plans", want, len(all))
		}
	}
}

func TestGetByIDFound(t *testing.T) {
	ctx, repo := testRepo(t)
	all, _, _ := repo.List(ctx, paging.Params{Page: 1, Size: 20})
	p, err := repo.GetByID(ctx, all[0].ID)
	if err != nil || p.ID != all[0].ID {
		t.Fatalf("GetByID: %v %+v", err, p)
	}
}

func TestGetByIDNotFound(t *testing.T) {
	ctx, repo := testRepo(t)
	if _, err := repo.GetByID(ctx, 999999); err != ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
