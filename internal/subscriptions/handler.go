package subscriptions

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"apisix-portal/internal/auth"
	"apisix-portal/internal/events"
	"apisix-portal/internal/httpx"
	"apisix-portal/internal/metrics"
)

// OwnerCheck reports whether appID belongs to userID.
type OwnerCheck func(ctx context.Context, appID, userID int64) (bool, error)

// Reader is the read surface for app detail (satisfied by *Repo).
type Reader interface {
	GetCredential(ctx context.Context, appID int64) (Credential, error)
	SubscriptionsForApp(ctx context.Context, appID int64) ([]SubscriptionView, error)
}

// EventReader returns an application's recent activity (satisfied by
// *events.Repo). nil disables the feed.
type EventReader interface {
	Recent(ctx context.Context, appID int64, limit int) ([]events.View, error)
}

// UsageReader returns gateway traffic metrics for a consumer over a range
// (satisfied by *metrics.Service). nil disables the usage endpoint.
type UsageReader interface {
	Usage(ctx context.Context, consumer string, r metrics.Range) (metrics.Usage, error)
}

// feedLimit caps how many activity rows the Overview feed loads.
const feedLimit = 20

// defaultUsageRange is used when the request omits ?range=.
const defaultUsageRange = "24h"

type Handler struct {
	svc    *Service
	reader Reader
	events EventReader
	usage  UsageReader
	owns   OwnerCheck
	router chi.Router
}

func NewHandler(svc *Service, reader Reader, eventReader EventReader, owns OwnerCheck) *Handler {
	h := &Handler{svc: svc, reader: reader, events: eventReader, owns: owns, router: chi.NewRouter()}
	h.router.Get("/api/applications/{appID}", h.detail)
	h.router.Get("/api/applications/{appID}/usage", h.usageHandler)
	h.router.Post("/api/applications/{appID}/subscriptions", h.subscribe)
	h.router.Delete("/api/applications/{appID}/subscriptions/{productID}", h.unsubscribe)
	return h
}

// SetUsageReader wires the metrics backend. Left unset (nil) when metrics are
// not configured; the usage endpoint then reports unavailable.
func (h *Handler) SetUsageReader(u UsageReader) { h.usage = u }

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.router.ServeHTTP(w, r) }

func (h *Handler) authorize(w http.ResponseWriter, r *http.Request) (int64, bool) {
	appID, err := strconv.ParseInt(chi.URLParam(r, "appID"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "bad application id")
		return 0, false
	}
	ok, err := h.owns(r.Context(), appID, auth.UserID(r.Context()))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "ownership check failed")
		return 0, false
	}
	if !ok {
		httpx.Error(w, http.StatusForbidden, "not your application")
		return 0, false
	}
	return appID, true
}

func (h *Handler) subscribe(w http.ResponseWriter, r *http.Request) {
	appID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	var body struct {
		ProductID int64 `json:"productId"`
		PlanID    int64 `json:"planId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ProductID == 0 || body.PlanID == 0 {
		httpx.Error(w, http.StatusBadRequest, "productId and planId are required")
		return
	}
	cred, err := h.svc.Subscribe(r.Context(), appID, body.ProductID, body.PlanID)
	if errors.Is(err, ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "product or plan not found")
		return
	}
	if errors.Is(err, ErrAlreadySubscribed) {
		httpx.Error(w, http.StatusConflict, "already subscribed to this product")
		return
	}
	if err != nil {
		log.Printf("subscribe failed (app=%d product=%d): %v", appID, body.ProductID, err)
		httpx.Error(w, http.StatusInternalServerError, "subscription failed")
		return
	}
	httpx.JSON(w, http.StatusCreated, cred)
}

func (h *Handler) unsubscribe(w http.ResponseWriter, r *http.Request) {
	appID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	productID, err := strconv.ParseInt(chi.URLParam(r, "productID"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "bad product id")
		return
	}
	if err := h.svc.Unsubscribe(r.Context(), appID, productID); err != nil {
		log.Printf("unsubscribe failed (app=%d product=%d): %v", appID, productID, err)
		httpx.Error(w, http.StatusInternalServerError, "unsubscribe failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) detail(w http.ResponseWriter, r *http.Request) {
	appID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	out := AppDetail{Subscriptions: []SubscriptionView{}, Events: []events.View{}}
	cred, err := h.reader.GetCredential(r.Context(), appID)
	if err == nil {
		out.APIKey = cred.APIKey
		out.ConsumerUsername = cred.ConsumerUsername
	} else if err != ErrNotFound {
		httpx.Error(w, http.StatusInternalServerError, "failed to load credential")
		return
	}
	subs, err := h.reader.SubscriptionsForApp(r.Context(), appID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to load subscriptions")
		return
	}
	if subs != nil {
		out.Subscriptions = subs
	}
	if h.events != nil {
		// The feed is cosmetic; the detail page is load-bearing. Degrade
		// gracefully — a feed read error leaves out.Events as [] rather than
		// failing the whole page (symmetric with the best-effort write path).
		if feed, err := h.events.Recent(r.Context(), appID, feedLimit); err != nil {
			log.Printf("activity feed for app %d: %v", appID, err)
		} else if feed != nil {
			out.Events = feed
		}
	}
	httpx.JSON(w, http.StatusOK, out)
}

// usageHandler serves GET /api/applications/{id}/usage?range=24h|7d|30d — the
// Overview stat cards and traffic chart, scoped to the app's gateway consumer.
func (h *Handler) usageHandler(w http.ResponseWriter, r *http.Request) {
	appID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	rangeKey := r.URL.Query().Get("range")
	if rangeKey == "" {
		rangeKey = defaultUsageRange
	}
	rng, err := metrics.ParseRange(rangeKey)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "unsupported range")
		return
	}
	// The metrics are usage data per tenant — same no-store posture as detail.
	w.Header().Set("Cache-Control", "no-store")

	// Resolve the gateway consumer. No credential means the app has no
	// subscription yet, so no traffic — return zeroed usage, not an error.
	cred, err := h.reader.GetCredential(r.Context(), appID)
	if errors.Is(err, ErrNotFound) {
		httpx.JSON(w, http.StatusOK, metrics.Usage{Series: []metrics.Point{}})
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to load credential")
		return
	}
	if h.usage == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "metrics unavailable")
		return
	}
	u, err := h.usage.Usage(r.Context(), cred.ConsumerUsername, rng)
	if err != nil {
		// Surface the dependency outage explicitly; never fall back to demo data.
		log.Printf("usage for app %d (consumer %s): %v", appID, cred.ConsumerUsername, err)
		httpx.Error(w, http.StatusServiceUnavailable, "metrics unavailable")
		return
	}
	if u.Series == nil {
		u.Series = []metrics.Point{}
	}
	httpx.JSON(w, http.StatusOK, u)
}
