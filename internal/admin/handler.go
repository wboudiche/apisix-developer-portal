package admin

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"apisix-portal/internal/httpx"
	"apisix-portal/internal/paging"
)

// ProductService is the surface the handler needs (satisfied by *Service).
type ProductService interface {
	List(ctx context.Context, p paging.Params) ([]Product, int, error)
	Get(ctx context.Context, id int64) (Product, error)
	Create(ctx context.Context, p Product) (Product, error)
	Update(ctx context.Context, p Product) (Product, error)
	Delete(ctx context.Context, id int64) error
	AddChangelog(ctx context.Context, productID int64, e ChangelogEntry) (ChangelogEntry, error)
	ListChangelog(ctx context.Context, productID int64) ([]ChangelogEntry, error)
	DeleteChangelog(ctx context.Context, productID, entryID int64) error
}

type Handler struct {
	svc            ProductService
	router         chi.Router
	allowPrivate   bool
	oidcConfigured bool
}

func NewHandler(svc ProductService, allowPrivate bool, oidcConfigured bool) *Handler {
	h := &Handler{svc: svc, router: chi.NewRouter(), allowPrivate: allowPrivate, oidcConfigured: oidcConfigured}
	h.router.Get("/api/admin/products", h.list)
	h.router.Post("/api/admin/products", h.create)
	h.router.Post("/api/admin/products/import", h.importSpec)
	h.router.Get("/api/admin/products/{id}", h.get)
	h.router.Put("/api/admin/products/{id}", h.update)
	h.router.Delete("/api/admin/products/{id}", h.delete)
	h.router.Post("/api/admin/products/{id}/changelog", h.addChangelog)
	h.router.Get("/api/admin/products/{id}/changelog", h.listChangelog)
	h.router.Delete("/api/admin/products/{id}/changelog/{entryId}", h.deleteChangelog)
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.router.ServeHTTP(w, r) }

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	p := paging.Parse(r.URL.Query())
	items, total, err := h.svc.List(r.Context(), p)
	if err != nil {
		log.Printf("admin list products: %v", err)
		httpx.Error(w, http.StatusInternalServerError, "failed to list products")
		return
	}
	httpx.JSON(w, http.StatusOK, paging.New(items, total, p))
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
	p, ok := h.decodeProduct(w, r)
	if !ok {
		return
	}
	created, err := h.svc.Create(r.Context(), p)
	if errors.Is(err, ErrSlugTaken) {
		httpx.Error(w, http.StatusConflict, "slug already exists")
		return
	}
	if errors.Is(err, ErrContextPathTaken) {
		httpx.Error(w, http.StatusConflict, "contextPath conflicts with an existing product")
		return
	}
	if err != nil {
		log.Printf("admin create product: %v", err)
		httpx.Error(w, http.StatusInternalServerError, "failed to create product")
		return
	}
	httpx.JSON(w, http.StatusCreated, created)
}

// importSpec parses an OpenAPI/Swagger spec (from a pasted body or a fetched
// URL) into a draft product and returns it WITHOUT persisting. The admin then
// reviews it in the form and POSTs it to /api/admin/products as usual.
func (h *Handler) importSpec(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Spec string `json:"spec"`
		URL  string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	body.Spec = strings.TrimSpace(body.Spec)
	body.URL = strings.TrimSpace(body.URL)
	if (body.Spec == "") == (body.URL == "") {
		httpx.Error(w, http.StatusBadRequest, "provide exactly one of spec or url")
		return
	}

	data := []byte(body.Spec)
	if body.URL != "" {
		fetched, err := fetchSpec(r.Context(), body.URL, h.allowPrivate)
		if err != nil {
			httpx.Error(w, http.StatusUnprocessableEntity, "could not fetch spec from url")
			return
		}
		data = fetched
	}

	draft, err := parseSpec(data)
	if err != nil {
		httpx.Error(w, http.StatusUnprocessableEntity, "spec could not be parsed (need OpenAPI 3.x or Swagger 2.0 with a title)")
		return
	}
	draft.OpenAPISpec = string(data)
	httpx.JSON(w, http.StatusOK, draft)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	p, ok := h.decodeProduct(w, r)
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
	if errors.Is(err, ErrContextPathTaken) {
		httpx.Error(w, http.StatusConflict, "contextPath conflicts with an existing product")
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

// validChangelogKinds mirrors the changelog_entries.kind CHECK constraint.
var validChangelogKinds = map[string]bool{
	"added": true, "changed": true, "fixed": true,
	"removed": true, "deprecated": true, "security": true,
}

func (h *Handler) addChangelog(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var e ChangelogEntry
	if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	if strings.TrimSpace(e.Version) == "" {
		httpx.Error(w, http.StatusBadRequest, "version is required")
		return
	}
	if !validChangelogKinds[e.Kind] {
		httpx.Error(w, http.StatusBadRequest, "kind must be one of added, changed, fixed, removed, deprecated, security")
		return
	}
	if _, err := time.Parse("2006-01-02", e.Date); err != nil {
		httpx.Error(w, http.StatusBadRequest, "date must be a valid YYYY-MM-DD date")
		return
	}
	created, err := h.svc.AddChangelog(r.Context(), id, e)
	if err != nil {
		log.Printf("admin add changelog for product %d: %v", id, err)
		httpx.Error(w, http.StatusInternalServerError, "failed to add changelog entry")
		return
	}
	httpx.JSON(w, http.StatusCreated, created)
}

// listChangelog returns all changelog entries for a product, including
// drafts/unpublished — unlike the public GET /api/products/{slug}/changelog,
// which is published-only.
func (h *Handler) listChangelog(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	entries, err := h.svc.ListChangelog(r.Context(), id)
	if err != nil {
		log.Printf("admin list changelog for product %d: %v", id, err)
		httpx.Error(w, http.StatusInternalServerError, "failed to list changelog entries")
		return
	}
	if entries == nil {
		entries = []ChangelogEntry{}
	}
	httpx.JSON(w, http.StatusOK, entries)
}

func (h *Handler) deleteChangelog(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	entryID, err := strconv.ParseInt(chi.URLParam(r, "entryId"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "bad changelog entry id")
		return
	}
	if err := h.svc.DeleteChangelog(r.Context(), id, entryID); err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "changelog entry not found")
			return
		}
		log.Printf("admin delete changelog entry %d for product %d: %v", entryID, id, err)
		httpx.Error(w, http.StatusInternalServerError, "failed to delete changelog entry")
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

func (h *Handler) decodeProduct(w http.ResponseWriter, r *http.Request) (Product, bool) {
	var p Product
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid body")
		return Product{}, false
	}
	if p.Tags == nil {
		p.Tags = []string{}
	}
	if msg := p.validate(h.allowPrivate); msg != "" {
		httpx.Error(w, http.StatusBadRequest, msg)
		return Product{}, false
	}
	if p.AuthType == "oauth2" && !h.oidcConfigured {
		httpx.Error(w, http.StatusBadRequest, "OAuth2 is not configured on this portal")
		return Product{}, false
	}
	if p.OpenAPISpec != "" {
		if _, err := parseSpec([]byte(p.OpenAPISpec)); err != nil {
			httpx.Error(w, http.StatusBadRequest, "openapiSpec is not a valid OpenAPI 3.x / Swagger 2.0 document")
			return Product{}, false
		}
	}
	return p, true
}
