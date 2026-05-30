package auth

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"apisix-portal/internal/db"
)

func testUserRepo(t *testing.T) (context.Context, *Repo) {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://portal:portal@localhost:5432/portal?sslmode=disable"
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, url)
	if err != nil {
		t.Skipf("no database available: %v", err)
	}
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(pool.Close)
	return ctx, NewRepo(pool)
}

func randSuffix() string { return time.Now().Format("150405.000000000") }

func TestCreateAndGetUser(t *testing.T) {
	ctx, repo := testUserRepo(t)
	email := "dev+" + randSuffix() + "@example.com"
	u, err := repo.Create(ctx, email, "hash", "Dev")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if u.ID == 0 || u.Role != "developer" {
		t.Fatalf("unexpected user: %+v", u)
	}
	got, hash, err := repo.GetByEmail(ctx, email)
	if err != nil {
		t.Fatalf("GetByEmail: %v", err)
	}
	if got.ID != u.ID || hash != "hash" {
		t.Fatalf("mismatch: %+v hash=%q", got, hash)
	}
}

func TestCreateDuplicateEmailFails(t *testing.T) {
	ctx, repo := testUserRepo(t)
	email := "dup+" + randSuffix() + "@example.com"
	if _, err := repo.Create(ctx, email, "h", "A"); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := repo.Create(ctx, email, "h", "B")
	if err == nil {
		t.Fatal("duplicate email should fail")
	}
	if !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("expected ErrEmailTaken, got %v", err)
	}
}
