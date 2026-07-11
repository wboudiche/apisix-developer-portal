package auth

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"apisix-portal/internal/db"

	"github.com/jackc/pgx/v5/pgxpool"
)

func testUserRepo(t *testing.T) (context.Context, *Repo) {
	t.Helper()
	pool := testPool(t)
	return context.Background(), NewRepo(pool)
}

// testPool connects to the test Postgres instance (skipping the test if none
// is reachable) and makes sure the schema is up to date.
func testPool(t *testing.T) *pgxpool.Pool {
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
	return pool
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

func TestEmailVerificationRepoFlow(t *testing.T) {
	pool := testPool(t) // use/extract the file's existing pool setup; skip if none
	repo := NewRepo(pool)
	ctx := context.Background()
	email := "verify+" + time.Now().Format("150405.000000000") + "@e2e.test"

	plain, hash := GenerateVerifyToken()
	u, err := repo.CreateUnverified(ctx, email, "x", "V User", "fr", hash, time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("CreateUnverified: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, u.ID) })

	got, _, err := repo.GetByEmail(ctx, email)
	if err != nil || got.Verified {
		t.Fatalf("fresh user must be unverified (err=%v verified=%v)", err, got.Verified)
	}

	if err := repo.VerifyByTokenHash(ctx, HashVerifyToken("wrong-token")); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("wrong token: got %v, want ErrTokenInvalid", err)
	}
	if err := repo.VerifyByTokenHash(ctx, HashVerifyToken(plain)); err != nil {
		t.Fatalf("VerifyByTokenHash: %v", err)
	}
	got, _, _ = repo.GetByEmail(ctx, email)
	if !got.Verified {
		t.Fatal("user must be verified after VerifyByTokenHash")
	}
	// Single-use: same token again fails (hash was cleared).
	if err := repo.VerifyByTokenHash(ctx, HashVerifyToken(plain)); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("reused token: got %v, want ErrTokenInvalid", err)
	}
	// Resend on a verified account refuses.
	if _, err := repo.ResetVerifyToken(ctx, email, "newhash", time.Now().Add(time.Hour)); !errors.Is(err, ErrAlreadyVerified) {
		t.Fatalf("reset on verified: got %v, want ErrAlreadyVerified", err)
	}
	// Resend on an unknown account reports not-found (handler still answers 204).
	if _, err := repo.ResetVerifyToken(ctx, "nobody@nowhere.test", "h", time.Now()); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("reset unknown: got %v, want ErrUserNotFound", err)
	}
}

func TestExpiredTokenIsInvalid(t *testing.T) {
	pool := testPool(t)
	repo := NewRepo(pool)
	ctx := context.Background()
	email := "expired+" + time.Now().Format("150405.000000000") + "@e2e.test"
	plain, hash := GenerateVerifyToken()
	u, err := repo.CreateUnverified(ctx, email, "x", "", "en", hash, time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatalf("CreateUnverified: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, u.ID) })
	if err := repo.VerifyByTokenHash(ctx, HashVerifyToken(plain)); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("expired token: got %v, want ErrTokenInvalid", err)
	}
	// A reset (resend) revives the flow with a fresh token.
	plain2, hash2 := GenerateVerifyToken()
	_ = plain2
	if _, err := repo.ResetVerifyToken(ctx, email, hash2, time.Now().Add(24*time.Hour)); err != nil {
		t.Fatalf("ResetVerifyToken: %v", err)
	}
	if err := repo.VerifyByTokenHash(ctx, hash2); err != nil {
		t.Fatalf("verify after reset: %v", err)
	}
}
