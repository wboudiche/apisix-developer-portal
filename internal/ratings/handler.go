package ratings

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"apisix-portal/internal/auth"
	"apisix-portal/internal/httpx"
)

var ErrNotFound = errors.New("ratings: product not found")

type Products interface {
	ProductBySlug(ctx context.Context, slug string) (int64, error)
}
type Subscribers interface {
	IsApprovedSubscriber(ctx context.Context, userID, productID int64) (bool, error)
}
type Store interface {
	Upsert(ctx context.Context, productID, userID int64, stars int, comment string) error
	List(ctx context.Context, productID int64) ([]Review, error)
	Mine(ctx context.Context, productID, userID int64) (*Review, error)
	SummaryFor(ctx context.Context, productID int64) (Summary, error)
}

type RatingsView struct {
	Average float64  `json:"average"`
	Count   int      `json:"count"`
	Items   []Review `json:"items"`
	Mine    *Review  `json:"mine"`
	CanRate bool     `json:"canRate"`
}

const maxComment = 500

type Handler struct {
	store    Store
	products Products
	subs     Subscribers
	tok      *auth.Tokenizer
	router   chi.Router
}

func NewHandler(store Store, products Products, subs Subscribers, tok *auth.Tokenizer) *Handler {
	h := &Handler{store: store, products: products, subs: subs, tok: tok, router: chi.NewRouter()}
	h.router.Get("/api/ratings/{slug}", h.get)
	h.router.With(auth.RequireAuth(tok)).Put("/api/ratings/{slug}", h.put)
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.router.ServeHTTP(w, r) }

// optionalUserID returns the caller's id when a valid bearer token is present,
// else (0,false). The GET path is public but enriches the view when authed.
func (h *Handler) optionalUserID(r *http.Request) (int64, bool) {
	a := r.Header.Get("Authorization")
	if !strings.HasPrefix(a, "Bearer ") {
		return 0, false
	}
	claims, err := h.tok.Parse(strings.TrimPrefix(a, "Bearer "))
	if err != nil {
		return 0, false
	}
	return claims.UserID, true
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	pid, err := h.products.ProductBySlug(r.Context(), chi.URLParam(r, "slug"))
	if errors.Is(err, ErrNotFound) {
		httpx.ErrorT(w, r, http.StatusNotFound, "catalog.productNotFound")
		return
	}
	if err != nil {
		httpx.ErrorT(w, r, http.StatusInternalServerError, "subscribe.oidcSetFailed")
		return
	}
	view, err := h.buildView(r.Context(), pid, r)
	if err != nil {
		httpx.ErrorT(w, r, http.StatusInternalServerError, "subscribe.oidcSetFailed")
		return
	}
	httpx.JSON(w, http.StatusOK, view)
}

func (h *Handler) put(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserID(r.Context())
	pid, err := h.products.ProductBySlug(r.Context(), chi.URLParam(r, "slug"))
	if errors.Is(err, ErrNotFound) {
		httpx.ErrorT(w, r, http.StatusNotFound, "catalog.productNotFound")
		return
	}
	if err != nil {
		httpx.ErrorT(w, r, http.StatusInternalServerError, "subscribe.oidcSetFailed")
		return
	}
	approved, err := h.subs.IsApprovedSubscriber(r.Context(), userID, pid)
	if err != nil {
		httpx.ErrorT(w, r, http.StatusInternalServerError, "subscribe.oidcSetFailed")
		return
	}
	if !approved {
		httpx.ErrorT(w, r, http.StatusForbidden, "ratings.subscribeToRate")
		return
	}
	var body struct {
		Stars   int    `json:"stars"`
		Comment string `json:"comment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Stars < 1 || body.Stars > 5 {
		httpx.ErrorT(w, r, http.StatusBadRequest, "ratings.badStars")
		return
	}
	comment := strings.TrimSpace(body.Comment)
	if r := []rune(comment); len(r) > maxComment {
		comment = string(r[:maxComment])
	}
	if err := h.store.Upsert(r.Context(), pid, userID, body.Stars, comment); err != nil {
		httpx.ErrorT(w, r, http.StatusInternalServerError, "ratings.saveFailed")
		return
	}
	view, err := h.buildView(r.Context(), pid, r)
	if err != nil {
		httpx.ErrorT(w, r, http.StatusInternalServerError, "subscribe.oidcSetFailed")
		return
	}
	httpx.JSON(w, http.StatusOK, view)
}

func (h *Handler) buildView(ctx context.Context, pid int64, r *http.Request) (RatingsView, error) {
	sum, err := h.store.SummaryFor(ctx, pid)
	if err != nil {
		return RatingsView{}, err
	}
	items, err := h.store.List(ctx, pid)
	if err != nil {
		return RatingsView{}, err
	}
	if items == nil {
		items = []Review{}
	}
	v := RatingsView{Average: sum.Average, Count: sum.Count, Items: items}
	if uid, ok := h.optionalUserID(r); ok {
		if mine, err := h.store.Mine(ctx, pid, uid); err == nil {
			v.Mine = mine
		}
		if can, err := h.subs.IsApprovedSubscriber(ctx, uid, pid); err == nil {
			v.CanRate = can
		}
	}
	return v, nil
}
