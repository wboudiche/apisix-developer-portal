package catalog

import (
	"context"
	"os"
	"testing"

	"apisix-portal/internal/db"
	"apisix-portal/internal/paging"
)

func testPool(t *testing.T) (context.Context, *Repo) {
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

func TestListReturnsSeededProducts(t *testing.T) {
	ctx, repo := testPool(t)
	all, total, err := repo.List(ctx, Query{}, paging.Params{Page: 1, Size: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// At least the 9 seed products must be present and listable. Exact-count was
	// brittle: Plan 4a added admin product CRUD, so a live DB can legitimately
	// hold more than the seeds.
	if len(all) < 9 {
		t.Fatalf("expected at least 9 seeded products, got %d", len(all))
	}
	if total < 9 {
		t.Fatalf("expected total >= 9, got %d", total)
	}
}

func TestListFiltersByCategory(t *testing.T) {
	ctx, repo := testPool(t)
	fin, total, err := repo.List(ctx, Query{Category: "Finance"}, paging.Params{Page: 1, Size: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(fin) != 2 {
		t.Fatalf("expected 2 Finance products, got %d", len(fin))
	}
	if total != 2 {
		t.Fatalf("expected total 2 for Finance, got %d", total)
	}
}

func TestGetBySlug(t *testing.T) {
	ctx, repo := testPool(t)
	p, err := repo.GetBySlug(ctx, "pizzashackapi")
	if err != nil {
		t.Fatalf("GetBySlug: %v", err)
	}
	if p.Name != "PizzaShackAPI" {
		t.Fatalf("expected PizzaShackAPI, got %q", p.Name)
	}
}

func TestListSearchMatchesNameAndDescription(t *testing.T) {
	ctx, repo := testPool(t)
	// "pizza" appears in the PizzaShackAPI name
	byName, _, err := repo.List(ctx, Query{Search: "pizza"}, paging.Params{Page: 1, Size: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(byName) != 1 || byName[0].Slug != "pizzashackapi" {
		t.Fatalf("search 'pizza' => %d results, want 1 (pizzashackapi)", len(byName))
	}
	// "backlinks" appears only in the SEOAPI description, not any name
	byDesc, _, err := repo.List(ctx, Query{Search: "backlinks"}, paging.Params{Page: 1, Size: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(byDesc) != 1 || byDesc[0].Slug != "seoapi" {
		t.Fatalf("search 'backlinks' => %d results, want 1 (seoapi)", len(byDesc))
	}
	// case-insensitive
	ci, _, _ := repo.List(ctx, Query{Search: "PIZZA"}, paging.Params{Page: 1, Size: 20})
	if len(ci) != 1 {
		t.Fatalf("search 'PIZZA' (uppercase) => %d, want 1", len(ci))
	}
}

func TestListFiltersByTag(t *testing.T) {
	ctx, repo := testPool(t)
	seo, total, err := repo.List(ctx, Query{Tag: "seo"}, paging.Params{Page: 1, Size: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// seoapi and keywordresearchapi both carry the 'seo' tag
	if len(seo) != 2 {
		t.Fatalf("tag 'seo' => %d, want 2", len(seo))
	}
	if total != 2 {
		t.Fatalf("tag 'seo' total => %d, want 2", total)
	}
}

func TestGetBySlugNotFound(t *testing.T) {
	ctx, repo := testPool(t)
	if _, err := repo.GetBySlug(ctx, "does-not-exist"); err != ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
