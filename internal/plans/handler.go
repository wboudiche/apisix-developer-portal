package plans

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"apisix-portal/internal/httpx"
)

type Lister interface {
	List(ctx context.Context) ([]Plan, error)
}

type Handler struct {
	repo   Lister
	router chi.Router
}

func NewHandler(repo Lister) *Handler {
	h := &Handler{repo: repo, router: chi.NewRouter()}
	h.router.Get("/api/plans", h.list)
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.router.ServeHTTP(w, r) }

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	items, err := h.repo.List(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to list plans")
		return
	}
	if items == nil {
		items = []Plan{}
	}
	httpx.JSON(w, http.StatusOK, items)
}
