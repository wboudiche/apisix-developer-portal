package applications

import (
	"context"
	"os"
	"testing"
	"time"

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
	list, err := repo.ListByOwner(ctx, uid)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListByOwner: %v len=%d", err, len(list))
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
