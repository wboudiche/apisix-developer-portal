package db

import (
	"context"
	"os"
	"testing"
)

func TestAuditForcedMigration(t *testing.T) {
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

	// A row inserted without naming the column must default to FALSE — this
	// is the path every pre-migration audit row (and Reset's insert) takes.
	if _, err := pool.Exec(ctx,
		`INSERT INTO portal_settings_audit(key, old_value, new_value, admin_id) VALUES('TEST_FORCED_DEFAULT', NULL, 'v', NULL)`); err != nil {
		t.Fatalf("insert without forced: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM portal_settings_audit WHERE key='TEST_FORCED_DEFAULT'`) })
	var forced bool
	if err := pool.QueryRow(ctx,
		`SELECT forced FROM portal_settings_audit WHERE key='TEST_FORCED_DEFAULT'`).Scan(&forced); err != nil {
		t.Fatalf("forced column missing: %v", err)
	}
	if forced {
		t.Fatal("default must be FALSE")
	}

	// The column round-trips an explicit TRUE (a forced Set).
	if _, err := pool.Exec(ctx,
		`INSERT INTO portal_settings_audit(key, old_value, new_value, admin_id, forced) VALUES('TEST_FORCED_TRUE', NULL, 'v', NULL, TRUE)`); err != nil {
		t.Fatalf("insert with forced=true: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM portal_settings_audit WHERE key='TEST_FORCED_TRUE'`) })
	if err := pool.QueryRow(ctx,
		`SELECT forced FROM portal_settings_audit WHERE key='TEST_FORCED_TRUE'`).Scan(&forced); err != nil {
		t.Fatalf("select forced=true: %v", err)
	}
	if !forced {
		t.Fatal("forced=true must round-trip")
	}
}
