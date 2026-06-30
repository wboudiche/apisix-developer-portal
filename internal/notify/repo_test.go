package notify

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
	suf := time.Now().Format("150405.000000000")
	var uid, appID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,name,role) VALUES($1,'x','Dev','developer') RETURNING id`,
		"owner+"+suf+"@e.com").Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO applications(owner_id,name) VALUES($1,$2) RETURNING id`,
		uid, "App "+suf).Scan(&appID); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM applications WHERE id=$1`, appID)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, uid)
	})
	return ctx, NewRepo(pool), appID
}

func TestOwnerEmailAndAdmins(t *testing.T) {
	ctx, repo, appID := testRepo(t)
	email, name, err := repo.OwnerEmailForApp(ctx, appID)
	if err != nil || email == "" || name == "" {
		t.Fatalf("OwnerEmailForApp = %q,%q,%v", email, name, err)
	}
	admins, err := repo.AdminEmails(ctx)
	if err != nil {
		t.Fatalf("AdminEmails: %v", err)
	}
	for _, a := range admins {
		if a == "" {
			t.Fatal("empty admin email")
		}
	}
}
