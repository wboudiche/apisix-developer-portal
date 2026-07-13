package server

import (
	"context"
	"log"
	"net/http"

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
	"apisix-portal/internal/settings"
	"apisix-portal/internal/subscriptions"
	"apisix-portal/internal/teams"
	"apisix-portal/internal/tryit"
)

// New builds the portal's HTTP handler: all API routes wired to the given
// database pool and config. It also seeds the admin role for cfg.AdminEmail
// (best-effort). Extracted from main so tests can mount the real app
// in-process via httptest.
//
// gw is accepted for backward compatibility with existing callers (main,
// the e2e harness) but is no longer used to provision the gateway: every
// APISIX-talking consumer instead goes through a SwappableGateway seeded from
// the runtime settings snapshot (see settingsSvc below), so admin-edited
// settings take effect without a process restart.
func New(ctx context.Context, pool *pgxpool.Pool, cfg config.Config, gw apisix.Gateway) http.Handler {
	tok := auth.NewTokenizer(cfg.JWTSecret)

	catRepo := catalog.NewRepo(pool)
	catalogH := catalog.NewHandler(catRepo)
	authRepo := auth.NewRepo(pool)

	// cipher and settingsSvc are constructed before anything that seeds from
	// config (the proxy-CIDR parse and the admin-role seed below) so that
	// DB-persisted overrides of TRUSTED_PROXIES / ADMIN_EMAIL apply at boot,
	// not just after the first settings change.
	cipher, err := crypto.New(cfg.CredentialEncKey)
	if err != nil {
		log.Fatalf("credential cipher: %v", err)
	}
	settingsSvc, err := settings.NewService(pool, cipher, cfg, settings.NewProber())
	if err != nil {
		log.Fatalf("settings: %v", err)
	}
	snap := settingsSvc.Snapshot()

	ipLimiter := httpx.NewRateLimiter(10, 0.5)    // per client IP, all /api/auth/ endpoints
	loginLimiter := httpx.NewRateLimiter(10, 0.5) // per account (email), login only
	// The snapshot value may carry a DB override, and a bad DB row must never
	// prevent boot (the settings UI has to stay reachable to fix it): on parse
	// error, log and fall back to the operator-controlled env value. Only the
	// env fallback failing is fatal — today's semantics for a bad env value.
	proxies, err := httpx.ParseProxyCIDRs(snap.Get("TRUSTED_PROXIES"))
	if err != nil {
		log.Printf("TRUSTED_PROXIES (settings): %v; falling back to env value", err)
		if proxies, err = httpx.ParseProxyCIDRs(cfg.TrustedProxies); err != nil {
			log.Fatalf("TRUSTED_PROXIES: %v", err)
		}
	}
	if proxies != nil {
		// Behind a reverse proxy, RemoteAddr is the proxy; honor its
		// X-Forwarded-For so per-IP limiting isolates real clients.
		ipLimiter.SetTrustedProxies(proxies)
	}
	authH := auth.NewHandler(authRepo, tok, loginLimiter)
	if err := authRepo.EnsureAdminRole(ctx, snap.Get("ADMIN_EMAIL")); err != nil {
		log.Printf("seed admin role (%s): %v", snap.Get("ADMIN_EMAIL"), err)
	}
	plansH := plans.NewHandler(plans.NewRepo(pool))
	eventRepo := events.NewRepo(pool)
	appsRepo := applications.NewRepo(pool)
	teamsRepo := teams.NewRepo(pool)
	appsH := applications.NewHandler(appsRepo, teamsRepo, eventRepo)

	newProd := func(e *settings.Effective) apisix.Gateway {
		return apisix.NewClient(e.Get("APISIX_ADMIN_URL"), e.Get("APISIX_ADMIN_KEY"))
	}
	newSandbox := func(e *settings.Effective) apisix.Gateway {
		if !e.SandboxConfigured() {
			return nil
		}
		return apisix.NewClient(e.Get("APISIX_SANDBOX_ADMIN_URL"), e.Get("APISIX_SANDBOX_ADMIN_KEY"))
	}
	prodGW := apisix.NewSwappable(newProd(snap))
	sandboxGW := apisix.NewSwappable(newSandbox(snap))
	// sandboxGatewayURL is "" whenever the sandbox is not configured (mirrors
	// Effective.SandboxConfigured, which requires both the admin AND gateway
	// URL) — shared by the subscriptions and try-it handlers, both of which
	// use an empty string to mean "sandbox unavailable".
	sandboxGatewayURL := func() string {
		e := settingsSvc.Snapshot()
		if !e.SandboxConfigured() {
			return ""
		}
		return e.Get("APISIX_SANDBOX_GATEWAY_URL")
	}

	subRepo := subscriptions.NewRepo(pool, cipher)
	subSvc := subscriptions.NewService(subRepo, prodGW, sandboxGW, subscriptions.GenerateKey, eventRepo)
	subSvc.SetSandboxEnabledFn(func() bool { return settingsSvc.Snapshot().SandboxConfigured() })
	subSvc.ConfigureOIDC(snap.Get("OIDC_ISSUER"), snap.Get("OIDC_CLIENT_ID_CLAIM"))
	billingSvc := billing.NewService(billing.NewRepo(pool), billing.ManualProvider{})
	subSvc.SetBiller(billingSvc)

	// SMTP is now always wired: DynamicSender no-ops with ErrSMTPNotConfigured
	// when the live snapshot has no host/from, and notify already logs and
	// drops send errors, so an unconfigured mail server stays a silent no-op
	// exactly as before — but a later settings change takes effect immediately.
	dynSender := notify.NewDynamicSender(settingsSvc)
	subSvc.SetNotifier(notify.NewNotifier(dynSender, notify.NewRepo(pool),
		func() string { return settingsSvc.Snapshot().Get("PORTAL_BASE_URL") }))
	// Verification resends are limited to 3 quick tries then 1/min per email
	// address. Enablement is decided per request from the live snapshot, so
	// flipping REQUIRE_EMAIL_VERIFICATION takes effect without a restart.
	authH.EnableDynamicVerification(verificationFromSettings{settingsSvc}, dynSender, httpx.NewRateLimiter(3, 1.0/60))

	owns := func(ctx context.Context, appID, userID int64) (bool, error) {
		return teamsRepo.IsMemberOfApp(ctx, userID, appID)
	}
	subH := subscriptions.NewHandler(subSvc, subRepo, eventRepo, owns, sandboxGatewayURL)
	subH.SetOIDCIssuer(snap.Get("OIDC_ISSUER"))
	// Usage metrics are a read-only consumer of Prometheus; left unconfigured
	// (empty URL) the /usage endpoint reports unavailable rather than guessing.
	if pu := snap.Get("PROMETHEUS_URL"); pu != "" {
		subH.SetUsageReader(metrics.NewService(metrics.NewClient(pu)))
	}
	adminSvc := admin.NewService(admin.NewRepo(pool), subSvc)
	adminH := admin.NewHandler(adminSvc,
		func() bool { return settingsSvc.Snapshot().Bool("UPSTREAM_ALLOW_PRIVATE") },
		func() bool { return settingsSvc.Snapshot().Get("OIDC_ISSUER") != "" },
		func() bool { return settingsSvc.Snapshot().SandboxConfigured() },
	)
	planAdminSvc := admin.NewPlanService(admin.NewPlanRepo(pool), subSvc)
	planAdminH := admin.NewPlanHandler(planAdminSvc)
	subAdminH := subscriptions.NewAdminHandler(subSvc)
	teamsH := teams.NewHandler(teamsRepo)
	billingTeamH := billing.NewTeamHandler(billingSvc)
	billingAdminH := billing.NewAdminHandler(billingSvc)
	settingsH := settings.NewHandler(settingsSvc, auth.UserID)

	// Re-arm every setting-derived binding whenever the admin settings API
	// commits a change. Hooks run under the settings service's writer lock
	// (Task 4): they must never call back into settingsSvc.Set/Reset/OnChange
	// — only Snapshot() reads and side-effect-free swaps below.
	prevAdminEmail := snap.Get("ADMIN_EMAIL")
	settingsSvc.OnChange(func(e *settings.Effective) {
		prodGW.Swap(newProd(e))
		sandboxGW.Swap(newSandbox(e))
		subSvc.ConfigureOIDC(e.Get("OIDC_ISSUER"), e.Get("OIDC_CLIENT_ID_CLAIM"))
		subH.SetOIDCIssuer(e.Get("OIDC_ISSUER"))
		if pu := e.Get("PROMETHEUS_URL"); pu != "" {
			subH.SetUsageReader(metrics.NewService(metrics.NewClient(pu)))
		}
		if proxies, err := httpx.ParseProxyCIDRs(e.Get("TRUSTED_PROXIES")); err != nil {
			log.Printf("settings: TRUSTED_PROXIES invalid, keeping previous: %v", err)
		} else {
			ipLimiter.SetTrustedProxies(proxies)
		}
		if ae := e.Get("ADMIN_EMAIL"); ae != prevAdminEmail {
			prevAdminEmail = ae
			if err := authRepo.EnsureAdminRole(context.Background(), ae); err != nil {
				log.Printf("settings: promote %q: %v", ae, err)
			}
		}
	})

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
	mux.Handle("/api/admin/meta", requireAdmin(adminH))
	mux.Handle("/api/admin/products", requireAdmin(adminH))
	mux.Handle("/api/admin/products/", requireAdmin(adminH))
	mux.Handle("/api/admin/plans", requireAdmin(planAdminH))
	mux.Handle("/api/admin/plans/", requireAdmin(planAdminH))
	mux.Handle("/api/admin/subscriptions", requireAdmin(subAdminH))
	mux.Handle("/api/admin/subscriptions/", requireAdmin(subAdminH))
	mux.Handle("/api/admin/settings", requireAdmin(settingsH))
	mux.Handle("/api/admin/settings/", requireAdmin(settingsH))
	mux.Handle("/api/teams", requireAuth(teamsH))
	mux.Handle("/api/teams/", requireAuth(teamsH))
	mux.Handle("/api/billing/invoices", requireAuth(billingTeamH))
	mux.Handle("/api/admin/invoices", requireAdmin(billingAdminH))
	mux.Handle("/api/admin/invoices/", requireAdmin(billingAdminH))
	mux.Handle("/api/me/language", requireAuth(http.HandlerFunc(authH.PutLanguage)))
	tryProducts := tryitProductsAdapter{repo: catRepo}
	tryAccess := tryitAccessAdapter{teams: teamsRepo, subs: subRepo}
	tryH := tryit.NewHandler(tryProducts, tryAccess,
		func() string { return settingsSvc.Snapshot().Get("APISIX_GATEWAY_URL") },
		sandboxGatewayURL,
	)
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

// verificationFromSettings adapts *settings.Service to auth.VerificationProvider
// so the email-verification gate reads REQUIRE_EMAIL_VERIFICATION and
// PORTAL_BASE_URL from the live snapshot on every request.
type verificationFromSettings struct{ svc *settings.Service }

func (v verificationFromSettings) VerificationEnabled() bool {
	return v.svc.Snapshot().Bool("REQUIRE_EMAIL_VERIFICATION")
}
func (v verificationFromSettings) VerificationBaseURL() string {
	return v.svc.Snapshot().Get("PORTAL_BASE_URL")
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
