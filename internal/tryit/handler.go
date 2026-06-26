package tryit

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"apisix-portal/internal/auth"
	"apisix-portal/internal/httpx"
)

const (
	statusActive   = "active" // mirrors subscriptions.StatusActive
	maxBodyBytes   = 2 << 20   // 2 MiB cap on request and response bodies
	gatewayTimeout = 15 * time.Second
)

// hopByHop and sensitive inbound headers are never forwarded to the gateway.
var stripHeaders = map[string]bool{
	"Host": true, "Cookie": true, "Authorization": true, "Connection": true,
	"Keep-Alive": true, "Proxy-Authenticate": true, "Proxy-Authorization": true,
	"Te": true, "Trailer": true, "Transfer-Encoding": true, "Upgrade": true,
	"Content-Length": true,
}

type Handler struct {
	products Products
	access   Access
	gateway  string
	client   *http.Client
	router   chi.Router
}

func NewHandler(p Products, a Access, gatewayURL string) *Handler {
	h := &Handler{
		products: p, access: a,
		gateway: strings.TrimRight(gatewayURL, "/"),
		client:  &http.Client{Timeout: gatewayTimeout},
		router:  chi.NewRouter(),
	}
	h.router.Get("/api/try/{slug}/context", h.context)
	h.router.Handle("/api/try/{slug}/{appId}", http.HandlerFunc(h.proxy))
	h.router.Handle("/api/try/{slug}/{appId}/*", http.HandlerFunc(h.proxy))
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.router.ServeHTTP(w, r) }

func (h *Handler) context(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserID(r.Context())
	id, _, err := h.products.ProductBySlug(r.Context(), chi.URLParam(r, "slug"))
	if errors.Is(err, ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "product not found")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed")
		return
	}
	apps, err := h.access.ApprovedApps(r.Context(), userID, id)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed")
		return
	}
	if apps == nil {
		apps = []AppRef{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"apps": apps})
}

func (h *Handler) proxy(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserID(r.Context())
	slug := chi.URLParam(r, "slug")
	appID, err := strconv.ParseInt(chi.URLParam(r, "appId"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "bad app id")
		return
	}

	productID, contextPath, err := h.products.ProductBySlug(r.Context(), slug)
	if errors.Is(err, ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "product not found")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed")
		return
	}

	owns, err := h.access.OwnsApp(r.Context(), appID, userID)
	if err != nil || !owns {
		httpx.Error(w, http.StatusForbidden, "not your application")
		return
	}
	status, err := h.access.SubscriptionStatus(r.Context(), appID, productID)
	if err != nil || status != statusActive {
		httpx.Error(w, http.StatusForbidden, "no approved subscription for this API")
		return
	}
	key, err := h.access.APIKey(r.Context(), appID)
	if err != nil || key == "" {
		httpx.Error(w, http.StatusForbidden, "no key for this application")
		return
	}

	// Build the gateway target from the product's context path + the wildcard
	// remainder. The host is ALWAYS the configured gateway — never client input.
	rest := strings.TrimPrefix(chi.URLParam(r, "*"), "/")
	target := h.gateway + contextPath
	if rest != "" {
		target += "/" + rest
	}
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}

	body := http.MaxBytesReader(w, r.Body, maxBodyBytes)
	out, err := http.NewRequestWithContext(r.Context(), r.Method, target, body)
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "could not build gateway request")
		return
	}
	for name, vals := range r.Header {
		if stripHeaders[http.CanonicalHeaderKey(name)] {
			continue
		}
		for _, v := range vals {
			out.Header.Add(name, v)
		}
	}
	out.Header.Set("apikey", key)

	resp, err := h.client.Do(out)
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "gateway unreachable")
		return
	}
	defer resp.Body.Close()

	for name, vals := range resp.Header {
		if stripHeaders[http.CanonicalHeaderKey(name)] {
			continue
		}
		for _, v := range vals {
			w.Header().Add(name, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, io.LimitReader(resp.Body, maxBodyBytes))
}
