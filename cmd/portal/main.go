package main

import (
	"context"
	"log"
	"net/http"

	"apisix-portal/internal/applications"
	"apisix-portal/internal/apisix"
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
	authH := auth.NewHandler(auth.NewRepo(pool), tok)
	plansH := plans.NewHandler(plans.NewRepo(pool))
	appsRepo := applications.NewRepo(pool)
	appsH := applications.NewHandler(appsRepo)
	subSvc := subscriptions.NewService(subscriptions.NewRepo(pool), gw, subscriptions.GenerateKey)
	owns := func(ctx context.Context, appID, userID int64) (bool, error) {
		if _, err := appsRepo.Get(ctx, appID, userID); err != nil {
			if err == applications.ErrNotFound {
				return false, nil
			}
			return false, err
		}
		return true, nil
	}
	subH := subscriptions.NewHandler(subSvc, owns)

	requireAuth := auth.RequireAuth(tok)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.Handle("/api/products", catalogH)
	mux.Handle("/api/products/", catalogH)
	mux.Handle("/api/auth/", authH)
	mux.Handle("/api/plans", plansH)
	mux.Handle("/api/applications", requireAuth(appsH))
	mux.Handle("/api/applications/", requireAuth(subH))

	log.Printf("portal listening on %s", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, logRequests(mux)); err != nil {
		log.Fatal(err)
	}
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
