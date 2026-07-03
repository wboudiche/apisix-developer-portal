package catalog

import (
	"context"
	"errors"
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
	GetSpecBySlug(ctx context.Context, slug string) (string, error)
	ListChangelogBySlug(ctx context.Context, slug string) ([]ChangelogEntry, error)
}

type Handler struct {
	repo   Lister
	router chi.Router
}

func NewHandler(repo Lister) *Handler {
	h := &Handler{repo: repo, router: chi.NewRouter()}
	h.router.Get("/api/products", h.list)
	h.router.Get("/api/products/{slug}", h.getBySlug)
	h.router.Get("/api/products/{slug}/spec", h.getSpec)
	h.router.Get("/api/products/{slug}/changelog", h.getChangelog)
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
		httpx.ErrorT(w, r, http.StatusInternalServerError, "catalog.list.failed")
		return
	}
	httpx.JSON(w, http.StatusOK, paging.New(items, total, p))
}

func (h *Handler) getBySlug(w http.ResponseWriter, r *http.Request) {
	p, err := h.repo.GetBySlug(r.Context(), chi.URLParam(r, "slug"))
	if err == ErrNotFound {
		httpx.ErrorT(w, r, http.StatusNotFound, "catalog.productNotFound")
		return
	}
	if err != nil {
		httpx.ErrorT(w, r, http.StatusInternalServerError, "catalog.get.failed")
		return
	}
	httpx.JSON(w, http.StatusOK, p)
}

func (h *Handler) getSpec(w http.ResponseWriter, r *http.Request) {
	spec, err := h.repo.GetSpecBySlug(r.Context(), chi.URLParam(r, "slug"))
	if err == ErrNotFound {
		httpx.ErrorT(w, r, http.StatusNotFound, "catalog.specNotFound")
		return
	}
	if err != nil {
		httpx.ErrorT(w, r, http.StatusInternalServerError, "catalog.spec.failed")
		return
	}
	w.Header().Set("Content-Type", specContentType(spec))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(spec))
}

func (h *Handler) getChangelog(w http.ResponseWriter, r *http.Request) {
	entries, err := h.repo.ListChangelogBySlug(r.Context(), chi.URLParam(r, "slug"))
	if errors.Is(err, ErrNotFound) {
		httpx.ErrorT(w, r, http.StatusNotFound, "catalog.productNotFound")
		return
	}
	if err != nil {
		httpx.ErrorT(w, r, http.StatusInternalServerError, "catalog.changelog.failed")
		return
	}
	if entries == nil {
		entries = []ChangelogEntry{}
	}
	httpx.JSON(w, http.StatusOK, entries)
}

// specContentType guesses JSON vs YAML from the first non-space byte.
func specContentType(s string) string {
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			continue
		}
		if r == '{' || r == '[' {
			return "application/json"
		}
		break
	}
	return "application/yaml"
}
