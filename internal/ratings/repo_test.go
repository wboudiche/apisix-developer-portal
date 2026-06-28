package ratings

import (
	"context"
	"os"
	"testing"
	"time"

	"apisix-portal/internal/db"
)

func testRepo(t *testing.T) (context.Context, *Repo, int64, int64) {
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
	suf := time.Now().Format("150405.000000000")
	var uid, pid int64
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,name) VALUES($1,'x',$2) RETURNING id`,
		"rater+"+suf+"@e.com", "Rater "+suf).Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO api_products(name,slug,category,context_path,published) VALUES($1,$2,'C','/r',true) RETURNING id`,
		"RateProd "+suf, "rateprod-"+suf).Scan(&pid); err != nil {
		t.Fatalf("seed product: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM api_products WHERE id=$1`, pid)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, uid)
	})
	return ctx, NewRepo(pool), uid, pid
}

func TestUpsertRecomputesAndIsOnePerUser(t *testing.T) {
	ctx, repo, uid, pid := testRepo(t)
	if err := repo.Upsert(ctx, pid, uid, 4, "bien"); err != nil {
		t.Fatalf("upsert1: %v", err)
	}
	s, _ := repo.SummaryFor(ctx, pid)
	if s.Count != 1 || s.Average != 4 {
		t.Fatalf("after 1: %+v", s)
	}
	// Same user re-rates: updates, does not insert a second row.
	if err := repo.Upsert(ctx, pid, uid, 2, "finalement bof"); err != nil {
		t.Fatalf("upsert2: %v", err)
	}
	s, _ = repo.SummaryFor(ctx, pid)
	if s.Count != 1 || s.Average != 2 {
		t.Fatalf("after re-rate: %+v", s)
	}
	mine, err := repo.Mine(ctx, pid, uid)
	if err != nil || mine == nil || mine.Stars != 2 || mine.Comment != "finalement bof" {
		t.Fatalf("mine: %+v %v", mine, err)
	}
	list, err := repo.List(ctx, pid)
	if err != nil || len(list) != 1 || list[0].Author == "" {
		t.Fatalf("list: %+v %v", list, err)
	}
}

func TestMineNilWhenNoRating(t *testing.T) {
	ctx, repo, uid, pid := testRepo(t)
	mine, err := repo.Mine(ctx, pid, uid)
	if err != nil || mine != nil {
		t.Fatalf("mine = %+v, %v (want nil)", mine, err)
	}
}
