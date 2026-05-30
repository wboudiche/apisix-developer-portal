package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"apisix-portal/internal/admin"
	"apisix-portal/internal/apisix"
	"apisix-portal/internal/applications"
	"apisix-portal/internal/auth"
	"apisix-portal/internal/catalog"
	"apisix-portal/internal/config"
	"apisix-portal/internal/db"
	"apisix-portal/internal/plans"
	"apisix-portal/internal/subscriptions"
)

func main() {
	ctx := context.Background()
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("config: %v", err)
	}
	if cfg.UsesDevSecrets() {
		log.Printf("WARNING: using built-in dev secrets (JWT/APISIX admin key) — set JWT_SECRET and APISIX_ADMIN_KEY before any non-dev deploy")
	}

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	tok := auth.NewTokenizer(cfg.JWTSecret)
	gw := apisix.NewClient(cfg.APISIXAdminURL, cfg.APISIXAdminKey)

	catalogH := catalog.NewHandler(catalog.NewRepo(pool))
	authRepo := auth.NewRepo(pool)
	authH := auth.NewHandler(authRepo, tok)
	if err := authRepo.EnsureAdminRole(ctx, cfg.AdminEmail); err != nil {
		log.Printf("seed admin role (%s): %v", cfg.AdminEmail, err)
	}
	plansH := plans.NewHandler(plans.NewRepo(pool))
	appsRepo := applications.NewRepo(pool)
	appsH := applications.NewHandler(appsRepo)
	subRepo := subscriptions.NewRepo(pool)
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
	adminSvc := admin.NewService(admin.NewRepo(pool), subSvc)
	adminH := admin.NewHandler(adminSvc)
	planAdminSvc := admin.NewPlanService(admin.NewPlanRepo(pool), subSvc)
	planAdminH := admin.NewPlanHandler(planAdminSvc)
	subAdminH := subscriptions.NewAdminHandler(subSvc)

	requireAuth := auth.RequireAuth(tok)
	requireAdmin := auth.RequireAdmin(tok)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.Handle("/api/products", catalogH)
	mux.Handle("/api/products/", catalogH)
	mux.Handle("/api/auth/", authH)
	mux.Handle("/api/plans", plansH)
	mux.Handle("/api/applications", requireAuth(appsH))
	mux.Handle("/api/applications/", requireAuth(subH))
	mux.Handle("/api/admin/products", requireAdmin(adminH))
	mux.Handle("/api/admin/products/", requireAdmin(adminH))
	mux.Handle("/api/admin/plans", requireAdmin(planAdminH))
	mux.Handle("/api/admin/plans/", requireAdmin(planAdminH))
	mux.Handle("/api/admin/subscriptions", requireAdmin(subAdminH))
	mux.Handle("/api/admin/subscriptions/", requireAdmin(subAdminH))

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           logRequests(mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		log.Printf("portal listening on %s", cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Println("shutting down…")
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
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
