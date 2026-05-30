package subscriptions

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"apisix-portal/internal/auth"
	"apisix-portal/internal/httpx"
)

// OwnerCheck reports whether appID belongs to userID.
type OwnerCheck func(ctx context.Context, appID, userID int64) (bool, error)

type Handler struct {
	svc    *Service
	owns   OwnerCheck
	router chi.Router
}

func NewHandler(svc *Service, owns OwnerCheck) *Handler {
	h := &Handler{svc: svc, owns: owns, router: chi.NewRouter()}
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
		httpx.Error(w, http.StatusInternalServerError, "subscription/provisioning failed: "+err.Error())
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
		httpx.Error(w, http.StatusInternalServerError, "unsubscribe failed: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
