package admin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
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
	SetUploadedIcon(ctx context.Context, productID int64, png []byte) (time.Time, error)
	GetIcon(ctx context.Context, productID int64) ([]byte, time.Time, error)
}

type Handler struct {
	svc               ProductService
	router            chi.Router
	allowPrivate      func() bool // dynamic: reads the live settings snapshot
	oidcConfigured    func() bool
	sandboxConfigured func() bool
}

func NewHandler(svc ProductService, allowPrivate func() bool, oidcConfigured func() bool, sandboxConfigured func() bool) *Handler {
	h := &Handler{svc: svc, router: chi.NewRouter(), allowPrivate: allowPrivate, oidcConfigured: oidcConfigured, sandboxConfigured: sandboxConfigured}
	h.router.Get("/api/admin/meta", h.meta)
	h.router.Get("/api/admin/products", h.list)
	h.router.Post("/api/admin/products", h.create)
	h.router.Post("/api/admin/products/import", h.importSpec)
	h.router.Get("/api/admin/products/{id}", h.get)
	h.router.Put("/api/admin/products/{id}", h.update)
	h.router.Delete("/api/admin/products/{id}", h.delete)
	h.router.Post("/api/admin/products/{id}/changelog", h.addChangelog)
	h.router.Get("/api/admin/products/{id}/changelog", h.listChangelog)
	h.router.Delete("/api/admin/products/{id}/changelog/{entryId}", h.deleteChangelog)
	h.router.Post("/api/admin/products/{id}/icon", h.uploadIcon)
	h.router.Get("/api/admin/products/{id}/icon", h.serveIcon)
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.router.ServeHTTP(w, r) }

