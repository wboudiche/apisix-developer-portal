package applications

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"apisix-portal/internal/auth"
	"apisix-portal/internal/events"
	"apisix-portal/internal/httpx"
	"apisix-portal/internal/paging"
)

type Store interface {
	Create(ctx context.Context, ownerID, teamID int64, name, description string) (Application, error)
	ListForUser(ctx context.Context, userID int64, p paging.Params) ([]Application, int, error)
}

// Membership resolves the caller's default team + validates chosen teams.
type Membership interface {
	PersonalTeamID(ctx context.Context, userID int64) (int64, error)
	Role(ctx context.Context, teamID, userID int64) (string, bool, error)
}

// EventLogger records the app-created activity event (satisfied by
// *events.Repo). nil disables logging.
type EventLogger interface {
	Log(ctx context.Context, appID int64, kind string, productID, planID *int64) error
}

type Handler struct {
	store  Store
	teams  Membership
	events EventLogger
	router chi.Router
}

func NewHandler(store Store, teams Membership, eventLog EventLogger) *Handler {
	h := &Handler{store: store, teams: teams, events: eventLog, router: chi.NewRouter()}
	h.router.Post("/api/applications", h.create)
	h.router.Get("/api/applications", h.list)
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.router.ServeHTTP(w, r) }

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	uid := auth.UserID(r.Context())
	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		TeamID      int64  `json:"teamId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Name) == "" {
		httpx.ErrorT(w, r, http.StatusBadRequest, "common.nameRequired")
		return
	}
	teamID := body.TeamID
	if teamID == 0 {
		var err error
		if teamID, err = h.teams.PersonalTeamID(r.Context(), uid); err != nil {
			httpx.ErrorT(w, r, http.StatusInternalServerError, "app.create.noPersonalTeam")
			return
		}
	} else {
		_, isMember, err := h.teams.Role(r.Context(), teamID, uid)
		if err != nil {
			httpx.ErrorT(w, r, http.StatusInternalServerError, "app.create.membershipCheckFailed")
			return
		}
		if !isMember {
			httpx.ErrorT(w, r, http.StatusForbidden, "app.create.notMember")
			return
		}
	}
	a, err := h.store.Create(r.Context(), uid, teamID, strings.TrimSpace(body.Name), body.Description)
	if err != nil {
		httpx.ErrorT(w, r, http.StatusInternalServerError, "app.create.failed")
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
	p := paging.Parse(r.URL.Query())
	apps, total, err := h.store.ListForUser(r.Context(), auth.UserID(r.Context()), p)
	if err != nil {
		httpx.ErrorT(w, r, http.StatusInternalServerError, "app.list.failed")
		return
	}
	httpx.JSON(w, http.StatusOK, paging.New(apps, total, p))
}
