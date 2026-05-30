package auth

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrEmailTaken is returned by Create when the email address is already registered.
var ErrEmailTaken = errors.New("email already registered")

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
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return User{}, ErrEmailTaken
		}
		return User{}, err
	}
	return u, nil
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

// EnsureAdminRole promotes the user with the given email to role 'admin'.
// Idempotent and a no-op if no such user exists yet (e.g. before first register).
func (r *Repo) EnsureAdminRole(ctx context.Context, email string) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET role='admin' WHERE email=$1`, email)
	return err
}
