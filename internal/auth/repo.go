package auth

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrEmailTaken is returned by Create when the email address is already registered.
var ErrEmailTaken = errors.New("email already registered")

// ErrUserNotFound is returned by GetRole when the user no longer exists.
var ErrUserNotFound = errors.New("auth: user not found")

// ErrTokenInvalid is returned by VerifyByTokenHash when no user carries the
// hash or the token has expired.
var ErrTokenInvalid = errors.New("auth: verification token invalid or expired")

// ErrAlreadyVerified is returned by ResetVerifyToken for accounts that no
// longer need verification.
var ErrAlreadyVerified = errors.New("auth: email already verified")

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

// Create inserts a developer user AND their personal team (a team of one) in a
// single transaction, returning the user. The user is email-verified (the
// column default) — used when REQUIRE_EMAIL_VERIFICATION is off.
func (r *Repo) Create(ctx context.Context, email, passwordHash, name, lang string) (User, error) {
	return r.create(ctx, email, passwordHash, name, lang, "", nil)
}

// CreateUnverified is Create with email_verified=FALSE plus a pending
// verification token (hash + expiry) — used when REQUIRE_EMAIL_VERIFICATION
// is on.
func (r *Repo) CreateUnverified(ctx context.Context, email, passwordHash, name, lang, verifyTokenHash string, expiresAt time.Time) (User, error) {
	return r.create(ctx, email, passwordHash, name, lang, verifyTokenHash, &expiresAt)
}

func (r *Repo) create(ctx context.Context, email, passwordHash, name, lang, tokenHash string, expiresAt *time.Time) (User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback(ctx)
	var u User
	if expiresAt == nil {
		err = tx.QueryRow(ctx,
			`INSERT INTO users (email, password_hash, name, role, language)
			 VALUES ($1,$2,$3,'developer',$4)
			 RETURNING id, email, name, role, language, email_verified`,
			email, passwordHash, name, lang,
		).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.Language, &u.Verified)
	} else {
		err = tx.QueryRow(ctx,
			`INSERT INTO users (email, password_hash, name, role, language, email_verified, verify_token_hash, verify_token_expires_at)
			 VALUES ($1,$2,$3,'developer',$4, FALSE, $5, $6)
			 RETURNING id, email, name, role, language, email_verified`,
			email, passwordHash, name, lang, tokenHash, *expiresAt,
		).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.Language, &u.Verified)
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return User{}, ErrEmailTaken
		}
		return User{}, err
	}
	teamName := name
	if teamName == "" {
		teamName = email
	}
	var teamID int64
	if err := tx.QueryRow(ctx,
		`INSERT INTO teams(name, personal) VALUES($1, true) RETURNING id`, teamName).Scan(&teamID); err != nil {
		return User{}, err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO team_members(team_id, user_id, role) VALUES($1,$2,'owner')`, teamID, u.ID); err != nil {
		return User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, err
	}
	return u, nil
}

// GetByEmail returns the user and its password hash.
func (r *Repo) GetByEmail(ctx context.Context, email string) (User, string, error) {
	var u User
	var hash string
	err := r.pool.QueryRow(ctx,
		`SELECT id, email, name, role, language, email_verified, password_hash FROM users WHERE email=$1`, email,
	).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.Language, &u.Verified, &hash)
	return u, hash, err
}

// VerifyByTokenHash marks the user carrying this token hash as verified and
// burns the token. ErrTokenInvalid covers unknown, already-used and expired
// tokens alike (they are indistinguishable to the caller by design).
func (r *Repo) VerifyByTokenHash(ctx context.Context, tokenHash string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE users
		 SET email_verified = TRUE, verify_token_hash = NULL, verify_token_expires_at = NULL
		 WHERE verify_token_hash = $1 AND verify_token_expires_at > now()`, tokenHash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrTokenInvalid
	}
	return nil
}

// ResetVerifyToken stores a fresh token hash/expiry for an unverified account
// (invalidating any previous link) and returns the user for email rendering.
// The not-yet-verified guard is part of the UPDATE itself (atomic), so a
// token can never be written onto an account that a concurrent
// VerifyByTokenHash just verified.
func (r *Repo) ResetVerifyToken(ctx context.Context, email, tokenHash string, expiresAt time.Time) (User, error) {
	var u User
	err := r.pool.QueryRow(ctx,
		`UPDATE users SET verify_token_hash=$2, verify_token_expires_at=$3
		 WHERE email=$1 AND NOT email_verified
		 RETURNING id, email, name, role, language`,
		email, tokenHash, expiresAt,
	).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.Language)
	if errors.Is(err, pgx.ErrNoRows) {
		// Nothing was written; classify why with a read-only lookup:
		// no such account, or an account that no longer needs verification.
		var exists bool
		if err := r.pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM users WHERE email=$1)`, email).Scan(&exists); err != nil {
			return User{}, err
		}
		if !exists {
			return User{}, ErrUserNotFound
		}
		return User{}, ErrAlreadyVerified
	}
	if err != nil {
		return User{}, err
	}
	return u, nil
}

// SetLanguage updates the user's stored UI language ('fr'|'en').
func (r *Repo) SetLanguage(ctx context.Context, userID int64, lang string) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET language=$2 WHERE id=$1`, userID, lang)
	return err
}

// EnsureAdminRole promotes the user with the given email to role 'admin'.
// Idempotent and a no-op if no such user exists yet (e.g. before first register).
func (r *Repo) EnsureAdminRole(ctx context.Context, email string) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET role='admin' WHERE email=$1`, email)
	return err
}

// GetRole returns the current role of the user with the given id.
// Returns ErrUserNotFound if no such user exists.
func (r *Repo) GetRole(ctx context.Context, userID int64) (string, error) {
	var role string
	err := r.pool.QueryRow(ctx, `SELECT role FROM users WHERE id=$1`, userID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrUserNotFound
	}
	return role, err
}
