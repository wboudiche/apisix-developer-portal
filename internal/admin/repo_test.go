package admin

import (
	"context"
	"os"
	"testing"
	"time"

	"apisix-portal/internal/db"
)

// adminTestRepo connects to a live database (skipping the test if none is
// available) and returns a ready-to-use Repo. Mirrors catalog.testPool /
// subscriptions.testRepo.
func adminTestRepo(t *testing.T) (context.Context, *Repo) {
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

func TestProductLifecycleRoundTrip(t *testing.T) {
	ctx, repo := adminTestRepo(t)
	sunset := "2026-12-31"
	p, err := repo.Create(ctx, Product{
		Name: "Lc", Slug: "lc-" + time.Now().Format("150405.000000000"), Category: "C",
		ContextPath: "/lc", Published: true, Tags: []string{}, LifecycleStatus: "sunset", SunsetDate: &sunset,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { _, _ = repo.pool.Exec(ctx, `DELETE FROM api_products WHERE id=$1`, p.ID) })
	if p.LifecycleStatus != "sunset" || p.SunsetDate == nil || *p.SunsetDate != "2026-12-31" {
		t.Fatalf("lifecycle round-trip: %+v", p)
	}
}

func TestProductLifecycleDefaultsToActive(t *testing.T) {
	ctx, repo := adminTestRepo(t)
	p, err := repo.Create(ctx, Product{
		Name: "LcDef", Slug: "lcdef-" + time.Now().Format("150405.000000000"), Category: "C",
		ContextPath: "/lcdef", Published: true, Tags: []string{},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { _, _ = repo.pool.Exec(ctx, `DELETE FROM api_products WHERE id=$1`, p.ID) })
	if p.LifecycleStatus != "active" {
		t.Fatalf("expected default lifecycle_status active, got %q", p.LifecycleStatus)
	}
	if p.SunsetDate != nil {
		t.Fatalf("expected nil sunsetDate, got %v", *p.SunsetDate)
	}
}

// specColumn reads the raw openapi_spec column directly, since productCols
// (and therefore Create/Update/Get's returned Product) never re-selects it.
func specColumn(t *testing.T, ctx context.Context, repo *Repo, id int64) string {
	t.Helper()
	var spec string
	if err := repo.pool.QueryRow(ctx, `SELECT openapi_spec FROM api_products WHERE id=$1`, id).Scan(&spec); err != nil {
		t.Fatalf("read openapi_spec: %v", err)
	}
	return spec
}

// TestUpdateRemovesOpenapiSpec is a regression test for #10: there was no way
// to clear an attached spec — an empty OpenAPISpec on Update means "leave it
// untouched" (see the COALESCE in Update's SQL), so removal needs the explicit
// RemoveOpenapiSpec flag to actually blank the column.
func TestUpdateRemovesOpenapiSpec(t *testing.T) {
	ctx, repo := adminTestRepo(t)
	spec := `{"openapi":"3.0.0","info":{"title":"X","version":"1.0.0"}}`
	p, err := repo.Create(ctx, Product{
		Name: "Sp", Slug: "sp-" + time.Now().Format("150405.000000000"), Category: "C",
		ContextPath: "/sp", Tags: []string{}, OpenAPISpec: spec,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { _, _ = repo.pool.Exec(ctx, `DELETE FROM api_products WHERE id=$1`, p.ID) })
	if got := specColumn(t, ctx, repo, p.ID); got != spec {
		t.Fatalf("spec not stored on create: %q", got)
	}

	// An update with an empty OpenAPISpec and no removal flag must keep the
	// existing spec (unchanged pre-existing behavior).
	unrelated := p
	unrelated.OpenAPISpec = ""
	if _, err := repo.Update(ctx, unrelated); err != nil {
		t.Fatalf("update (unrelated field): %v", err)
	}
	if got := specColumn(t, ctx, repo, p.ID); got != spec {
		t.Fatalf("spec should be kept when RemoveOpenapiSpec is false, got %q", got)
	}

	// Setting RemoveOpenapiSpec must blank the column, even though
	// OpenAPISpec itself is also empty in the same request.
	removal := p
	removal.OpenAPISpec = ""
	removal.RemoveOpenapiSpec = true
	if _, err := repo.Update(ctx, removal); err != nil {
		t.Fatalf("update (remove spec): %v", err)
	}
	if got := specColumn(t, ctx, repo, p.ID); got != "" {
		t.Fatalf("expected spec cleared, got %q", got)
	}
}

func TestChangelogAddDelete(t *testing.T) {
	ctx, repo := adminTestRepo(t)
	p, err := repo.Create(ctx, Product{
		Name: "Cl", Slug: "cl-" + time.Now().Format("150405.000000000"), Category: "C",
		ContextPath: "/cl2", Published: true, Tags: []string{},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { _, _ = repo.pool.Exec(ctx, `DELETE FROM api_products WHERE id=$1`, p.ID) })

	e, err := repo.AddChangelog(ctx, p.ID, ChangelogEntry{Version: "v2", Kind: "changed", Notes: "n", Date: "2026-03-01"})
	if err != nil || e.ID == 0 {
		t.Fatalf("add: %+v %v", e, err)
	}
	if e.Version != "v2" || e.Kind != "changed" || e.Notes != "n" || e.Date != "2026-03-01" {
		t.Fatalf("add returned unexpected entry: %+v", e)
	}

	if err := repo.DeleteChangelog(ctx, p.ID, e.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := repo.DeleteChangelog(ctx, p.ID, e.ID); err != ErrNotFound {
		t.Fatalf("delete missing err = %v, want ErrNotFound", err)
	}
}

// TestListChangelogReturnsEntryForUnpublishedProduct guards against the admin
// changelog editor going blind on a draft: ListChangelog must not filter on
// published, unlike the public catalog changelog listing.
func TestListChangelogReturnsEntryForUnpublishedProduct(t *testing.T) {
	ctx, repo := adminTestRepo(t)
	p, err := repo.Create(ctx, Product{
		Name: "ClList", Slug: "cl-list-" + time.Now().Format("150405.000000000"), Category: "C",
		ContextPath: "/cl-list", Published: false, Tags: []string{},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { _, _ = repo.pool.Exec(ctx, `DELETE FROM api_products WHERE id=$1`, p.ID) })

	seeded, err := repo.AddChangelog(ctx, p.ID, ChangelogEntry{Version: "v1", Kind: "added", Notes: "seed", Date: "2026-01-01"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	got, err := repo.ListChangelog(ctx, p.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].ID != seeded.ID || got[0].Version != "v1" || got[0].Kind != "added" || got[0].Notes != "seed" || got[0].Date != "2026-01-01" {
		t.Fatalf("list changelog for draft product = %+v, want the seeded entry", got)
	}
}
