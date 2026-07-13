package auth

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"apisix-portal/internal/httpx"
	"apisix-portal/internal/i18n"
	"apisix-portal/internal/notify"
)

// dummyHash is a valid bcrypt hash compared against when the user is absent, so
// login response time does not reveal whether an account exists (M3). Passwords
// over 72 bytes short-circuit in CheckPassword on BOTH login paths, so that
// class stays symmetric too.
const dummyHash = "$2a$12$kBCKU4PMSdprqnbX9uYhN.uuNofR4mwH3zF5a8xEADAFoRn2M2FMC"

// UserStore is the persistence surface the handler needs (satisfied by *Repo).
type UserStore interface {
	Create(ctx context.Context, email, passwordHash, name, lang string) (User, error)
	CreateUnverified(ctx context.Context, email, passwordHash, name, lang, verifyTokenHash string, expiresAt time.Time) (User, error)
	GetByEmail(ctx context.Context, email string) (User, string, error)
	SetLanguage(ctx context.Context, userID int64, lang string) error
	VerifyByTokenHash(ctx context.Context, tokenHash string) error
	ResetVerifyToken(ctx context.Context, email, tokenHash string, expiresAt time.Time) (User, error)
}

// VerificationConfig wires the opt-in email-verification gate (spec
// 2026-07-11). Zero-value fields get safe defaults in EnableEmailVerification.
type VerificationConfig struct {
	Sender   notify.Sender
	BaseURL  string
	Limiter  *httpx.RateLimiter
	TokenTTL time.Duration
	GenToken func() (plain, hash string)
}

// VerificationProvider reports, per request, whether the email-verification
// gate is currently on and where the verification link should point.
// Enabled and BaseURL are resolved at the same moment for consistency (a
// single snapshot read), and Enabled is consulted fresh on every request so
// a dynamic provider can flip the gate without re-registering routes.
type VerificationProvider interface {
	VerificationEnabled() bool
	VerificationBaseURL() string
}

// staticProvider adapts the legacy VerificationConfig one-shot enablement:
// always on, with a fixed BaseURL.
type staticProvider struct{ baseURL string }

func (p staticProvider) VerificationEnabled() bool   { return true }
func (p staticProvider) VerificationBaseURL() string { return p.baseURL }

type Handler struct {
	store        UserStore
	tk           *Tokenizer
	loginLimiter *httpx.RateLimiter
	router       chi.Router

	verifyProv    VerificationProvider // nil = feature entirely absent
	verifySender  notify.Sender
	verifyLimiter *httpx.RateLimiter
	verifyTTL     time.Duration
	verifyGen     func() (plain, hash string)
}

// NewHandler creates an auth handler. loginLimiter is an optional per-account
// rate limiter applied to the login endpoint (nil = disabled).
func NewHandler(store UserStore, tk *Tokenizer, loginLimiter *httpx.RateLimiter) *Handler {
	h := &Handler{store: store, tk: tk, loginLimiter: loginLimiter, router: chi.NewRouter()}
	h.router.Post("/api/auth/register", h.register)
	h.router.Post("/api/auth/login", h.login)
	return h
}

// EnableEmailVerification switches the handler into verified-only mode:
// register withholds the JWT and emails a link, login refuses unverified
// accounts, and the verify/resend endpoints are mounted.
func (h *Handler) EnableEmailVerification(vc VerificationConfig) {
	if vc.TokenTTL == 0 {
		vc.TokenTTL = 24 * time.Hour
	}
	if vc.GenToken == nil {
		vc.GenToken = GenerateVerifyToken
	}
	h.verifySender, h.verifyLimiter, h.verifyTTL, h.verifyGen = vc.Sender, vc.Limiter, vc.TokenTTL, vc.GenToken
	h.verifyProv = staticProvider{baseURL: vc.BaseURL}
	h.mountVerifyRoutes()
}

