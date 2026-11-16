package db

import (
	"context"
	"os"
	"testing"
)

func TestVersioningSchema(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		// Never fall back to the dev database: tests insert users, teams
		// and apps they don't clean up. CI sets DATABASE_URL.
		t.Skip("DATABASE_URL not set; skipping DB-backed test")
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
	// lifecycle_status default + CHECK
	var def string
	if err := pool.QueryRow(ctx,
		`SELECT column_default FROM information_schema.columns WHERE table_name='api_products' AND column_name='lifecycle_status'`).Scan(&def); err != nil {
		t.Fatalf("lifecycle_status column: %v", err)
	}
	// CHECK rejects a bad status
	if _, err := pool.Exec(ctx, `UPDATE api_products SET lifecycle_status='bogus' WHERE id=(SELECT id FROM api_products LIMIT 1)`); err == nil {
		t.Error("expected CHECK to reject bogus lifecycle_status")
	}
	// changelog_entries exists + kind CHECK
	if _, err := pool.Exec(ctx,
		`INSERT INTO changelog_entries(product_id, version, kind, notes, entry_date)
		 SELECT id, 'v1', 'boguskind', '', '2026-01-01' FROM api_products LIMIT 1`); err == nil {
		t.Error("expected CHECK to reject bogus kind")
	}
}
