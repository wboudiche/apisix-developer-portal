package applications

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"apisix-portal/internal/auth"
	"apisix-portal/internal/httpx"
)

type Store interface {
	Create(ctx context.Context, ownerID int64, name, description string) (Application, error)
	ListByOwner(ctx context.Context, ownerID int64) ([]Application, error)
}

type Handler struct {
	store  Store
	router chi.Router
}

func NewHandler(store Store) *Handler {
	h := &Handler{store: store, router: chi.NewRouter()}
	h.router.Post("/api/applications", h.create)
	h.router.Get("/api/applications", h.list)
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.router.ServeHTTP(w, r) }

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		httpx.Error(w, http.StatusBadRequest, "name is required")
		return
	}
	a, err := h.store.Create(r.Context(), auth.UserID(r.Context()), body.Name, body.Description)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to create application")
		return
	}
	httpx.JSON(w, http.StatusCreated, a)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	apps, err := h.store.ListByOwner(r.Context(), auth.UserID(r.Context()))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to list applications")
		return
	}
	if apps == nil {
		apps = []Application{}
	}
	httpx.JSON(w, http.StatusOK, apps)
}
