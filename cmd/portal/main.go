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

	"apisix-portal/internal/apisix"
	"apisix-portal/internal/config"
	"apisix-portal/internal/db"
	"apisix-portal/internal/server"
)

func main() {
	ctx := context.Background()
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("config: %v", err)
	}
	if cfg.UsesDevSecrets() {
		log.Printf("WARNING: using built-in dev secrets — set JWT_SECRET, APISIX_ADMIN_KEY and CREDENTIAL_ENC_KEY before any non-dev deploy")
	}

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	gw := apisix.NewClient(cfg.APISIXAdminURL, cfg.APISIXAdminKey)
	// Best-effort: enable gateway request metrics for the KPI cards. A failure
	// here only means the metrics endpoint stays empty; the portal still runs.
	if err := gw.EnsureGlobalPrometheus(ctx); err != nil {
		log.Printf("enable gateway prometheus metrics: %v", err)
	}
	handler := server.New(ctx, pool, cfg, gw)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
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
