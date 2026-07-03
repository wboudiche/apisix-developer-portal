package billing

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"apisix-portal/internal/auth"
	"apisix-portal/internal/httpx"
)

// TeamHandler serves the authenticated user's own team invoices.
type TeamHandler struct {
	svc    *Service
	router chi.Router
}

func NewTeamHandler(svc *Service) *TeamHandler {
	h := &TeamHandler{svc: svc, router: chi.NewRouter()}
	h.router.Get("/api/billing/invoices", h.listMine)
	return h
}
func (h *TeamHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.router.ServeHTTP(w, r) }

func (h *TeamHandler) listMine(w http.ResponseWriter, r *http.Request) {
	uid := auth.UserID(r.Context())
	if uid == 0 {
		httpx.ErrorT(w, r, http.StatusUnauthorized, "auth.middleware.missingToken")
		return
	}
	invoices, err := h.svc.ListForUser(r.Context(), uid)
	if err != nil {
		httpx.ErrorT(w, r, http.StatusInternalServerError, "billing.listFailed")
		return
	}
	if invoices == nil {
		invoices = []Invoice{}
	}
	httpx.JSON(w, http.StatusOK, invoices)
}

// AdminHandler serves all invoices + settlement actions (mounted behind requireAdmin).
type AdminHandler struct {
	svc    *Service
	router chi.Router
}

func NewAdminHandler(svc *Service) *AdminHandler {
	h := &AdminHandler{svc: svc, router: chi.NewRouter()}
	h.router.Get("/api/admin/invoices", h.listAll)
	h.router.Post("/api/admin/invoices/{id}/pay", h.pay)
	h.router.Post("/api/admin/invoices/{id}/void", h.void)
	return h
}
func (h *AdminHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.router.ServeHTTP(w, r) }

func (h *AdminHandler) listAll(w http.ResponseWriter, r *http.Request) {
	invoices, err := h.svc.ListAll(r.Context(), r.URL.Query().Get("status"))
	if err != nil {
		httpx.ErrorT(w, r, http.StatusInternalServerError, "billing.listFailed")
		return
	}
	if invoices == nil {
		invoices = []Invoice{}
	}
	httpx.JSON(w, http.StatusOK, invoices)
}

func (h *AdminHandler) pay(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, h.svc.MarkPaid)
}
func (h *AdminHandler) void(w http.ResponseWriter, r *http.Request) { h.transition(w, r, h.svc.Void) }

func (h *AdminHandler) transition(w http.ResponseWriter, r *http.Request, fn func(ctx context.Context, id int64) error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.ErrorT(w, r, http.StatusBadRequest, "billing.badInvoiceID")
		return
	}
	switch err := fn(r.Context(), id); {
	case err == nil:
		w.WriteHeader(http.StatusOK)
	case errors.Is(err, ErrNotFound):
		httpx.ErrorT(w, r, http.StatusNotFound, "billing.invoiceNotFound")
	case errors.Is(err, ErrInvalidTransition):
		httpx.ErrorT(w, r, http.StatusConflict, "billing.invalidTransition")
	default:
		httpx.ErrorT(w, r, http.StatusInternalServerError, "billing.actionFailed")
	}
}
