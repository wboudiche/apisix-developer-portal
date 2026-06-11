package server

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"apisix-portal/internal/admin"
	"apisix-portal/internal/apisix"
	"apisix-portal/internal/applications"
	"apisix-portal/internal/auth"
	"apisix-portal/internal/catalog"
	"apisix-portal/internal/config"
	"apisix-portal/internal/crypto"
	"apisix-portal/internal/httpx"
	"apisix-portal/internal/plans"
	"apisix-portal/internal/subscriptions"
)

// New builds the portal's HTTP handler: all API routes wired to the given
// database pool, config, and APISIX gateway. It also seeds the admin role for
// cfg.AdminEmail (best-effort). Extracted from main so tests can mount the real
// app in-process via httptest.
func New(ctx context.Context, pool *pgxpool.Pool, cfg config.Config, gw apisix.Gateway) http.Handler {
	tok := auth.NewTokenizer(cfg.JWTSecret)

	catalogH := catalog.NewHandler(catalog.NewRepo(pool))
	authRepo := auth.NewRepo(pool)
	ipLimiter := httpx.NewRateLimiter(10, 0.5)    // per client IP, all /api/auth/ endpoints
	loginLimiter := httpx.NewRateLimiter(10, 0.5) // per account (email), login only
	if proxies, err := httpx.ParseProxyCIDRs(cfg.TrustedProxies); err != nil {
		log.Fatalf("TRUSTED_PROXIES: %v", err)
	} else if proxies != nil {
		// Behind a reverse proxy, RemoteAddr is the proxy; honor its
		// X-Forwarded-For so per-IP limiting isolates real clients.
		ipLimiter.SetTrustedProxies(proxies)
	}
	authH := auth.NewHandler(authRepo, tok, loginLimiter)
	if err := authRepo.EnsureAdminRole(ctx, cfg.AdminEmail); err != nil {
		log.Printf("seed admin role (%s): %v", cfg.AdminEmail, err)
	}
	plansH := plans.NewHandler(plans.NewRepo(pool))
	appsRepo := applications.NewRepo(pool)
	appsH := applications.NewHandler(appsRepo)
	cipher, err := crypto.New(cfg.CredentialEncKey)
	if err != nil {
		log.Fatalf("credential cipher: %v", err)
	}
	subRepo := subscriptions.NewRepo(pool, cipher)
	subSvc := subscriptions.NewService(subRepo, gw, subscriptions.GenerateKey)
	owns := func(ctx context.Context, appID, userID int64) (bool, error) {
		if _, err := appsRepo.Get(ctx, appID, userID); err != nil {
			if err == applications.ErrNotFound {
				return false, nil
			}
			return false, err
		}
		return true, nil
	}
	subH := subscriptions.NewHandler(subSvc, subRepo, owns)
	allowPrivate := os.Getenv("UPSTREAM_ALLOW_PRIVATE") == "1"
	adminSvc := admin.NewService(admin.NewRepo(pool), subSvc)
	adminH := admin.NewHandler(adminSvc, allowPrivate)
	planAdminSvc := admin.NewPlanService(admin.NewPlanRepo(pool), subSvc)
	planAdminH := admin.NewPlanHandler(planAdminSvc)
	subAdminH := subscriptions.NewAdminHandler(subSvc)

	requireAuth := auth.RequireAuth(tok)
	requireAdmin := auth.RequireAdmin(tok, authRepo.GetRole)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.Handle("/api/products", catalogH)
	mux.Handle("/api/products/", catalogH)
	mux.Handle("/api/auth/", ipLimiter.Middleware()(authH))
	mux.Handle("/api/plans", plansH)
	mux.Handle("/api/applications", requireAuth(appsH))
	mux.Handle("/api/applications/", requireAuth(subH))
	mux.Handle("/api/admin/products", requireAdmin(adminH))
	mux.Handle("/api/admin/products/", requireAdmin(adminH))
	mux.Handle("/api/admin/plans", requireAdmin(planAdminH))
	mux.Handle("/api/admin/plans/", requireAdmin(planAdminH))
	mux.Handle("/api/admin/subscriptions", requireAdmin(subAdminH))
	mux.Handle("/api/admin/subscriptions/", requireAdmin(subAdminH))

	return httpx.SecurityHeaders(httpx.MaxBodyBytes(1<<20)(logRequests(mux)))
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Printf("%s %s -> %d", r.Method, r.URL.Path, rec.status)
	})
}
