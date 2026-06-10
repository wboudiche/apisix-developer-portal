package auth

import (
	"context"
	"net/http"
	"strings"

	"apisix-portal/internal/httpx"
)

type ctxKey int

const (
	userIDKey ctxKey = 0
	roleKey   ctxKey = 1
)

// RequireAuth returns middleware that requires a valid Bearer JWT and stores
// the authenticated user id in the request context (read it with UserID).
func RequireAuth(tk *Tokenizer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			if !strings.HasPrefix(h, "Bearer ") {
				httpx.Error(w, http.StatusUnauthorized, "missing bearer token")
				return
			}
			claims, err := tk.Parse(strings.TrimPrefix(h, "Bearer "))
			if err != nil {
				httpx.Error(w, http.StatusUnauthorized, "invalid token")
				return
			}
			ctx := WithUserID(r.Context(), claims.UserID)
			ctx = WithRole(ctx, claims.Role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// WithUserID returns a context carrying the given authenticated user id.
// Used by RequireAuth and available to tests/handlers that bypass HTTP auth.
func WithUserID(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, userIDKey, id)
}

// UserID returns the authenticated user id, or 0 if unauthenticated.
func UserID(ctx context.Context) int64 {
	id, _ := ctx.Value(userIDKey).(int64)
	return id
}

// WithRole returns a context carrying the given role.
func WithRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, roleKey, role)
}

// Role returns the authenticated user's role, or "" if unauthenticated.
func Role(ctx context.Context) string {
	role, _ := ctx.Value(roleKey).(string)
	return role
}

// RoleLookup returns the current role for a user id (satisfied by *Repo.GetRole).
type RoleLookup func(ctx context.Context, userID int64) (string, error)

// RequireAdmin requires a valid Bearer JWT AND that the user's CURRENT role in
// the database is "admin" (the token claim alone is not trusted, so a demoted
// admin loses access immediately rather than for the token's lifetime). H5.
func RequireAdmin(tk *Tokenizer, lookup RoleLookup) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			if !strings.HasPrefix(h, "Bearer ") {
				httpx.Error(w, http.StatusUnauthorized, "missing bearer token")
				return
			}
			claims, err := tk.Parse(strings.TrimPrefix(h, "Bearer "))
			if err != nil {
				httpx.Error(w, http.StatusUnauthorized, "invalid token")
				return
			}
			role, err := lookup(r.Context(), claims.UserID)
			if err != nil {
				// A failed lookup (DB outage) is "could not verify", not "admin only".
				httpx.Error(w, http.StatusInternalServerError, "could not verify role")
				return
			}
			if role != "admin" {
				httpx.Error(w, http.StatusForbidden, "admin only")
				return
			}
			ctx := WithUserID(r.Context(), claims.UserID)
			ctx = WithRole(ctx, role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
