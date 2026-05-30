package subscriptions

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
	// seed a user and an application to own the credential (FK targets)
	suffix := time.Now().Format("150405.000000000")
	var uid, appID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users(email,password_hash,name) VALUES($1,'x','U') RETURNING id`,
		"credowner+"+suffix+"@example.com").Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO applications(owner_id,name) VALUES($1,'CredApp') RETURNING id`, uid).Scan(&appID); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	return ctx, NewRepo(pool), appID
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
