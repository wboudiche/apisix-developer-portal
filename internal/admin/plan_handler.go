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

// PlanAdminService is the surface the plan handler needs (satisfied by *PlanService).
type PlanAdminService interface {
	List(ctx context.Context) ([]Plan, error)
	Create(ctx context.Context, p Plan) (Plan, error)
	Update(ctx context.Context, p Plan) (Plan, error)
	Delete(ctx context.Context, id int64) error
}

type PlanHandler struct {
	svc    PlanAdminService
	router chi.Router
}

func NewPlanHandler(svc PlanAdminService) *PlanHandler {
	h := &PlanHandler{svc: svc, router: chi.NewRouter()}
	h.router.Get("/api/admin/plans", h.list)
	h.router.Post("/api/admin/plans", h.create)
	h.router.Put("/api/admin/plans/{id}", h.update)
	h.router.Delete("/api/admin/plans/{id}", h.delete)
	return h
}

func (h *PlanHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.router.ServeHTTP(w, r) }

func (h *PlanHandler) list(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List(r.Context())
	if err != nil {
		log.Printf("admin list plans: %v", err)
		httpx.Error(w, http.StatusInternalServerError, "failed to list plans")
		return
	}
	if items == nil {
		items = []Plan{}
	}
	httpx.JSON(w, http.StatusOK, items)
}

func (h *PlanHandler) create(w http.ResponseWriter, r *http.Request) {
	p, ok := decodePlan(w, r)
	if !ok {
		return
	}
	created, err := h.svc.Create(r.Context(), p)
	if errors.Is(err, ErrPlanNameTaken) {
		httpx.Error(w, http.StatusConflict, "plan name already exists")
		return
	}
	if err != nil {
		log.Printf("admin create plan: %v", err)
		httpx.Error(w, http.StatusInternalServerError, "failed to create plan")
		return
	}
	httpx.JSON(w, http.StatusCreated, created)
}

func (h *PlanHandler) update(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePlanID(w, r)
	if !ok {
		return
	}
	p, ok := decodePlan(w, r)
	if !ok {
		return
	}
	p.ID = id
	updated, err := h.svc.Update(r.Context(), p)
	if errors.Is(err, ErrPlanNotFound) {
		httpx.Error(w, http.StatusNotFound, "plan not found")
		return
	}
	if errors.Is(err, ErrPlanNameTaken) {
		httpx.Error(w, http.StatusConflict, "plan name already exists")
		return
	}
	if err != nil {
		log.Printf("admin update plan %d: %v", id, err)
		httpx.Error(w, http.StatusInternalServerError, "failed to update plan")
		return
	}
	httpx.JSON(w, http.StatusOK, updated)
}

func (h *PlanHandler) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePlanID(w, r)
	if !ok {
		return
	}
	err := h.svc.Delete(r.Context(), id)
	if errors.Is(err, ErrPlanNotFound) {
		httpx.Error(w, http.StatusNotFound, "plan not found")
		return
	}
	if errors.Is(err, ErrPlanInUse) {
		httpx.Error(w, http.StatusConflict, "plan is referenced by subscriptions")
		return
	}
	if err != nil {
		log.Printf("admin delete plan %d: %v", id, err)
		httpx.Error(w, http.StatusInternalServerError, "failed to delete plan")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parsePlanID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "bad plan id")
		return 0, false
	}
	return id, true
}

func decodePlan(w http.ResponseWriter, r *http.Request) (Plan, bool) {
	var p Plan
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid body")
		return Plan{}, false
	}
	if msg := p.validate(); msg != "" {
		httpx.Error(w, http.StatusBadRequest, msg)
		return Plan{}, false
	}
	return p, true
}
