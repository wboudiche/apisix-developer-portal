package applications

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

	"apisix-portal/internal/auth"
	"apisix-portal/internal/events"
	"apisix-portal/internal/httpx"
)

type Store interface {
	Create(ctx context.Context, ownerID int64, name, description string) (Application, error)
	ListByOwner(ctx context.Context, ownerID int64) ([]Application, error)
}

// EventLogger records the app-created activity event (satisfied by
// *events.Repo). nil disables logging.
type EventLogger interface {
	Log(ctx context.Context, appID int64, kind string, productID, planID *int64) error
}

type Handler struct {
	store  Store
	events EventLogger
	router chi.Router
}

func NewHandler(store Store, eventLog EventLogger) *Handler {
	h := &Handler{store: store, events: eventLog, router: chi.NewRouter()}
	h.router.Post("/api/applications", h.create)
	h.router.Get("/api/applications", h.list)
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.router.ServeHTTP(w, r) }

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		httpx.Error(w, http.StatusBadRequest, "name is required")
		return
	}
	a, err := h.store.Create(r.Context(), auth.UserID(r.Context()), body.Name, body.Description)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to create application")
		return
	}
	// Best-effort activity log: a failure here must not fail app creation.
	if h.events != nil {
		if err := h.events.Log(r.Context(), a.ID, events.KindAppCreated, nil, nil); err != nil {
			log.Printf("activity log app_created for app %d: %v", a.ID, err)
		}
	}
	httpx.JSON(w, http.StatusCreated, a)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	apps, err := h.store.ListByOwner(r.Context(), auth.UserID(r.Context()))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to list applications")
		return
	}
	if apps == nil {
		apps = []Application{}
	}
	httpx.JSON(w, http.StatusOK, apps)
}
