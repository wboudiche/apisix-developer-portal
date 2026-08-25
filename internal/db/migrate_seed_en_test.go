package db

import (
	"context"
	"os"
	"slices"
	"testing"
)

// TestSeedEnMigration is a regression test for #4: the demo seed data
// (0002_seed.sql) was authored in French with no localization mechanism
// behind it, so descriptions/tags stayed French regardless of the selected
// UI language. 0023_seed_en.sql re-seeds it in English.
func TestSeedEnMigration(t *testing.T) {
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

	cases := []struct {
		slug        string
		description string
		tags        []string
	}{
		{"seoapi", "On-page audits, rank tracking, and on-demand backlink analysis.", []string{"seo", "marketing", "real-time"}},
		{"reviewsapi", "Collect and aggregate customer reviews from multiple sources.", []string{"reviews", "marketing"}},
		{"stockanalysisapi", "Stock quotes, technical indicators, and real-time signals.", []string{"finance", "real-time"}},
		{"testapi", "Sandbox for validating your integrations before going to production.", []string{"sandbox", "internal"}},
		{"keywordresearchapi", "Search volume, difficulty, and keyword suggestions.", []string{"seo", "keywords"}},
		{"peopleapi", "Directory, roles, and user provisioning for the organization.", []string{"identity", "admin"}},
		{"currencyconverterapi", "Up-to-date exchange rates and instant multi-currency conversion.", []string{"finance", "currency"}},
		{"phoneverification", "Phone number verification and OTP codes via SMS.", []string{"otp", "identity"}},
		{"pizzashackapi", "Ordering, delivery tracking, and menu — the demo API.", []string{"pizza", "demo"}},
	}

	for _, c := range cases {
		var desc string
		var tags []string
		if err := pool.QueryRow(ctx,
			`SELECT description, tags FROM api_products WHERE slug=$1`, c.slug).Scan(&desc, &tags); err != nil {
			t.Fatalf("select %s: %v", c.slug, err)
		}
		if desc != c.description {
			t.Errorf("%s description = %q, want %q", c.slug, desc, c.description)
		}
		if !slices.Equal(tags, c.tags) {
			t.Errorf("%s tags = %v, want %v", c.slug, tags, c.tags)
		}
	}
}

// TestSeedEnMigrationDoesNotClobberCustomizedContent is a regression test for
// a code-review finding on #4's own fix: on an already-deployed instance
// where an admin customized a seeded product's description/tags before
// upgrading, the migration must not silently overwrite that admin-authored
// content back to the hardcoded English demo text. Mirrors the guard pattern
// 0004_seed_upstream.sql already uses (WHERE upstream_url = '').
func TestSeedEnMigrationDoesNotClobberCustomizedContent(t *testing.T) {
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

	const slug = "testapi"
	const customized = "Internal sandbox — customized by an admin before the #4 upgrade."

	// Re-execute 0023's actual SQL (loaded from the embedded FS, so this can't
	// drift from the real migration) directly against a simulated customized
	// row, instead of resetting schema_migrations to make Migrate() redo the
	// work. `go test ./...` runs every package's tests concurrently against
	// this same shared dev DB, and other packages' tests call Migrate() too —
	// deleting/re-inserting a global schema_migrations row here previously
	// raced with those and produced a duplicate-key error, or (worse) briefly
	// left a real seeded row corrupted if the assertion below failed before
	// cleanup ran. Re-running the file's own SQL only touches this one row.
	sql023, err := migrationsFS.ReadFile("migrations/0023_seed_en.sql")
	if err != nil {
		t.Fatalf("read 0023_seed_en.sql: %v", err)
	}

	// This test mutates a real seeded row on the shared dev DB, so the restore
	// MUST happen before the pool closes. A t.Cleanup here would run too late:
	// it fires after the test function (and its defers, including the
	// `defer pool.Close()` above) has already returned, so Exec against the
	// by-then-closed pool would silently no-op and leave the row corrupted —
	// exactly what happened the first time this test was written with
	// t.Cleanup. A plain defer registered after pool.Close() runs first
	// (LIFO), while the pool is still open.
	defer func() {
		if _, err := pool.Exec(ctx, `UPDATE api_products SET description=$1, tags=$2 WHERE slug=$3`,
			"Sandbox for validating your integrations before going to production.", []string{"sandbox", "internal"}, slug); err != nil {
			t.Errorf("restore original description: %v", err)
		}
	}()

	// Simulate an admin who customized this product's description on an
	// already-deployed instance before 0023 ships: neither the original
	// French seed nor the new English text.
	if _, err := pool.Exec(ctx, `UPDATE api_products SET description=$1 WHERE slug=$2`, customized, slug); err != nil {
		t.Fatalf("simulate admin customization: %v", err)
	}
	if _, err := pool.Exec(ctx, string(sql023)); err != nil {
		t.Fatalf("re-run 0023: %v", err)
	}

	var desc string
	if err := pool.QueryRow(ctx, `SELECT description FROM api_products WHERE slug=$1`, slug).Scan(&desc); err != nil {
		t.Fatalf("select: %v", err)
	}
	if desc != customized {
		t.Fatalf("admin-customized description was overwritten: got %q, want unchanged %q", desc, customized)
	}
}
