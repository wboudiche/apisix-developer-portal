package settings

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

	"apisix-portal/internal/httpx"
)

// ItemView is the wire shape of one registry entry, merged with its current
// effective value. Secrets never carry Value/EnvDefault — only whether one is
// currently Set — so a GET response can never leak a credential.
type ItemView struct {
	Key        string  `json:"key"`
	Type       string  `json:"type"`
	Editable   bool    `json:"editable"`
	Secret     bool    `json:"secret"`
	Value      *string `json:"value"`      // nil for secrets
	Set        bool    `json:"set"`        // secrets: non-empty?
	Source     string  `json:"source"`     // "env"|"db"
	EnvDefault *string `json:"envDefault"` // nil for secrets
}

// GroupView is a UI section: every registry entry sharing the same Def.Group,
// in registry order.
type GroupView struct {
	Group string     `json:"group"`
	Items []ItemView `json:"items"`
}

// Handler serves the admin settings API. It does not itself enforce
// authentication/authorization — the caller mounts it behind admin-only
// middleware — but it does need the acting admin's id for Set/Reset audit
// rows, supplied via adminID (see NewHandler).
type Handler struct {
	svc     *Service
	adminID func(context.Context) int64
	router  chi.Router
}

// NewHandler builds the settings admin API. adminID extracts the
// authenticated admin's id from the request context for audit logging —
// callers pass auth.UserID. It is injected rather than imported directly
// because internal/auth already depends on internal/notify, which depends on
// internal/settings (for the dynamic SMTP sender's *settings.Effective
// reads); settings importing auth back would be an import cycle. Passing the
// extractor function keeps settings decoupled from auth while still reusing
// the exact same context key the auth middleware populates.
func NewHandler(svc *Service, adminID func(context.Context) int64) *Handler {
	h := &Handler{svc: svc, adminID: adminID, router: chi.NewRouter()}
	h.router.Get("/api/admin/settings", h.list)
	h.router.Put("/api/admin/settings", h.put)
	h.router.Post("/api/admin/settings/test", h.test)
	h.router.Delete("/api/admin/settings/{key}", h.reset)
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.router.ServeHTTP(w, r) }

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	e := h.svc.Snapshot()
	var groups []GroupView
	idx := map[string]int{}
	for _, d := range Registry {
		gi, ok := idx[d.Group]
		if !ok {
			groups = append(groups, GroupView{Group: d.Group})
			gi = len(groups) - 1
			idx[d.Group] = gi
		}
		it := ItemView{
			Key: d.Key, Type: string(d.Type), Editable: d.Editable, Secret: d.Secret,
			Set: e.Get(d.Key) != "", Source: e.Source[d.Key],
		}
		if !d.Secret {
			v := e.Get(d.Key)
			it.Value = &v
			def := h.svc.EnvDefault(d.Key)
			it.EnvDefault = &def
		}
		groups[gi].Items = append(groups[gi].Items, it)
	}
	httpx.JSON(w, http.StatusOK, groups)
}

type putBody struct {
	Values map[string]string `json:"values"`
	Force  bool              `json:"force"`
}

func (h *Handler) put(w http.ResponseWriter, r *http.Request) {
	var body putBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Values) == 0 {
		httpx.ErrorT(w, r, http.StatusBadRequest, "common.invalidBody")
		return
	}
	err := h.svc.Set(r.Context(), body.Values, h.adminID(r.Context()), body.Force)
	h.respond(w, r, err)
}

func (h *Handler) reset(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if _, ok := Lookup(key); !ok {
		httpx.ErrorT(w, r, http.StatusNotFound, "common.notFound")
		return
	}
	err := h.svc.Reset(r.Context(), key, h.adminID(r.Context()))
	h.respond(w, r, err)
}

func (h *Handler) respond(w http.ResponseWriter, r *http.Request, err error) {
	var fe FieldErrors
	var pe *ProbeError
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, ErrUnknownKey), errors.Is(err, ErrReadOnlyKey):
		httpx.ErrorT(w, r, http.StatusBadRequest, "settings.badKey")
	case errors.As(err, &fe):
		httpx.JSON(w, http.StatusUnprocessableEntity, map[string]any{"fields": fe})
	case errors.As(err, &pe):
		log.Printf("settings: probe-failed save attempt by admin=%d", h.adminID(r.Context()))
		httpx.JSON(w, http.StatusUnprocessableEntity, map[string]any{"probe": pe.Results})
	default:
		httpx.ErrorT(w, r, http.StatusInternalServerError, "settings.saveFailed")
	}
}

func (h *Handler) test(w http.ResponseWriter, r *http.Request) {
	var body putBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.ErrorT(w, r, http.StatusBadRequest, "common.invalidBody")
		return
	}
	httpx.JSON(w, http.StatusOK, h.svc.Test(r.Context(), body.Values, h.adminID(r.Context())))
}
