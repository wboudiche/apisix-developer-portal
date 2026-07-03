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
	maxBodyBytes   = 2 << 20  // 2 MiB cap on request and response bodies
	gatewayTimeout = 15 * time.Second
)

// hopByHop and sensitive inbound headers are never forwarded to the gateway.
var stripHeaders = map[string]bool{
	"Host": true, "Cookie": true, "Authorization": true, "Connection": true,
	"Keep-Alive": true, "Proxy-Authenticate": true, "Proxy-Authorization": true,
	"Te": true, "Trailer": true, "Transfer-Encoding": true, "Upgrade": true,
	"Content-Length": true,
	// Defense in depth: never let an apikey cross the proxy in either direction.
	// The key is injected server-side; a gateway that echoes it back must not
	// leak it to the browser.
	"Apikey": true,
}

type Handler struct {
	products Products
	access   Access
	gateway  string
	sandbox  string
	client   *http.Client
	router   chi.Router
}

func NewHandler(p Products, a Access, gatewayURL string, sandboxGatewayURL string) *Handler {
	h := &Handler{
		products: p, access: a,
		gateway: strings.TrimRight(gatewayURL, "/"),
		sandbox: strings.TrimRight(sandboxGatewayURL, "/"),
		client:  &http.Client{Timeout: gatewayTimeout},
		router:  chi.NewRouter(),
	}
	h.router.Get("/api/try/{slug}/context", h.context)
	// Sandbox routes must be registered before the prod catch-all so they match first.
	h.router.Handle("/api/try/{slug}/{appId}/sandbox", http.HandlerFunc(h.sandboxProxy))
	h.router.Handle("/api/try/{slug}/{appId}/sandbox/*", http.HandlerFunc(h.sandboxProxy))
	h.router.Handle("/api/try/{slug}/{appId}", http.HandlerFunc(h.proxy))
	h.router.Handle("/api/try/{slug}/{appId}/*", http.HandlerFunc(h.proxy))
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.router.ServeHTTP(w, r) }

func (h *Handler) context(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserID(r.Context())
	if userID == 0 {
		httpx.ErrorT(w, r, http.StatusUnauthorized, "tryit.unauthenticated")
		return
	}
	slug := chi.URLParam(r, "slug")
	id, _, err := h.products.ProductBySlug(r.Context(), slug)
	if errors.Is(err, ErrNotFound) {
		httpx.ErrorT(w, r, http.StatusNotFound, "catalog.productNotFound")
		return
	}
	if err != nil {
		httpx.ErrorT(w, r, http.StatusInternalServerError, "subscribe.oidcSetFailed")
		return
	}
	apps, err := h.access.ApprovedApps(r.Context(), userID, id)
	if err != nil {
		httpx.ErrorT(w, r, http.StatusInternalServerError, "subscribe.oidcSetFailed")
		return
	}
	if apps == nil {
		apps = []AppRef{}
	}
	sbAvail := false
	if h.sandbox != "" {
		sbAvail, _ = h.products.SandboxUpstream(r.Context(), slug)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"apps": apps, "sandboxAvailable": sbAvail})
}

func (h *Handler) proxy(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserID(r.Context())
	if userID == 0 {
		httpx.ErrorT(w, r, http.StatusUnauthorized, "tryit.unauthenticated")
		return
	}
	slug := chi.URLParam(r, "slug")
	appID, err := strconv.ParseInt(chi.URLParam(r, "appId"), 10, 64)
	if err != nil {
		httpx.ErrorT(w, r, http.StatusBadRequest, "tryit.badAppID")
		return
	}

	productID, contextPath, err := h.products.ProductBySlug(r.Context(), slug)
	if errors.Is(err, ErrNotFound) {
		httpx.ErrorT(w, r, http.StatusNotFound, "catalog.productNotFound")
		return
	}
	if err != nil {
		httpx.ErrorT(w, r, http.StatusInternalServerError, "subscribe.oidcSetFailed")
		return
	}

	owns, err := h.access.OwnsApp(r.Context(), appID, userID)
	if err != nil || !owns {
		httpx.ErrorT(w, r, http.StatusForbidden, "subscribe.notYourApplication")
		return
	}
	status, err := h.access.SubscriptionStatus(r.Context(), appID, productID)
	if err != nil || status != statusActive {
		httpx.ErrorT(w, r, http.StatusForbidden, "tryit.noApprovedSubscription")
		return
	}
	key, err := h.access.APIKey(r.Context(), appID)
	if err != nil || key == "" {
		httpx.ErrorT(w, r, http.StatusForbidden, "tryit.noKeyForApplication")
		return
	}

	h.do(w, r, h.gateway, key, contextPath, chi.URLParam(r, "*"))
}

func (h *Handler) sandboxProxy(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserID(r.Context())
	if userID == 0 {
		httpx.ErrorT(w, r, http.StatusUnauthorized, "tryit.unauthenticated")
		return
	}

	if h.sandbox == "" {
		httpx.ErrorT(w, r, http.StatusNotFound, "tryit.sandboxNotAvailable")
		return
	}

	slug := chi.URLParam(r, "slug")
	appID, err := strconv.ParseInt(chi.URLParam(r, "appId"), 10, 64)
	if err != nil {
		httpx.ErrorT(w, r, http.StatusBadRequest, "tryit.badAppID")
		return
	}

	productID, contextPath, err := h.products.ProductBySlug(r.Context(), slug)
	if errors.Is(err, ErrNotFound) {
		httpx.ErrorT(w, r, http.StatusNotFound, "catalog.productNotFound")
		return
	}
	if err != nil {
		httpx.ErrorT(w, r, http.StatusInternalServerError, "subscribe.oidcSetFailed")
		return
	}

	owns, err := h.access.OwnsApp(r.Context(), appID, userID)
	if err != nil || !owns {
		httpx.ErrorT(w, r, http.StatusForbidden, "subscribe.notYourApplication")
		return
	}
	status, err := h.access.SubscriptionStatus(r.Context(), appID, productID)
	if err != nil || status != statusActive {
		httpx.ErrorT(w, r, http.StatusForbidden, "tryit.noApprovedSubscription")
		return
	}

	ok, _ := h.products.SandboxUpstream(r.Context(), slug)
	if !ok {
		httpx.ErrorT(w, r, http.StatusNotFound, "tryit.noSandboxForProduct")
		return
	}

	key, _ := h.access.SandboxKey(r.Context(), appID)
	if key == "" {
		httpx.ErrorT(w, r, http.StatusForbidden, "tryit.noSandboxKeyForApplication")
		return
	}

	h.do(w, r, h.sandbox, key, contextPath, chi.URLParam(r, "*"))
}

// do builds and executes the proxied request to the gateway. It handles header
// stripping, key injection, body capping, and response forwarding. The host is
// ALWAYS gatewayBase — never client-supplied input (SSRF prevention).
func (h *Handler) do(w http.ResponseWriter, r *http.Request, gatewayBase, key, contextPath, rest string) {
	// Build the gateway target from the product's context path + the wildcard
	// remainder. The host is ALWAYS the configured gateway — never client input.
	rest = strings.TrimPrefix(rest, "/")
	target := gatewayBase + contextPath
	if rest != "" {
		target += "/" + rest
	}
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}

	body := http.MaxBytesReader(w, r.Body, maxBodyBytes)
	out, err := http.NewRequestWithContext(r.Context(), r.Method, target, body)
	if err != nil {
		httpx.ErrorT(w, r, http.StatusBadGateway, "tryit.buildRequestFailed")
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
		httpx.ErrorT(w, r, http.StatusBadGateway, "tryit.gatewayUnreachable")
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
