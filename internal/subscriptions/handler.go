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
	ActivePlanForApp(ctx context.Context, appID int64) (PlanInfo, error)
	GetSandboxKey(ctx context.Context, appID int64) (string, error)
	GetAppOIDCClientID(ctx context.Context, appID int64) (string, error)
	OAuthProductsForApp(ctx context.Context, appID int64) ([]ProductInfo, error)
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
	RequestsInWindow(ctx context.Context, consumer string, windowSeconds int) (int64, error)
}

// Quota is the per-app rate-limit usage snapshot returned by GET
// /api/applications/{id}/quota.
type Quota struct {
	HasQuota      bool  `json:"hasQuota"`
	Used          int64 `json:"used"`
	Limit         int   `json:"limit"`
	WindowSeconds int   `json:"windowSeconds"`
	Available     bool  `json:"available"`
}

// feedLimit caps how many activity rows the Overview feed loads.
const feedLimit = 20

// defaultUsageRange is used when the request omits ?range=.
const defaultUsageRange = "24h"

type Handler struct {
	svc               *Service
	reader            Reader
	events            EventReader
	usage             UsageReader
	owns              OwnerCheck
	router            chi.Router
	sandboxGatewayURL string
	oidcIssuer        string
}

func NewHandler(svc *Service, reader Reader, eventReader EventReader, owns OwnerCheck, sandboxGatewayURL string) *Handler {
	h := &Handler{svc: svc, reader: reader, events: eventReader, owns: owns, router: chi.NewRouter(), sandboxGatewayURL: sandboxGatewayURL}
	h.router.Get("/api/applications/{appID}", h.detail)
	h.router.Get("/api/applications/{appID}/usage", h.usageHandler)
	h.router.Get("/api/applications/{appID}/quota", h.quotaHandler)
	h.router.Post("/api/applications/{appID}/subscriptions", h.subscribe)
	h.router.Delete("/api/applications/{appID}/subscriptions/{productID}", h.unsubscribe)
	h.router.Post("/api/applications/{appID}/credentials/rotate", h.rotateKey)
	h.router.Post("/api/applications/{appID}/sandbox/enable", h.enableSandbox)
	h.router.Post("/api/applications/{appID}/sandbox/rotate", h.rotateSandbox)
	h.router.Put("/api/applications/{appID}/oidc-client", h.setOIDCClient)
	return h
}

// SetUsageReader wires the metrics backend. Left unset (nil) when metrics are
// not configured; the usage endpoint then reports unavailable.
func (h *Handler) SetUsageReader(u UsageReader) { h.usage = u }

// SetOIDCIssuer wires the OIDC issuer URL for the app detail endpoint.
// Left unset when OIDC is not configured.
func (h *Handler) SetOIDCIssuer(s string) { h.oidcIssuer = s }

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.router.ServeHTTP(w, r) }

