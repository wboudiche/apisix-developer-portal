package admin

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"apisix-portal/internal/httpx"
)

// ProductService is the surface the handler needs (satisfied by *Service).
type ProductService interface {
	List(ctx context.Context) ([]Product, error)
	Get(ctx context.Context, id int64) (Product, error)
	Create(ctx context.Context, p Product) (Product, error)
	Update(ctx context.Context, p Product) (Product, error)
	Delete(ctx context.Context, id int64) error
}

type Handler struct {
	svc    ProductService
	router chi.Router
}

func NewHandler(svc ProductService) *Handler {
	h := &Handler{svc: svc, router: chi.NewRouter()}
	h.router.Get("/api/admin/products", h.list)
	h.router.Post("/api/admin/products", h.create)
	h.router.Get("/api/admin/products/{id}", h.get)
	h.router.Put("/api/admin/products/{id}", h.update)
	h.router.Delete("/api/admin/products/{id}", h.delete)
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.router.ServeHTTP(w, r) }

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List(r.Context())
	if err != nil {
		log.Printf("admin list products: %v", err)
		httpx.Error(w, http.StatusInternalServerError, "failed to list products")
		return
	}
	if items == nil {
		items = []Product{}
	}
	httpx.JSON(w, http.StatusOK, items)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	p, err := h.svc.Get(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "product not found")
		return
	}
	if err != nil {
		log.Printf("admin get product %d: %v", id, err)
		httpx.Error(w, http.StatusInternalServerError, "failed to load product")
		return
	}
	httpx.JSON(w, http.StatusOK, p)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	p, ok := decodeProduct(w, r)
	if !ok {
		return
	}
	created, err := h.svc.Create(r.Context(), p)
	if errors.Is(err, ErrSlugTaken) {
		httpx.Error(w, http.StatusConflict, "slug already exists")
		return
	}
	if err != nil {
		log.Printf("admin create product: %v", err)
		httpx.Error(w, http.StatusInternalServerError, "failed to create product")
		return
	}
	httpx.JSON(w, http.StatusCreated, created)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	p, ok := decodeProduct(w, r)
	if !ok {
		return
	}
	p.ID = id
	updated, err := h.svc.Update(r.Context(), p)
	if errors.Is(err, ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "product not found")
		return
	}
	if errors.Is(err, ErrSlugTaken) {
		httpx.Error(w, http.StatusConflict, "slug already exists")
		return
	}
	if err != nil {
		log.Printf("admin update product %d: %v", id, err)
		httpx.Error(w, http.StatusInternalServerError, "failed to update product")
		return
	}
	httpx.JSON(w, http.StatusOK, updated)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	err := h.svc.Delete(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "product not found")
		return
	}
	if errors.Is(err, ErrHasSubscriptions) {
		httpx.Error(w, http.StatusConflict, "product has active subscriptions")
		return
	}
	if err != nil {
		log.Printf("admin delete product %d: %v", id, err)
		httpx.Error(w, http.StatusInternalServerError, "failed to delete product")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "bad product id")
		return 0, false
	}
	return id, true
}

func decodeProduct(w http.ResponseWriter, r *http.Request) (Product, bool) {
	var p Product
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid body")
		return Product{}, false
	}
	if p.Tags == nil {
		p.Tags = []string{}
	}
	if msg := p.validate(); msg != "" {
		httpx.Error(w, http.StatusBadRequest, msg)
		return Product{}, false
	}
	return p, true
}
