package plans

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"apisix-portal/internal/httpx"
	"apisix-portal/internal/paging"
)

type Lister interface {
	List(ctx context.Context, p paging.Params) ([]Plan, int, error)
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
	p := paging.Parse(r.URL.Query())
	items, total, err := h.repo.List(r.Context(), p)
	if err != nil {
		httpx.ErrorT(w, r, http.StatusInternalServerError, "admin.plan.listFailed")
		return
	}
	httpx.JSON(w, http.StatusOK, paging.New(items, total, p))
}
