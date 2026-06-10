package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"apisix-portal/internal/httpx"
)

// dummyHash is a valid bcrypt hash compared against when the user is absent, so
// login response time does not reveal whether an account exists (M3).
var dummyHash = "$2a$12$kBCKU4PMSdprqnbX9uYhN.uuNofR4mwH3zF5a8xEADAFoRn2M2FMC"

// UserStore is the persistence surface the handler needs (satisfied by *Repo).
type UserStore interface {
	Create(ctx context.Context, email, passwordHash, name string) (User, error)
	GetByEmail(ctx context.Context, email string) (User, string, error)
}

type Handler struct {
	store  UserStore
	tk     *Tokenizer
	router chi.Router
}

func NewHandler(store UserStore, tk *Tokenizer) *Handler {
	h := &Handler{store: store, tk: tk, router: chi.NewRouter()}
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
		httpx.Error(w, http.StatusBadRequest, "email and password (min 8 chars) required")
		return
	}
	hash, err := HashPassword(c.Password)
	if errors.Is(err, ErrPasswordTooLong) {
		httpx.Error(w, http.StatusBadRequest, "password must be at most 72 bytes")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not hash password")
		return
	}
	u, err := h.store.Create(r.Context(), c.Email, hash, c.Name)
	if errors.Is(err, ErrEmailTaken) {
		// NOTE: a distinct 409 is an intentional UX trade-off (users must learn an
		// email is taken). The timing oracle on login is the closed leak (M3).
		httpx.Error(w, http.StatusConflict, "email already registered")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not create account")
		return
	}
	token, err := h.tk.Issue(u.ID, u.Email, u.Role)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not issue token")
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"user": u, "token": token})
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	u, hash, err := h.store.GetByEmail(r.Context(), c.Email)
	if err != nil {
		// Equalize timing: run a comparison against a dummy hash so an absent
		// account is indistinguishable from a wrong password.
		CheckPassword(dummyHash, c.Password)
		httpx.Error(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if !CheckPassword(hash, c.Password) {
		httpx.Error(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	token, err := h.tk.Issue(u.ID, u.Email, u.Role)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not issue token")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"user": u, "token": token})
}