// EnableDynamicVerification switches the handler into verified-only mode
// whose enablement is decided per request by p (spec: runtime settings can
// flip REQUIRE_EMAIL_VERIFICATION without restarting the process). Unlike
// EnableEmailVerification's fixed TTL/token generator, this path always uses
// the 24h default TTL and GenerateVerifyToken.
func (h *Handler) EnableDynamicVerification(p VerificationProvider, sender notify.Sender, limiter *httpx.RateLimiter) {
	h.verifySender, h.verifyLimiter, h.verifyTTL, h.verifyGen = sender, limiter, 24*time.Hour, GenerateVerifyToken
	h.verifyProv = p
	h.mountVerifyRoutes()
}

// mountVerifyRoutes registers the verify/resend routes. Callers must invoke
// only one of EnableEmailVerification/EnableDynamicVerification per handler —
// chi panics on duplicate route registration.
func (h *Handler) mountVerifyRoutes() {
	h.router.Post("/api/auth/verify", h.verifyEmail)
	h.router.Post("/api/auth/resend-verification", h.resendVerification)
}

// verificationOn reports whether the email-verification gate is active for
// this request. nil provider means the feature was never enabled.
func (h *Handler) verificationOn() bool {
	return h.verifyProv != nil && h.verifyProv.VerificationEnabled()
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.router.ServeHTTP(w, r) }

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil || c.Email == "" || len(c.Password) < 8 {
		httpx.ErrorT(w, r, http.StatusBadRequest, "auth.register.credentialsRequired")
		return
	}
	hash, err := HashPassword(c.Password)
	if errors.Is(err, ErrPasswordTooLong) {
		httpx.ErrorT(w, r, http.StatusBadRequest, "auth.password.tooLong")
		return
	}
	if err != nil {
		httpx.ErrorT(w, r, http.StatusInternalServerError, "auth.password.hashFailed")
		return
	}
	lang := string(i18n.FromContext(r.Context()))
	if h.verificationOn() {
		baseURL := h.verifyProv.VerificationBaseURL()
		plain, tokenHash := h.verifyGen()
		u, err := h.store.CreateUnverified(r.Context(), c.Email, hash, c.Name, lang, tokenHash, time.Now().Add(h.verifyTTL))
		if errors.Is(err, ErrEmailTaken) {
			httpx.ErrorT(w, r, http.StatusConflict, "auth.register.emailTaken")
			return
		}
		if err != nil {
			httpx.ErrorT(w, r, http.StatusInternalServerError, "auth.register.createFailed")
			return
		}
		h.sendVerification(u, lang, plain, baseURL)
		httpx.JSON(w, http.StatusCreated, map[string]any{"user": u, "verificationRequired": true})
		return
	}
	u, err := h.store.Create(r.Context(), c.Email, hash, c.Name, lang)
	if errors.Is(err, ErrEmailTaken) {
		// NOTE: a distinct 409 is an intentional UX trade-off (users must learn an
		// email is taken). The timing oracle on login is the closed leak (M3).
		httpx.ErrorT(w, r, http.StatusConflict, "auth.register.emailTaken")
		return
	}
	if err != nil {
		httpx.ErrorT(w, r, http.StatusInternalServerError, "auth.register.createFailed")
		return
	}
	token, err := h.tk.Issue(u.ID, u.Email, u.Role)
	if err != nil {
		httpx.ErrorT(w, r, http.StatusInternalServerError, "auth.token.issueFailed")
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"user": u, "token": token})
}

