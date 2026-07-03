package notify

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

// Recipient is an email address plus the recipient's stored UI language.
type Recipient struct {
	Email string
	Lang  string
}

// OwnerEmailsForApp returns the owners of the app's team (email + language) + the app name.
func (r *Repo) OwnerEmailsForApp(ctx context.Context, appID int64) ([]Recipient, string, error) {
	var name string
	if err := r.pool.QueryRow(ctx, `SELECT name FROM applications WHERE id=$1`, appID).Scan(&name); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", nil
		}
		return nil, "", err
	}
	rows, err := r.pool.Query(ctx,
		`SELECT u.email, u.language FROM applications a
		 JOIN team_members tm ON tm.team_id = a.team_id AND tm.role='owner'
		 JOIN users u ON u.id = tm.user_id
		 WHERE a.id=$1`, appID)
	if err != nil {
		return nil, name, err
	}
	defer rows.Close()
	var out []Recipient
	for rows.Next() {
		var rc Recipient
		if err := rows.Scan(&rc.Email, &rc.Lang); err != nil {
			return nil, name, err
		}
		out = append(out, rc)
	}
	return out, name, rows.Err()
}

// AdminEmails returns all admin users (email + language).
func (r *Repo) AdminEmails(ctx context.Context) ([]Recipient, error) {
	rows, err := r.pool.Query(ctx, `SELECT email, language FROM users WHERE role='admin'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Recipient
	for rows.Next() {
		var rc Recipient
		if err := rows.Scan(&rc.Email, &rc.Lang); err != nil {
			return nil, err
		}
		out = append(out, rc)
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
