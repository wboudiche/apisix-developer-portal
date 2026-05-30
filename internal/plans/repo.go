package plans

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("plan not found")

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

func (r *Repo) List(ctx context.Context) ([]Plan, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, rate_limit_count, rate_limit_window_s FROM plans ORDER BY rate_limit_count ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Plan
	for rows.Next() {
		var p Plan
		if err := rows.Scan(&p.ID, &p.Name, &p.RateLimit, &p.WindowSeconds); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
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
