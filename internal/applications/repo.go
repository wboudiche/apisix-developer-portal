package applications

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"apisix-portal/internal/paging"
)

var ErrNotFound = errors.New("application not found")

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

func (r *Repo) Create(ctx context.Context, ownerID int64, name, description string) (Application, error) {
	var a Application
	err := r.pool.QueryRow(ctx,
		`INSERT INTO applications(owner_id,name,description) VALUES($1,$2,$3)
		 RETURNING id,owner_id,name,description,created_at`,
		ownerID, name, description,
	).Scan(&a.ID, &a.OwnerID, &a.Name, &a.Description, &a.CreatedAt)
	return a, err
}

func (r *Repo) ListByOwner(ctx context.Context, ownerID int64, p paging.Params) ([]Application, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM applications WHERE owner_id=$1`, ownerID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx,
		`SELECT a.id, a.owner_id, a.name, a.description, a.created_at,
		        (SELECT count(*) FROM subscriptions s WHERE s.application_id = a.id) AS sub_count,
		        EXISTS(SELECT 1 FROM credentials c WHERE c.application_id = a.id) AS has_key
		 FROM applications a
		 WHERE a.owner_id=$1 ORDER BY a.created_at DESC LIMIT $2 OFFSET $3`,
		ownerID, p.Limit(), p.Offset())
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Application
	for rows.Next() {
		var a Application
		if err := rows.Scan(&a.ID, &a.OwnerID, &a.Name, &a.Description, &a.CreatedAt, &a.SubscriptionCount, &a.HasKey); err != nil {
			return nil, 0, err
		}
		out = append(out, a)
	}
	return out, total, rows.Err()
}

func (r *Repo) Get(ctx context.Context, id, ownerID int64) (Application, error) {
	var a Application
	err := r.pool.QueryRow(ctx,
		`SELECT id,owner_id,name,description,created_at FROM applications WHERE id=$1 AND owner_id=$2`, id, ownerID,
	).Scan(&a.ID, &a.OwnerID, &a.Name, &a.Description, &a.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Application{}, ErrNotFound
	}
	return a, err
}
