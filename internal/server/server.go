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
	"apisix-portal/internal/billing"
	"apisix-portal/internal/catalog"
	"apisix-portal/internal/config"
	"apisix-portal/internal/crypto"
	"apisix-portal/internal/events"
	"apisix-portal/internal/httpx"
	"apisix-portal/internal/i18n"
	"apisix-portal/internal/metrics"
	"apisix-portal/internal/notify"
	"apisix-portal/internal/plans"
	"apisix-portal/internal/ratings"
	"apisix-portal/internal/subscriptions"
	"apisix-portal/internal/teams"
	"apisix-portal/internal/tryit"
)

// New builds the portal's HTTP handler: all API routes wired to the given
// database pool, config, and APISIX gateway. It also seeds the admin role for
// cfg.AdminEmail (best-effort). Extracted from main so tests can mount the real
// app in-process via httptest.
func New(ctx context.Context, pool *pgxpool.Pool, cfg config.Config, gw apisix.Gateway) http.Handler {
	tok := auth.NewTokenizer(cfg.JWTSecret)

	catRepo := catalog.NewRepo(pool)
	catalogH := catalog.NewHandler(catRepo)
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
	eventRepo := events.NewRepo(pool)
	appsRepo := applications.NewRepo(pool)
	teamsRepo := teams.NewRepo(pool)
	appsH := applications.NewHandler(appsRepo, teamsRepo, eventRepo)
	cipher, err := crypto.New(cfg.CredentialEncKey)
	if err != nil {
		log.Fatalf("credential cipher: %v", err)
	}
	subRepo := subscriptions.NewRepo(pool, cipher)
	var sandboxGW apisix.Gateway
	sandboxGatewayURL := ""
	if cfg.SandboxConfigured() {
		sandboxGW = apisix.NewClient(cfg.APISIXSandboxAdminURL, cfg.APISIXSandboxAdminKey)
		sandboxGatewayURL = cfg.APISIXSandboxGatewayURL
	}
	subSvc := subscriptions.NewService(subRepo, gw, sandboxGW, subscriptions.GenerateKey, eventRepo)
	subSvc.ConfigureOIDC(cfg.OIDCIssuer, cfg.OIDCClientIDClaim)
	subSvc.SetBiller(billing.NewService(billing.NewRepo(pool), billing.ManualProvider{}))
	if cfg.SMTPConfigured() {
		sender := notify.NewSMTPSender(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUsername, cfg.SMTPPassword, cfg.SMTPFrom)
		subSvc.SetNotifier(notify.NewNotifier(sender, notify.NewRepo(pool), cfg.PortalBaseURL))
	}
	owns := func(ctx context.Context, appID, userID int64) (bool, error) {
		return teamsRepo.IsMemberOfApp(ctx, userID, appID)
	}
	subH := subscriptions.NewHandler(subSvc, subRepo, eventRepo, owns, sandboxGatewayURL)
	subH.SetOIDCIssuer(cfg.OIDCIssuer)
	// Usage metrics are a read-only consumer of Prometheus; left unconfigured
	// (empty URL) the /usage endpoint reports unavailable rather than guessing.
	if cfg.PrometheusURL != "" {
		subH.SetUsageReader(metrics.NewService(metrics.NewClient(cfg.PrometheusURL)))
	}
	allowPrivate := os.Getenv("UPSTREAM_ALLOW_PRIVATE") == "1"
	adminSvc := admin.NewService(admin.NewRepo(pool), subSvc)
	adminH := admin.NewHandler(adminSvc, allowPrivate, cfg.OIDCConfigured())
	planAdminSvc := admin.NewPlanService(admin.NewPlanRepo(pool), subSvc)
	planAdminH := admin.NewPlanHandler(planAdminSvc)
	subAdminH := subscriptions.NewAdminHandler(subSvc)
	teamsH := teams.NewHandler(teamsRepo)

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
	mux.Handle("/api/teams", requireAuth(teamsH))
	mux.Handle("/api/teams/", requireAuth(teamsH))
	mux.Handle("/api/me/language", requireAuth(http.HandlerFunc(authH.PutLanguage)))
	tryProducts := tryitProductsAdapter{repo: catRepo}
	tryAccess := tryitAccessAdapter{teams: teamsRepo, subs: subRepo}
	tryH := tryit.NewHandler(tryProducts, tryAccess, cfg.APISIXGatewayURL, sandboxGatewayURL)
	ratingsH := ratings.NewHandler(
		ratings.NewRepo(pool),
		ratingsProductsAdapter{repo: catRepo},
		ratingsSubsAdapter{subs: subRepo},
		tok,
	)
	mux.Handle("/api/try/", requireAuth(tryH))
	mux.Handle("/api/ratings/", ratingsH)

	return i18n.Middleware(httpx.SecurityHeaders(httpx.MaxBodyBytes(1 << 20)(logRequests(mux))))
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
