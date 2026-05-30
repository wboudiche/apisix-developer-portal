package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireAuthRejectsMissingToken(t *testing.T) {
	tk := NewTokenizer("s")
	h := RequireAuth(tk)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token: status=%d want 401", rec.Code)
	}
}

func TestRequireAuthAllowsValidTokenAndExposesUserID(t *testing.T) {
	tk := NewTokenizer("s")
	token, _ := tk.Issue(7, "a@b.c", "developer")
	var seen int64
	h := RequireAuth(tk)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = UserID(r.Context())
		w.WriteHeader(200)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 || seen != 7 {
		t.Fatalf("valid token: status=%d userID=%d want 200/7", rec.Code, seen)
	}
}

func TestRequireAuthRejectsBadToken(t *testing.T) {
	tk := NewTokenizer("s")
	h := RequireAuth(tk)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer not.a.jwt")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad token: status=%d want 401", rec.Code)
	}
}

func TestWithUserIDRoundTrips(t *testing.T) {
	ctx := WithUserID(t.Context(), 99)
	if UserID(ctx) != 99 {
		t.Fatalf("WithUserID/UserID round trip failed: %d", UserID(ctx))
	}
}

func adminTestHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(Role(r.Context())))
	})
}

func TestRequireAdminAllowsAdmin(t *testing.T) {
	tk := NewTokenizer("s3cret")
	tok, err := tk.Issue(1, "boss@example.com", "admin")
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/products", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	RequireAdmin(tk)(adminTestHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "admin" {
		t.Fatalf("Role(ctx) = %q, want admin", rec.Body.String())
	}
}

func TestRequireAdminRejectsDeveloper(t *testing.T) {
	tk := NewTokenizer("s3cret")
	tok, _ := tk.Issue(2, "dev@example.com", "developer")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/products", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	RequireAdmin(tk)(adminTestHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestRequireAdminRejectsMissingToken(t *testing.T) {
	tk := NewTokenizer("s3cret")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/products", nil)

	RequireAdmin(tk)(adminTestHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}
