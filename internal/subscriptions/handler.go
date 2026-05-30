package subscriptions

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"apisix-portal/internal/auth"
	"apisix-portal/internal/httpx"
)

// OwnerCheck reports whether appID belongs to userID.
type OwnerCheck func(ctx context.Context, appID, userID int64) (bool, error)

// Reader is the read surface for app detail (satisfied by *Repo).
type Reader interface {
	GetCredential(ctx context.Context, appID int64) (Credential, error)
	SubscriptionsForApp(ctx context.Context, appID int64) ([]SubscriptionView, error)
}

type Handler struct {
	svc    *Service
	reader Reader
	owns   OwnerCheck
	router chi.Router
}

func NewHandler(svc *Service, reader Reader, owns OwnerCheck) *Handler {
	h := &Handler{svc: svc, reader: reader, owns: owns, router: chi.NewRouter()}
	h.router.Get("/api/applications/{appID}", h.detail)
	h.router.Post("/api/applications/{appID}/subscriptions", h.subscribe)
	h.router.Delete("/api/applications/{appID}/subscriptions/{productID}", h.unsubscribe)
	return h
}

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
	out := AppDetail{Subscriptions: []SubscriptionView{}}
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
	httpx.JSON(w, http.StatusOK, out)
}
