package admin

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"apisix-portal/internal/paging"
)

// Plan-specific sentinels (product sentinels live in repo.go).
var (
	ErrPlanNotFound  = errors.New("admin: plan not found")
	ErrPlanNameTaken = errors.New("admin: plan name already exists")
	ErrPlanInUse     = errors.New("admin: plan is referenced by subscriptions")
)

// PlanRepo is the SQL store for admin plan management.
type PlanRepo struct{ pool *pgxpool.Pool }

func NewPlanRepo(pool *pgxpool.Pool) *PlanRepo { return &PlanRepo{pool: pool} }

const planCols = `id, name, rate_limit_count, rate_limit_window_s`

func scanPlan(row pgx.Row) (Plan, error) {
	var p Plan
	err := row.Scan(&p.ID, &p.Name, &p.RateLimit, &p.WindowSeconds)
	return p, err
}

func (r *PlanRepo) ListPlans(ctx context.Context, p paging.Params) ([]Plan, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM plans`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx,
		`SELECT `+planCols+` FROM plans ORDER BY rate_limit_count ASC LIMIT $1 OFFSET $2`,
		p.Limit(), p.Offset())
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Plan
	for rows.Next() {
		pl, err := scanPlan(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, pl)
	}
	return out, total, rows.Err()
}

func (r *PlanRepo) GetPlan(ctx context.Context, id int64) (Plan, error) {
	p, err := scanPlan(r.pool.QueryRow(ctx, `SELECT `+planCols+` FROM plans WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Plan{}, ErrPlanNotFound
	}
	return p, err
}

func (r *PlanRepo) CreatePlan(ctx context.Context, p Plan) (Plan, error) {
	created, err := scanPlan(r.pool.QueryRow(ctx,
		`INSERT INTO plans(name, rate_limit_count, rate_limit_window_s)
		 VALUES($1,$2,$3) RETURNING `+planCols,
		p.Name, p.RateLimit, p.WindowSeconds))
	if isUniqueViolation(err) {
		return Plan{}, ErrPlanNameTaken
	}
	return created, err
}

func (r *PlanRepo) UpdatePlan(ctx context.Context, p Plan) (Plan, error) {
	updated, err := scanPlan(r.pool.QueryRow(ctx,
		`UPDATE plans SET name=$2, rate_limit_count=$3, rate_limit_window_s=$4
		 WHERE id=$1 RETURNING `+planCols,
		p.ID, p.Name, p.RateLimit, p.WindowSeconds))
	if errors.Is(err, pgx.ErrNoRows) {
		return Plan{}, ErrPlanNotFound
	}
	if isUniqueViolation(err) {
		return Plan{}, ErrPlanNameTaken
	}
	return updated, err
}

func (r *PlanRepo) DeletePlan(ctx context.Context, id int64) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM plans WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrPlanNotFound
	}
	return nil
}

func (r *PlanRepo) CountSubscriptionsForPlan(ctx context.Context, planID int64) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM subscriptions WHERE plan_id=$1`, planID).Scan(&n)
	return n, err
}
