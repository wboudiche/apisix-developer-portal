package admin

import (
	"context"
	"os"
	"testing"

	"apisix-portal/internal/db"

	"github.com/jackc/pgx/v5/pgxpool"
)

func iconTestRepo(t *testing.T) (context.Context, *Repo, *pgxpool.Pool) {
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

func seedIconProduct(t *testing.T, ctx context.Context, pool *pgxpool.Pool) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(ctx,
		`INSERT INTO api_products (name, slug, category, version, context_path, description, tags, icon)
		 VALUES ('IconProd', 'iconprod-'||floor(random()*1e9)::text, 'Engineering', '1.0.0',
		         '/iconprod'||floor(random()*1e9)::text, '', '{}', '') RETURNING id`).Scan(&id)
	if err != nil {
		t.Fatalf("seed product: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM api_products WHERE id=$1`, id) })
	return id
}

func TestSetUploadedIconUpsertsAndFlagsProduct(t *testing.T) {
	ctx, repo, pool := iconTestRepo(t)
	id := seedIconProduct(t, ctx, pool)

	if _, err := repo.SetUploadedIcon(ctx, id, []byte("PNGBYTES-1")); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	var icon string
	var data []byte
	if err := pool.QueryRow(ctx, `SELECT p.icon, i.data FROM api_products p JOIN product_icons i ON i.product_id=p.id WHERE p.id=$1`, id).
		Scan(&icon, &data); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if icon != "upload" || string(data) != "PNGBYTES-1" {
		t.Fatalf("got icon=%q data=%q", icon, data)
	}

	// upsert replaces on conflict
	if _, err := repo.SetUploadedIcon(ctx, id, []byte("PNGBYTES-2")); err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	_ = pool.QueryRow(ctx, `SELECT data FROM product_icons WHERE product_id=$1`, id).Scan(&data)
	if string(data) != "PNGBYTES-2" {
		t.Fatalf("upsert did not replace: %q", data)
	}
}

func TestDeleteIconRemovesRow(t *testing.T) {
	ctx, repo, pool := iconTestRepo(t)
	id := seedIconProduct(t, ctx, pool)
	if _, err := repo.SetUploadedIcon(ctx, id, []byte("X")); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteIcon(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	var n int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM product_icons WHERE product_id=$1`, id).Scan(&n)
	if n != 0 {
		t.Fatalf("row not deleted, count=%d", n)
	}
}

func TestProductDeleteCascadesIcon(t *testing.T) {
	ctx, repo, pool := iconTestRepo(t)
	id := seedIconProduct(t, ctx, pool)
	if _, err := repo.SetUploadedIcon(ctx, id, []byte("X")); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM api_products WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM product_icons WHERE product_id=$1`, id).Scan(&n)
	if n != 0 {
		t.Fatalf("cascade failed, count=%d", n)
	}
}