// sendVerification emails the verification link asynchronously, best-effort:
// failures are logged only — the resend endpoint is the recovery path (spec:
// best-effort like notify). Async so resend answers 204 after DB work alone
// whether or not a mail goes out (no account-existence timing oracle) and
// register never blocks on SMTP. baseURL is resolved by the caller BEFORE
// spawning this goroutine, so a dynamic provider's value at request time is
// what gets used, not whatever it might return later.
func (h *Handler) sendVerification(u User, lang, plainToken, baseURL string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		link := baseURL + "/verify-email?token=" + plainToken
		if err := notify.SendVerificationEmail(ctx, h.verifySender, lang, u.Email, u.Name, link); err != nil {
			log.Printf("auth: verification email to %q: %v", u.Email, err)
		}
	}()
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		httpx.ErrorT(w, r, http.StatusBadRequest, "common.invalidBody")
		return
	}
	if h.loginLimiter != nil && !h.loginLimiter.Allow(strings.ToLower(c.Email)) {
		if ra := h.loginLimiter.RetryAfter(); ra != "" {
			w.Header().Set("Retry-After", ra)
		}
		httpx.ErrorT(w, r, http.StatusTooManyRequests, "auth.login.tooManyAttempts")
		return
	}
	u, hash, err := h.store.GetByEmail(r.Context(), c.Email)
	if err != nil {
		// Equalize timing: run a comparison against a dummy hash so an absent
		// account is indistinguishable from a wrong password.
		CheckPassword(dummyHash, c.Password)
		httpx.ErrorT(w, r, http.StatusUnauthorized, "auth.login.invalidCredentials")
		return
	}
	if !CheckPassword(hash, c.Password) {
		httpx.ErrorT(w, r, http.StatusUnauthorized, "auth.login.invalidCredentials")
		return
	}
	if h.verificationOn() && !u.Verified {
		httpx.ErrorT(w, r, http.StatusForbidden, "auth.login.emailNotVerified")
		return
	}
	token, err := h.tk.Issue(u.ID, u.Email, u.Role)
	if err != nil {
		httpx.ErrorT(w, r, http.StatusInternalServerError, "auth.token.issueFailed")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"user": u, "token": token})
}

type languagePref struct {
	Language string `json:"language"`
}

// PutLanguage persists the authenticated user's UI language preference. Mounted
// at PUT /api/me/language behind RequireAuth.
func (h *Handler) PutLanguage(w http.ResponseWriter, r *http.Request) {
	uid := UserID(r.Context())
	if uid == 0 {
		httpx.ErrorT(w, r, http.StatusUnauthorized, "auth.middleware.missingToken")
		return
	}
	var p languagePref
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil || (p.Language != "fr" && p.Language != "en") {
		httpx.ErrorT(w, r, http.StatusBadRequest, "common.invalidBody")
		return
	}
	if err := h.store.SetLanguage(r.Context(), uid, p.Language); err != nil {
		httpx.ErrorT(w, r, http.StatusInternalServerError, "auth.register.createFailed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) verifyEmail(w http.ResponseWriter, r *http.Request) {
	if !h.verificationOn() {
		httpx.ErrorT(w, r, http.StatusNotFound, "common.notFound")
		return
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Token == "" {
		httpx.ErrorT(w, r, http.StatusBadRequest, "common.invalidBody")
		return
	}
	switch err := h.store.VerifyByTokenHash(r.Context(), HashVerifyToken(body.Token)); {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, ErrTokenInvalid):
		httpx.ErrorT(w, r, http.StatusGone, "auth.verify.invalidOrExpired")
	default:
		httpx.ErrorT(w, r, http.StatusInternalServerError, "auth.verify.failed")
	}
}

// resendVerification always answers 204 so responses never disclose whether
// an account exists or is verified (same discipline as the login timing
// equalization, M3).
func (h *Handler) resendVerification(w http.ResponseWriter, r *http.Request) {
	if !h.verificationOn() {
		httpx.ErrorT(w, r, http.StatusNotFound, "common.notFound")
		return
	}
	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Email == "" {
		httpx.ErrorT(w, r, http.StatusBadRequest, "common.invalidBody")
		return
	}
	if h.verifyLimiter != nil && !h.verifyLimiter.Allow(strings.ToLower(body.Email)) {
		if ra := h.verifyLimiter.RetryAfter(); ra != "" {
			w.Header().Set("Retry-After", ra)
		}
		httpx.ErrorT(w, r, http.StatusTooManyRequests, "auth.login.tooManyAttempts")
		return
	}
	baseURL := h.verifyProv.VerificationBaseURL()
	plain, tokenHash := h.verifyGen()
	u, err := h.store.ResetVerifyToken(r.Context(), body.Email, tokenHash, time.Now().Add(h.verifyTTL))
	if err == nil {
		lang := u.Language
		if lang == "" {
			lang = string(i18n.FromContext(r.Context()))
		}
		h.sendVerification(u, lang, plain, baseURL)
	} else if !errors.Is(err, ErrUserNotFound) && !errors.Is(err, ErrAlreadyVerified) {
		log.Printf("auth: resend verification for %q: %v", body.Email, err)
	}
	w.WriteHeader(http.StatusNoContent)
}
