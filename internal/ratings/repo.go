// Package ratings stores per-user API ratings and keeps the product's cached
// average + count in sync.
package ratings

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Review struct {
	Stars     int       `json:"stars"`
	Comment   string    `json:"comment"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"createdAt"`
}

type Summary struct {
	Average float64 `json:"average"`
	Count   int     `json:"count"`
}

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

// Upsert writes (or updates) the user's rating and recomputes the product's
// cached average + count, atomically.
func (r *Repo) Upsert(ctx context.Context, productID, userID int64, stars int, comment string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`INSERT INTO product_ratings(api_product_id, user_id, stars, comment)
		 VALUES($1,$2,$3,$4)
		 ON CONFLICT (api_product_id, user_id)
		 DO UPDATE SET stars=EXCLUDED.stars, comment=EXCLUDED.comment, updated_at=now()`,
		productID, userID, stars, comment); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE api_products SET
		   rating = COALESCE((SELECT AVG(stars) FROM product_ratings WHERE api_product_id=$1), 0),
		   rating_count = (SELECT count(*) FROM product_ratings WHERE api_product_id=$1)
		 WHERE id=$1`, productID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repo) List(ctx context.Context, productID int64) ([]Review, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT pr.stars, pr.comment, COALESCE(NULLIF(u.name,''),'Développeur'), pr.created_at
		   FROM product_ratings pr JOIN users u ON u.id = pr.user_id
		 WHERE pr.api_product_id=$1 ORDER BY pr.created_at DESC`, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Review
	for rows.Next() {
		var rv Review
		if err := rows.Scan(&rv.Stars, &rv.Comment, &rv.Author, &rv.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, rv)
	}
	return out, rows.Err()
}

func (r *Repo) Mine(ctx context.Context, productID, userID int64) (*Review, error) {
	var rv Review
	err := r.pool.QueryRow(ctx,
		`SELECT pr.stars, pr.comment, COALESCE(NULLIF(u.name,''),'Développeur'), pr.created_at
		   FROM product_ratings pr JOIN users u ON u.id = pr.user_id
		 WHERE pr.api_product_id=$1 AND pr.user_id=$2`, productID, userID).
		Scan(&rv.Stars, &rv.Comment, &rv.Author, &rv.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rv, nil
}

func (r *Repo) SummaryFor(ctx context.Context, productID int64) (Summary, error) {
	var s Summary
	err := r.pool.QueryRow(ctx,
		`SELECT rating, rating_count FROM api_products WHERE id=$1`, productID).Scan(&s.Average, &s.Count)
	return s, err
}
