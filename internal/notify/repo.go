package notify

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

// OwnerEmailsForApp returns the emails of the owners of the app's team + the app name.
func (r *Repo) OwnerEmailsForApp(ctx context.Context, appID int64) ([]string, string, error) {
	var name string
	if err := r.pool.QueryRow(ctx, `SELECT name FROM applications WHERE id=$1`, appID).Scan(&name); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", nil
		}
		return nil, "", err
	}
	rows, err := r.pool.Query(ctx,
		`SELECT u.email FROM applications a
		 JOIN team_members tm ON tm.team_id = a.team_id AND tm.role='owner'
		 JOIN users u ON u.id = tm.user_id
		 WHERE a.id=$1`, appID)
	if err != nil {
		return nil, name, err
	}
	defer rows.Close()
	var emails []string
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			return nil, name, err
		}
		emails = append(emails, e)
	}
	return emails, name, rows.Err()
}

// AdminEmails returns the emails of all admin users.
func (r *Repo) AdminEmails(ctx context.Context) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT email FROM users WHERE role='admin'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *Repo) ProductName(ctx context.Context, productID int64) (string, error) {
	var n string
	err := r.pool.QueryRow(ctx, `SELECT name FROM api_products WHERE id=$1`, productID).Scan(&n)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return n, err
}

func (r *Repo) PlanName(ctx context.Context, planID int64) (string, error) {
	var n string
	err := r.pool.QueryRow(ctx, `SELECT name FROM plans WHERE id=$1`, planID).Scan(&n)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return n, err
}
