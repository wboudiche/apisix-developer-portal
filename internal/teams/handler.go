package teams

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"apisix-portal/internal/auth"
	"apisix-portal/internal/httpx"
)

type Handler struct {
	store  Store
	router chi.Router
}

func NewHandler(store Store) *Handler {
	h := &Handler{store: store, router: chi.NewRouter()}
	h.router.Get("/api/teams", h.list)
	h.router.Post("/api/teams", h.create)
	h.router.Get("/api/teams/{id}/members", h.members)
	h.router.Post("/api/teams/{id}/members", h.addMember)
	h.router.Delete("/api/teams/{id}/members/{userId}", h.removeMember)
	h.router.Patch("/api/teams/{id}", h.rename)
	h.router.Delete("/api/teams/{id}", h.deleteTeam)
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.router.ServeHTTP(w, r) }

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	ts, err := h.store.ListForUser(r.Context(), auth.UserID(r.Context()))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not list teams")
		return
	}
	if ts == nil {
		ts = []TeamSummary{}
	}
	httpx.JSON(w, http.StatusOK, ts)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Name) == "" {
		httpx.Error(w, http.StatusBadRequest, "name is required")
		return
	}
	t, err := h.store.Create(r.Context(), strings.TrimSpace(body.Name), auth.UserID(r.Context()))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not create team")
		return
	}
	httpx.JSON(w, http.StatusCreated, t)
}

func (h *Handler) members(w http.ResponseWriter, r *http.Request) {
	teamID, ok := h.requireMember(w, r)
	if !ok {
		return
	}
	ms, err := h.store.Members(r.Context(), teamID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not list members")
		return
	}
	if ms == nil {
		ms = []Member{}
	}
	httpx.JSON(w, http.StatusOK, ms)
}

func (h *Handler) addMember(w http.ResponseWriter, r *http.Request) {
	teamID, ok := h.requireOwner(w, r)
	if !ok {
		return
	}
	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Email) == "" {
		httpx.Error(w, http.StatusBadRequest, "email is required")
		return
	}
	err := h.store.AddMemberByEmail(r.Context(), teamID, strings.TrimSpace(body.Email))
	switch {
	case errors.Is(err, ErrUserNotFound):
		httpx.Error(w, http.StatusNotFound, "no user with that email")
	case errors.Is(err, ErrAlreadyMember):
		httpx.Error(w, http.StatusConflict, "already a member")
	case errors.Is(err, ErrPersonalTeam):
		httpx.Error(w, http.StatusConflict, "cannot add members to a personal team")
	case err != nil:
		httpx.Error(w, http.StatusInternalServerError, "could not add member")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *Handler) removeMember(w http.ResponseWriter, r *http.Request) {
	teamID, ok := h.requireOwner(w, r)
	if !ok {
		return
	}
	userID, err := strconv.ParseInt(chi.URLParam(r, "userId"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "bad user id")
		return
	}
	err = h.store.RemoveMember(r.Context(), teamID, userID)
	switch {
	case errors.Is(err, ErrLastOwner):
		httpx.Error(w, http.StatusConflict, "cannot remove the last owner")
	case errors.Is(err, ErrPersonalTeam):
		httpx.Error(w, http.StatusConflict, "cannot modify a personal team")
	case err != nil:
		httpx.Error(w, http.StatusInternalServerError, "could not remove member")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *Handler) rename(w http.ResponseWriter, r *http.Request) {
	teamID, ok := h.requireOwner(w, r)
	if !ok {
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Name) == "" {
		httpx.Error(w, http.StatusBadRequest, "name is required")
		return
	}
	err := h.store.Rename(r.Context(), teamID, strings.TrimSpace(body.Name))
	switch {
	case errors.Is(err, ErrPersonalTeam):
		httpx.Error(w, http.StatusConflict, "cannot rename a personal team")
	case err != nil:
		httpx.Error(w, http.StatusInternalServerError, "could not rename team")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *Handler) deleteTeam(w http.ResponseWriter, r *http.Request) {
	teamID, ok := h.requireOwner(w, r)
	if !ok {
		return
	}
	err := h.store.Delete(r.Context(), teamID)
	switch {
	case errors.Is(err, ErrTeamHasApps):
		httpx.Error(w, http.StatusConflict, "team still has applications")
	case errors.Is(err, ErrPersonalTeam):
		httpx.Error(w, http.StatusConflict, "cannot delete a personal team")
	case err != nil:
		httpx.Error(w, http.StatusInternalServerError, "could not delete team")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *Handler) teamID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	return id, err == nil
}

func (h *Handler) requireMember(w http.ResponseWriter, r *http.Request) (int64, bool) {
	teamID, ok := h.teamID(r)
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "bad team id")
		return 0, false
	}
	_, isMember, err := h.store.Role(r.Context(), teamID, auth.UserID(r.Context()))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not check membership")
		return 0, false
	}
	if !isMember {
		httpx.Error(w, http.StatusForbidden, "not a team member")
		return 0, false
	}
	return teamID, true
}

func (h *Handler) requireOwner(w http.ResponseWriter, r *http.Request) (int64, bool) {
	teamID, ok := h.teamID(r)
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "bad team id")
		return 0, false
	}
	role, isMember, err := h.store.Role(r.Context(), teamID, auth.UserID(r.Context()))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not check membership")
		return 0, false
	}
	if !isMember || role != "owner" {
		httpx.Error(w, http.StatusForbidden, "owner only")
		return 0, false
	}
	return teamID, true
}