// meta reports deployment capabilities so the admin UI can hide features that
// are not wired up server-side (e.g. no sandbox gateway configured).
func (h *Handler) meta(w http.ResponseWriter, _ *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]bool{
		"sandboxConfigured": h.sandboxConfigured(),
		"oidcConfigured":    h.oidcConfigured(),
	})
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	p := paging.Parse(r.URL.Query())
	items, total, err := h.svc.List(r.Context(), p)
	if err != nil {
		log.Printf("admin list products: %v", err)
		httpx.ErrorT(w, r, http.StatusInternalServerError, "catalog.list.failed")
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
		httpx.ErrorT(w, r, http.StatusNotFound, "catalog.productNotFound")
		return
	}
	if err != nil {
		log.Printf("admin get product %d: %v", id, err)
		httpx.ErrorT(w, r, http.StatusInternalServerError, "catalog.get.failed")
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
		httpx.ErrorT(w, r, http.StatusConflict, "admin.product.slugTaken")
		return
	}
	if errors.Is(err, ErrContextPathTaken) {
		httpx.ErrorT(w, r, http.StatusConflict, "admin.product.contextPathTaken")
		return
	}
	if err != nil {
		log.Printf("admin create product: %v", err)
		httpx.ErrorT(w, r, http.StatusInternalServerError, "admin.product.createFailed")
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
		httpx.ErrorT(w, r, http.StatusBadRequest, "common.invalidBody")
		return
	}
	body.Spec = strings.TrimSpace(body.Spec)
	body.URL = strings.TrimSpace(body.URL)
	if (body.Spec == "") == (body.URL == "") {
		httpx.ErrorT(w, r, http.StatusBadRequest, "admin.import.oneOfSpecOrURL")
		return
	}

	data := []byte(body.Spec)
	if body.URL != "" {
		fetched, err := fetchSpec(r.Context(), body.URL, h.allowPrivate())
		if err != nil {
			httpx.ErrorT(w, r, http.StatusUnprocessableEntity, "admin.import.fetchFailed")
			return
		}
		data = fetched
	}

	draft, err := parseSpec(data)
	if err != nil {
		httpx.ErrorT(w, r, http.StatusUnprocessableEntity, "admin.import.parseFailed")
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
		httpx.ErrorT(w, r, http.StatusNotFound, "catalog.productNotFound")
		return
	}
	if errors.Is(err, ErrSlugTaken) {
		httpx.ErrorT(w, r, http.StatusConflict, "admin.product.slugTaken")
		return
	}
	if errors.Is(err, ErrContextPathTaken) {
		httpx.ErrorT(w, r, http.StatusConflict, "admin.product.contextPathTaken")
		return
	}
	if err != nil {
		log.Printf("admin update product %d: %v", id, err)
		httpx.ErrorT(w, r, http.StatusInternalServerError, "admin.product.updateFailed")
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
		httpx.ErrorT(w, r, http.StatusNotFound, "catalog.productNotFound")
		return
	}
	if errors.Is(err, ErrHasSubscriptions) {
		httpx.ErrorT(w, r, http.StatusConflict, "admin.product.hasSubscriptions")
		return
	}
	if err != nil {
		log.Printf("admin delete product %d: %v", id, err)
		httpx.ErrorT(w, r, http.StatusInternalServerError, "admin.product.deleteFailed")
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
		httpx.ErrorT(w, r, http.StatusBadRequest, "common.invalidBody")
		return
	}
	if strings.TrimSpace(e.Version) == "" {
		httpx.ErrorT(w, r, http.StatusBadRequest, "admin.changelog.versionRequired")
		return
	}
	if !validChangelogKinds[e.Kind] {
		httpx.ErrorT(w, r, http.StatusBadRequest, "admin.changelog.badKind")
		return
	}
	if _, err := time.Parse("2006-01-02", e.Date); err != nil {
		httpx.ErrorT(w, r, http.StatusBadRequest, "admin.changelog.badDate")
		return
	}
	created, err := h.svc.AddChangelog(r.Context(), id, e)
	if err != nil {
		log.Printf("admin add changelog for product %d: %v", id, err)
		httpx.ErrorT(w, r, http.StatusInternalServerError, "admin.changelog.addFailed")
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
		httpx.ErrorT(w, r, http.StatusInternalServerError, "admin.changelog.listFailed")
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
		httpx.ErrorT(w, r, http.StatusBadRequest, "admin.changelog.badID")
		return
	}
	if err := h.svc.DeleteChangelog(r.Context(), id, entryID); err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.ErrorT(w, r, http.StatusNotFound, "admin.changelog.notFound")
			return
		}
		log.Printf("admin delete changelog entry %d for product %d: %v", entryID, id, err)
		httpx.ErrorT(w, r, http.StatusInternalServerError, "admin.changelog.deleteFailed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

const iconMaxUpload = 256 << 10 // 256 KiB

func (h *Handler) uploadIcon(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, iconMaxUpload)
	file, _, err := r.FormFile("file")
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			httpx.ErrorT(w, r, http.StatusRequestEntityTooLarge, "admin.icon.tooLarge")
			return
		}
		httpx.ErrorT(w, r, http.StatusBadRequest, "admin.icon.badBody")
		return
	}
	defer file.Close()
	raw, err := io.ReadAll(file)
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			httpx.ErrorT(w, r, http.StatusRequestEntityTooLarge, "admin.icon.tooLarge")
			return
		}
		httpx.ErrorT(w, r, http.StatusBadRequest, "admin.icon.badBody")
		return
	}
	png, err := DecodeAndReencode(raw)
	if errors.Is(err, ErrIconType) {
		httpx.ErrorT(w, r, http.StatusUnsupportedMediaType, "admin.icon.badType")
		return
	} else if err != nil {
		httpx.ErrorT(w, r, http.StatusUnprocessableEntity, "admin.icon.undecodable")
		return
	}
	if _, err := h.svc.SetUploadedIcon(r.Context(), id, png); errors.Is(err, ErrNotFound) {
		httpx.ErrorT(w, r, http.StatusNotFound, "catalog.productNotFound")
		return
	} else if err != nil {
		log.Printf("upload icon (product=%d): %v", id, err)
		httpx.ErrorT(w, r, http.StatusInternalServerError, "catalog.list.failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// serveIcon returns a product's stored custom icon regardless of publish
// state. Mounted behind requireAdmin, so it is admin-only — used by the
// Composer's draft-icon preview, which can't use a plain <img src> against
// the published-only public endpoint.
func (h *Handler) serveIcon(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	data, _, err := h.svc.GetIcon(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpx.ErrorT(w, r, http.StatusNotFound, "catalog.productNotFound")
		return
	}
	if err != nil {
		log.Printf("serve admin icon (product=%d): %v", id, err)
		httpx.ErrorT(w, r, http.StatusInternalServerError, "catalog.list.failed")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(data)
}

func parseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.ErrorT(w, r, http.StatusBadRequest, "subscribe.badProductID")
		return 0, false
	}
	return id, true
}

func (h *Handler) decodeProduct(w http.ResponseWriter, r *http.Request) (Product, bool) {
	var p Product
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		httpx.ErrorT(w, r, http.StatusBadRequest, "common.invalidBody")
		return Product{}, false
	}
	if p.Tags == nil {
		p.Tags = []string{}
	}
	if msg := p.validate(h.allowPrivate()); msg != "" {
		httpx.ErrorT(w, r, http.StatusBadRequest, msg)
		return Product{}, false
	}
	if p.AuthType == "oauth2" && !h.oidcConfigured() {
		httpx.ErrorT(w, r, http.StatusBadRequest, "admin.product.oauthNotConfigured")
		return Product{}, false
	}
	if p.OpenAPISpec != "" {
		if _, err := parseSpec([]byte(p.OpenAPISpec)); err != nil {
			httpx.ErrorT(w, r, http.StatusBadRequest, "admin.product.badOpenapiSpec")
			return Product{}, false
		}
	}
	return p, true
}
