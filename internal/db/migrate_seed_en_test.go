package db

import (
	"context"
	"os"
	"strings"
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
		if strings.Join(tags, ",") != strings.Join(c.tags, ",") {
			t.Errorf("%s tags = %v, want %v", c.slug, tags, c.tags)
		}
	}
}
