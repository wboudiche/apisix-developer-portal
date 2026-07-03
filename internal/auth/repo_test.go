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
	u, err := repo.Create(ctx, email, "hash", "Dev", "fr")
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
	if _, err := repo.Create(ctx, email, "h", "A", "fr"); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := repo.Create(ctx, email, "h", "B", "fr")
	if err == nil {
		t.Fatal("duplicate email should fail")
	}
	if !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("expected ErrEmailTaken, got %v", err)
	}
}

func TestCreateAlsoCreatesPersonalTeam(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://portal:portal@localhost:5432/portal?sslmode=disable"
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, url)
	if err != nil {
		t.Skipf("no database: %v", err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := NewRepo(pool)
	suf := time.Now().Format("150405.000000000")
	u, err := repo.Create(ctx, "reg+"+suf+"@e.com", "hash", "Reg User", "fr")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, u.ID) })
	var personalTeams int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM teams t JOIN team_members tm ON tm.team_id=t.id
		 WHERE tm.user_id=$1 AND tm.role='owner' AND t.personal`, u.ID).Scan(&personalTeams); err != nil {
		t.Fatalf("query: %v", err)
	}
	if personalTeams != 1 {
		t.Errorf("personal teams for new user = %d, want 1", personalTeams)
	}
}

func TestCreateSeedsAndSetLanguage(t *testing.T) {
	ctx, repo := testUserRepo(t)
	email := "lang+" + randSuffix() + "@x.io"

	u, err := repo.Create(ctx, email, "hash", "Ada", "en")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { _, _ = repo.pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, u.ID) })
	if u.Language != "en" {
		t.Fatalf("seeded language = %q, want en", u.Language)
	}

	if err := repo.SetLanguage(ctx, u.ID, "fr"); err != nil {
		t.Fatalf("setlang: %v", err)
	}
	got, _, err := repo.GetByEmail(ctx, email)
	if err != nil {
		t.Fatalf("getbyemail: %v", err)
	}
	if got.Language != "fr" {
		t.Fatalf("after SetLanguage = %q, want fr", got.Language)
	}

	if err := repo.SetLanguage(ctx, u.ID, "de"); err == nil {
		t.Fatal("SetLanguage('de') should violate the CHECK")
	}
}