func (h *Handler) authorize(w http.ResponseWriter, r *http.Request) (int64, bool) {
	appID, err := strconv.ParseInt(chi.URLParam(r, "appID"), 10, 64)
	if err != nil {
		httpx.ErrorT(w, r, http.StatusBadRequest, "subscribe.badAppID")
		return 0, false
	}
	ok, err := h.owns(r.Context(), appID, auth.UserID(r.Context()))
	if err != nil {
		httpx.ErrorT(w, r, http.StatusInternalServerError, "subscribe.ownershipCheckFailed")
		return 0, false
	}
	if !ok {
		httpx.ErrorT(w, r, http.StatusForbidden, "subscribe.notYourApplication")
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
		httpx.ErrorT(w, r, http.StatusBadRequest, "subscribe.productPlanRequired")
		return
	}
	cred, err := h.svc.Subscribe(r.Context(), appID, body.ProductID, body.PlanID)
	if errors.Is(err, ErrNotFound) {
		httpx.ErrorT(w, r, http.StatusNotFound, "subscribe.productPlanNotFound")
		return
	}
	if errors.Is(err, ErrAlreadySubscribed) {
		httpx.ErrorT(w, r, http.StatusConflict, "subscribe.alreadySubscribed")
		return
	}
	if errors.Is(err, ErrProductDeprecated) {
		httpx.ErrorT(w, r, http.StatusConflict, "subscribe.productDeprecated")
		return
	}
	if err != nil {
		log.Printf("subscribe failed (app=%d product=%d): %v", appID, body.ProductID, err)
		httpx.ErrorT(w, r, http.StatusInternalServerError, "subscribe.subscribeFailed")
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
		httpx.ErrorT(w, r, http.StatusBadRequest, "subscribe.badProductID")
		return
	}
	if err := h.svc.Unsubscribe(r.Context(), appID, productID); err != nil {
		log.Printf("unsubscribe failed (app=%d product=%d): %v", appID, productID, err)
		httpx.ErrorT(w, r, http.StatusInternalServerError, "subscribe.unsubscribeFailed")
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
		httpx.ErrorT(w, r, http.StatusInternalServerError, "subscribe.credentialLoadFailed")
		return
	}
	subs, err := h.reader.SubscriptionsForApp(r.Context(), appID)
	if err != nil {
		httpx.ErrorT(w, r, http.StatusInternalServerError, "subscribe.subscriptionsLoadFailed")
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
	if h.sandboxGatewayURL != "" {
		out.SandboxGatewayUrl = h.sandboxGatewayURL
		if sk, err := h.reader.GetSandboxKey(r.Context(), appID); err == nil && sk != "" {
			out.SandboxEnabled = true
		}
	}
	if h.oidcIssuer != "" {
		out.OIDCIssuer = h.oidcIssuer
		out.OIDCClientID, _ = h.reader.GetAppOIDCClientID(r.Context(), appID)
		if prods, err := h.reader.OAuthProductsForApp(r.Context(), appID); err == nil {
			out.OAuthEligible = len(prods) > 0
		}
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) rotateKey(w http.ResponseWriter, r *http.Request) {
	appID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	newKey, err := h.svc.RotateKey(r.Context(), appID)
	if errors.Is(err, ErrNoCredential) || errors.Is(err, ErrNoActiveSubscription) {
		httpx.ErrorT(w, r, http.StatusConflict, "subscribe.noKeyToRotate")
		return
	}
	if err != nil {
		log.Printf("rotate key failed (app=%d): %v", appID, err)
		httpx.ErrorT(w, r, http.StatusInternalServerError, "subscribe.rotationFailed")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"apiKey": newKey})
}

// quotaHandler serves GET /api/applications/{id}/quota — the per-app rate-limit
// usage snapshot, including the active plan's limit and approximate used count.
func (h *Handler) quotaHandler(w http.ResponseWriter, r *http.Request) {
	appID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")

	cred, err := h.reader.GetCredential(r.Context(), appID)
	if errors.Is(err, ErrNotFound) {
		httpx.JSON(w, http.StatusOK, Quota{HasQuota: false})
		return
	}
	if err != nil {
		httpx.ErrorT(w, r, http.StatusInternalServerError, "subscribe.credentialLoadFailed")
		return
	}
	plan, err := h.reader.ActivePlanForApp(r.Context(), appID)
	if errors.Is(err, ErrNoActiveSubscription) {
		httpx.JSON(w, http.StatusOK, Quota{HasQuota: false})
		return
	}
	if err != nil {
		httpx.ErrorT(w, r, http.StatusInternalServerError, "subscribe.planLoadFailed")
		return
	}

	q := Quota{HasQuota: true, Limit: plan.Count, WindowSeconds: plan.WindowSeconds}
	if h.usage != nil {
		used, err := h.usage.RequestsInWindow(r.Context(), cred.ConsumerUsername, plan.WindowSeconds)
		if err != nil {
			log.Printf("quota used for app %d (consumer %s): %v", appID, cred.ConsumerUsername, err)
		} else {
			q.Used = used
			q.Available = true
		}
	}
	httpx.JSON(w, http.StatusOK, q)
}

func (h *Handler) enableSandbox(w http.ResponseWriter, r *http.Request) {
	appID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	key, err := h.svc.EnableSandbox(r.Context(), appID)
	if errors.Is(err, ErrNoSandboxEligibleSubscription) || errors.Is(err, ErrSandboxNotConfigured) {
		httpx.ErrorT(w, r, http.StatusConflict, "subscribe.sandboxUnavailable")
		return
	}
	if err != nil {
		log.Printf("enable sandbox failed (app=%d): %v", appID, err)
		httpx.ErrorT(w, r, http.StatusInternalServerError, "subscribe.enableSandboxFailed")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"sandboxApiKey": key})
}

func (h *Handler) rotateSandbox(w http.ResponseWriter, r *http.Request) {
	appID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	key, err := h.svc.RotateSandboxKey(r.Context(), appID)
	if errors.Is(err, ErrNoSandboxKey) || errors.Is(err, ErrSandboxNotConfigured) {
		httpx.ErrorT(w, r, http.StatusConflict, "subscribe.noSandboxKeyToRotate")
		return
	}
	if err != nil {
		log.Printf("rotate sandbox key failed (app=%d): %v", appID, err)
		httpx.ErrorT(w, r, http.StatusInternalServerError, "subscribe.rotationFailed")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"sandboxApiKey": key})
}

func (h *Handler) setOIDCClient(w http.ResponseWriter, r *http.Request) {
	appID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	var body struct {
		ClientID string `json:"clientId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.ErrorT(w, r, http.StatusBadRequest, "subscribe.badBody")
		return
	}
	if err := h.svc.SetOIDCClientID(r.Context(), appID, body.ClientID); errors.Is(err, ErrInvalidClientID) {
		httpx.ErrorT(w, r, http.StatusBadRequest, "subscribe.invalidClientID")
		return
	} else if err != nil {
		log.Printf("set oidc client (app=%d): %v", appID, err)
		httpx.ErrorT(w, r, http.StatusInternalServerError, "subscribe.oidcSetFailed")
		return
	}
	w.WriteHeader(http.StatusOK)
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
		httpx.ErrorT(w, r, http.StatusBadRequest, "subscribe.unsupportedRange")
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
		httpx.ErrorT(w, r, http.StatusInternalServerError, "subscribe.credentialLoadFailed")
		return
	}
	if h.usage == nil {
		httpx.ErrorT(w, r, http.StatusServiceUnavailable, "subscribe.metricsUnavailable")
		return
	}
	u, err := h.usage.Usage(r.Context(), cred.ConsumerUsername, rng)
	if err != nil {
		// Surface the dependency outage explicitly; never fall back to demo data.
		log.Printf("usage for app %d (consumer %s): %v", appID, cred.ConsumerUsername, err)
		httpx.ErrorT(w, r, http.StatusServiceUnavailable, "subscribe.metricsUnavailable")
		return
	}
	if u.Series == nil {
		u.Series = []metrics.Point{}
	}
	httpx.JSON(w, http.StatusOK, u)
}
