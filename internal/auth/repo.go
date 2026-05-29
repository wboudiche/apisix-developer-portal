package auth

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

// Create inserts a developer user and returns it.
func (r *Repo) Create(ctx context.Context, email, passwordHash, name string) (User, error) {
	var u User
	err := r.pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, name, role)
		 VALUES ($1,$2,$3,'developer')
		 RETURNING id, email, name, role`,
		email, passwordHash, name,
	).Scan(&u.ID, &u.Email, &u.Name, &u.Role)
	return u, err
}

// GetByEmail returns the user and its password hash.
func (r *Repo) GetByEmail(ctx context.Context, email string) (User, string, error) {
	var u User
	var hash string
	err := r.pool.QueryRow(ctx,
		`SELECT id, email, name, role, password_hash FROM users WHERE email=$1`, email,
	).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &hash)
	return u, hash, err
}
