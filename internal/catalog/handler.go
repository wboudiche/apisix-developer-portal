package catalog

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"apisix-portal/internal/httpx"
	"apisix-portal/internal/paging"
)

type contextStub = context.Context

// Lister is the read surface the handler needs (satisfied by *Repo).
type Lister interface {
	List(ctx context.Context, q Query, p paging.Params) ([]Product, int, error)
	GetBySlug(ctx context.Context, slug string) (Product, error)
}

type Handler struct {
	repo   Lister
	router chi.Router
}

func NewHandler(repo Lister) *Handler {
	h := &Handler{repo: repo, router: chi.NewRouter()}
	h.router.Get("/api/products", h.list)
	h.router.Get("/api/products/{slug}", h.getBySlug)
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.router.ServeHTTP(w, r) }

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	q := Query{
		Search:   r.URL.Query().Get("search"),
		Category: r.URL.Query().Get("category"),
		Tag:      r.URL.Query().Get("tag"),
		Sort:     r.URL.Query().Get("sort"),
	}
	p := paging.Parse(r.URL.Query())
	items, total, err := h.repo.List(r.Context(), q, p)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to list products")
		return
	}
	httpx.JSON(w, http.StatusOK, paging.New(items, total, p))
}

func (h *Handler) getBySlug(w http.ResponseWriter, r *http.Request) {
	p, err := h.repo.GetBySlug(r.Context(), chi.URLParam(r, "slug"))
	if err == ErrNotFound {
		httpx.Error(w, http.StatusNotFound, "product not found")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to load product")
		return
	}
	httpx.JSON(w, http.StatusOK, p)
}
