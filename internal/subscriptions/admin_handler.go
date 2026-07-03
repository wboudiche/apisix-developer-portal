package subscriptions

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"apisix-portal/internal/httpx"
	"apisix-portal/internal/paging"
)

// AdminService is the surface the admin subscription handler needs (satisfied by *Service).
type AdminService interface {
	AdminSubscriptions(ctx context.Context, statusFilter string, p paging.Params) ([]AdminSubscriptionView, int, error)
	Approve(ctx context.Context, subID int64) error
	Reject(ctx context.Context, subID int64) error
}

// AdminHandler serves the admin approval surface. Mount behind RequireAdmin.
type AdminHandler struct {
	svc    AdminService
	router chi.Router
}

func NewAdminHandler(svc AdminService) *AdminHandler {
	h := &AdminHandler{svc: svc, router: chi.NewRouter()}
	h.router.Get("/api/admin/subscriptions", h.list)
	h.router.Post("/api/admin/subscriptions/{id}/approve", h.approve)
	h.router.Post("/api/admin/subscriptions/{id}/reject", h.reject)
	return h
}

func (h *AdminHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.router.ServeHTTP(w, r) }

func (h *AdminHandler) list(w http.ResponseWriter, r *http.Request) {
	p := paging.Parse(r.URL.Query())
	items, total, err := h.svc.AdminSubscriptions(r.Context(), r.URL.Query().Get("status"), p)
	if err != nil {
		log.Printf("admin list subscriptions: %v", err)
		httpx.ErrorT(w, r, http.StatusInternalServerError, "subscribe.admin.listFailed")
		return
	}
	httpx.JSON(w, http.StatusOK, paging.New(items, total, p))
}

func (h *AdminHandler) approve(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, h.svc.Approve, "approve")
}

func (h *AdminHandler) reject(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, h.svc.Reject, "reject")
}

func (h *AdminHandler) transition(w http.ResponseWriter, r *http.Request, act func(context.Context, int64) error, name string) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.ErrorT(w, r, http.StatusBadRequest, "subscribe.admin.badSubscriptionID")
		return
	}
	if err := act(r.Context(), id); err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.ErrorT(w, r, http.StatusNotFound, "subscribe.admin.notFound")
			return
		}
		if errors.Is(err, ErrInvalidTransition) {
			httpx.ErrorT(w, r, http.StatusConflict, "subscribe.admin.invalidTransition")
			return
		}
		log.Printf("admin %s subscription %d: %v", name, id, err)
		httpx.ErrorT(w, r, http.StatusInternalServerError, "subscribe.admin.actionFailed", name)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
