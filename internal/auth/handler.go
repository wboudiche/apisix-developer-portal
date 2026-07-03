package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"apisix-portal/internal/httpx"
	"apisix-portal/internal/i18n"
)

// dummyHash is a valid bcrypt hash compared against when the user is absent, so
// login response time does not reveal whether an account exists (M3). Passwords
// over 72 bytes short-circuit in CheckPassword on BOTH login paths, so that
// class stays symmetric too.
const dummyHash = "$2a$12$kBCKU4PMSdprqnbX9uYhN.uuNofR4mwH3zF5a8xEADAFoRn2M2FMC"

// UserStore is the persistence surface the handler needs (satisfied by *Repo).
type UserStore interface {
	Create(ctx context.Context, email, passwordHash, name, lang string) (User, error)
	GetByEmail(ctx context.Context, email string) (User, string, error)
	SetLanguage(ctx context.Context, userID int64, lang string) error
}

type Handler struct {
	store        UserStore
	tk           *Tokenizer
	loginLimiter *httpx.RateLimiter
	router       chi.Router
}

// NewHandler creates an auth handler. loginLimiter is an optional per-account
// rate limiter applied to the login endpoint (nil = disabled).
func NewHandler(store UserStore, tk *Tokenizer, loginLimiter *httpx.RateLimiter) *Handler {
	h := &Handler{store: store, tk: tk, loginLimiter: loginLimiter, router: chi.NewRouter()}
	h.router.Post("/api/auth/register", h.register)
	h.router.Post("/api/auth/login", h.login)
	return h
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
	token, err := h.tk.Issue(u.ID, u.Email, u.Role)
	if err != nil {
		httpx.ErrorT(w, r, http.StatusInternalServerError, "auth.token.issueFailed")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"user": u, "token": token})
}
