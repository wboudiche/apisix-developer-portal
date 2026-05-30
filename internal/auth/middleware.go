package auth

import (
	"context"
	"net/http"
	"strings"

	"apisix-portal/internal/httpx"
)

type ctxKey int

const userIDKey ctxKey = 0

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
			next.ServeHTTP(w, r.WithContext(WithUserID(r.Context(), claims.UserID)))
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
