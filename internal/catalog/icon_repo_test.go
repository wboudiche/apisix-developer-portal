package catalog

import (
	"context"
	"os"
	"testing"

	"apisix-portal/internal/db"

	"github.com/jackc/pgx/v5/pgxpool"
)

func iconRepo(t *testing.T) (context.Context, *Repo, *pgxpool.Pool) {
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
	return ctx, NewRepo(pool), pool
}

func seedIconProduct(t *testing.T, ctx context.Context, pool *pgxpool.Pool, published bool) string {
	t.Helper()
	var id int64
	var slug string
	err := pool.QueryRow(ctx,
		`INSERT INTO api_products (name, slug, category, version, context_path, description, tags, icon, published)
		 VALUES ('IconCat', 'iconcat-'||floor(random()*1e9)::text, 'Engineering', '1.0.0',
		         '/iconcat'||floor(random()*1e9)::text, '', '{}', 'upload', $1)
		 RETURNING id, slug`, published).Scan(&id, &slug)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO product_icons (product_id, data) VALUES ($1, $2)`, id, []byte("PNG")); err != nil {
		t.Fatalf("seed icon: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM api_products WHERE id=$1`, id) })
	return slug
}

func TestGetIconBySlugServesPublished(t *testing.T) {
	ctx, repo, pool := iconRepo(t)
	slug := seedIconProduct(t, ctx, pool, true)
	data, _, err := repo.GetIconBySlug(ctx, slug)
	if err != nil || string(data) != "PNG" {
		t.Fatalf("published icon: data=%q err=%v", data, err)
	}
}

func TestGetIconBySlugHidesUnpublished(t *testing.T) {
	ctx, repo, pool := iconRepo(t)
	slug := seedIconProduct(t, ctx, pool, false)
	if _, _, err := repo.GetIconBySlug(ctx, slug); err != ErrNotFound {
		t.Fatalf("unpublished icon: want ErrNotFound, got %v", err)
	}
}
