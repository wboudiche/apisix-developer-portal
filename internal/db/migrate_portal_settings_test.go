package db

import (
	"context"
	"os"
	"testing"
)

func TestPortalSettingsMigration(t *testing.T) {
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
	// Override row round-trip; PK upsert works.
	if _, err := pool.Exec(ctx,
		`INSERT INTO portal_settings(key, value) VALUES('TEST_KEY','v1')
		 ON CONFLICT (key) DO UPDATE SET value='v2', updated_at=now()`); err != nil {
		t.Fatalf("portal_settings upsert: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM portal_settings WHERE key='TEST_KEY'`) })
	var v string
	if err := pool.QueryRow(ctx, `SELECT value FROM portal_settings WHERE key='TEST_KEY'`).Scan(&v); err != nil || v != "v1" {
		t.Fatalf("value = %q err=%v, want v1 (first insert wins the test's single statement)", v, err)
	}
	// Audit table accepts a row with a NULL admin (deleted user).
	if _, err := pool.Exec(ctx,
		`INSERT INTO portal_settings_audit(key, old_value, new_value, admin_id) VALUES('TEST_KEY', NULL, '(secret)', NULL)`); err != nil {
		t.Fatalf("portal_settings_audit insert: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM portal_settings_audit WHERE key='TEST_KEY'`) })
}
