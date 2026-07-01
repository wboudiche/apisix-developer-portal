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

func (r *Repo) Create(ctx context.Context, ownerID, teamID int64, name, description string) (Application, error) {
	var a Application
	err := r.pool.QueryRow(ctx,
		`INSERT INTO applications(owner_id, team_id, name, description) VALUES($1,$2,$3,$4)
		 RETURNING id, owner_id, team_id, name, description, created_at`,
		ownerID, teamID, name, description,
	).Scan(&a.ID, &a.OwnerID, &a.TeamID, &a.Name, &a.Description, &a.CreatedAt)
	return a, err
}

// ListForUser returns apps across every team the user belongs to.
func (r *Repo) ListForUser(ctx context.Context, userID int64, p paging.Params) ([]Application, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM applications a
		 WHERE a.team_id IN (SELECT team_id FROM team_members WHERE user_id=$1)`, userID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx,
		`SELECT a.id, a.owner_id, a.team_id, t.name, a.name, a.description, a.created_at,
		        (SELECT count(*) FROM subscriptions s WHERE s.application_id = a.id) AS sub_count
		 FROM applications a JOIN teams t ON t.id = a.team_id
		 WHERE a.team_id IN (SELECT team_id FROM team_members WHERE user_id=$1)
		 ORDER BY a.created_at DESC LIMIT $2 OFFSET $3`,
		userID, p.Limit(), p.Offset())
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Application
	for rows.Next() {
		var a Application
		if err := rows.Scan(&a.ID, &a.OwnerID, &a.TeamID, &a.TeamName, &a.Name, &a.Description, &a.CreatedAt, &a.SubscriptionCount); err != nil {
			return nil, 0, err
		}
		out = append(out, a)
	}
	return out, total, rows.Err()
}

// Get fetches an application by id (membership is enforced by the caller).
func (r *Repo) Get(ctx context.Context, id int64) (Application, error) {
	var a Application
	err := r.pool.QueryRow(ctx,
		`SELECT a.id, a.owner_id, a.team_id, t.name, a.name, a.description, a.created_at
		 FROM applications a JOIN teams t ON t.id = a.team_id WHERE a.id=$1`, id,
	).Scan(&a.ID, &a.OwnerID, &a.TeamID, &a.TeamName, &a.Name, &a.Description, &a.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Application{}, ErrNotFound
	}
	return a, err
}
