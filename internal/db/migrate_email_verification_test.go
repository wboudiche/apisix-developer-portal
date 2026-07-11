package db

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestEmailVerificationMigration(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://portal:portal@localhost:5432/portal?sslmode=disable"
	}
	ctx := context.Background()
	pool, err := Connect(ctx, url)
	if err != nil {
		t.Skipf("no database: %v", err)
	}
	defer pool.Close()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// A plain insert (no email_verified column) must default to TRUE — this is
	// the same path every pre-migration row takes, i.e. grandfathering.
	suf := time.Now().Format("150405.000000000")
	var uid int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users(email,password_hash,name,role) VALUES($1,'x','U','developer') RETURNING id`,
		"verif+"+suf+"@e.com").Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, uid) })
	var verified bool
	if err := pool.QueryRow(ctx,
		`SELECT email_verified FROM users WHERE id=$1`, uid).Scan(&verified); err != nil {
		t.Fatalf("email_verified column missing: %v", err)
	}
	if !verified {
		t.Fatal("default must be TRUE (grandfathering)")
	}
	// Token columns exist and are nullable.
	if _, err := pool.Exec(ctx,
		`UPDATE users SET verify_token_hash='h', verify_token_expires_at=now() WHERE id=$1`, uid); err != nil {
		t.Fatalf("token columns: %v", err)
	}
}
