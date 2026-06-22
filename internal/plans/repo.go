package plans

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"apisix-portal/internal/paging"
)

var ErrNotFound = errors.New("plan not found")

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

func (r *Repo) List(ctx context.Context, p paging.Params) ([]Plan, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM plans`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, rate_limit_count, rate_limit_window_s FROM plans
		 ORDER BY rate_limit_count ASC LIMIT $1 OFFSET $2`, p.Limit(), p.Offset())
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Plan
	for rows.Next() {
		var pl Plan
		if err := rows.Scan(&pl.ID, &pl.Name, &pl.RateLimit, &pl.WindowSeconds); err != nil {
			return nil, 0, err
		}
		out = append(out, pl)
	}
	return out, total, rows.Err()
}

func (r *Repo) GetByID(ctx context.Context, id int64) (Plan, error) {
	var p Plan
	err := r.pool.QueryRow(ctx,
		`SELECT id, name, rate_limit_count, rate_limit_window_s FROM plans WHERE id=$1`, id,
	).Scan(&p.ID, &p.Name, &p.RateLimit, &p.WindowSeconds)
	if errors.Is(err, pgx.ErrNoRows) {
		return Plan{}, ErrNotFound
	}
	return p, err
}
