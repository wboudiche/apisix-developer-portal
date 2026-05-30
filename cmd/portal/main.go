package main

import (
	"context"
	"log"
	"net/http"

	"apisix-portal/internal/auth"
	"apisix-portal/internal/catalog"
	"apisix-portal/internal/config"
	"apisix-portal/internal/db"
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

	catalogH := catalog.NewHandler(catalog.NewRepo(pool))
	authH := auth.NewHandler(auth.NewRepo(pool), auth.NewTokenizer(cfg.JWTSecret))

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.Handle("/api/products", catalogH)
	mux.Handle("/api/products/", catalogH)
	mux.Handle("/api/auth/", authH)

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
