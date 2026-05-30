package plans

import (
	"context"
	"os"
	"testing"

	"apisix-portal/internal/db"
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
	all, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("want 3 plans, got %d", len(all))
	}
}

func TestGetByIDFound(t *testing.T) {
	ctx, repo := testRepo(t)
	all, _ := repo.List(ctx)
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
