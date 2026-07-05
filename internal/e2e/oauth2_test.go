package e2e

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// mintOAuthToken mints a client-credentials JWT from the bundled LemonLDAP::NG.
// The IdP selects its issuer by Host vhost, so we hit :8081 with
// Host: auth.example.com (matching the compose network alias APISIX resolves).
func (h *harness) mintOAuthToken(clientID, secret string) string {
	h.t.Helper()
	tokenURL := envOr("OAUTH_TOKEN_URL", "http://localhost:8081/oauth2/token")
	req, err := http.NewRequest(http.MethodPost, tokenURL,
		strings.NewReader("grant_type=client_credentials&scope=openid"))
	if err != nil {
		h.t.Fatalf("token request: %v", err)
	}
	req.Host = envOr("OAUTH_ISSUER_HOST", "auth.example.com")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(clientID, secret)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatalf("mint token %s: %v", clientID, err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		h.t.Fatalf("mint token %s: got %d body=%s", clientID, res.StatusCode, raw)
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.AccessToken == "" {
		h.t.Fatalf("mint token %s: decode %v body=%s", clientID, err, raw)
	}
	return out.AccessToken
}

// gatewayGetBearer calls the APISIX gateway with a Bearer token (empty omits it).
func (h *harness) gatewayGetBearer(path, token string) int {
	h.t.Helper()
	gwURL := envOr("APISIX_GATEWAY_URL", "http://localhost:9080")
	req, _ := http.NewRequest(http.MethodGet, gwURL+path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatalf("gateway %s: %v", path, err)
	}
	defer res.Body.Close()
	_, _ = io.ReadAll(res.Body)
	return res.StatusCode
}

// approvePending finds THIS run's pending subscription by app name and approves
// it. The queue is a shared, paginated {items:[...]} envelope.
func (h *harness) approvePending(admin, appName string) {
	h.t.Helper()
	var queue struct {
		Items []struct {
			ID              int64  `json:"id"`
			ApplicationName string `json:"applicationName"`
		} `json:"items"`
	}
	if code := h.api(http.MethodGet, "/api/admin/subscriptions?status=pending", admin, nil, &queue); code != http.StatusOK {
		h.t.Fatalf("admin queue: got %d want 200", code)
	}
	var subID int64
	for _, q := range queue.Items {
		if q.ApplicationName == appName {
			subID = q.ID
			break
		}
	}
	if subID == 0 {
		h.t.Fatalf("no pending subscription found for app %q", appName)
	}
	if code := h.api(http.MethodPost, "/api/admin/subscriptions/"+itoa(subID)+"/approve", admin, nil, nil); code != http.StatusNoContent {
		h.t.Fatalf("approve %s: got %d want 204", appName, subID)
	}
}

// publishOAuthAPI creates an OAuth2 product and subscribes a fresh app that has
// registered clientID as its OIDC client id, then approves it — driving the
// portal's real EnsureOAuthRoute path so the APISIX route's whitelist becomes
// exactly [clientID]. Returns the product's gateway context path.
func (h *harness) publishOAuthAPI(admin string, planID int64, clientID string) string {
	h.t.Helper()
	ctxPath := "/" + uniq("oa")
	var product struct {
		ID int64 `json:"id"`
	}
	if code := h.api(http.MethodPost, "/api/admin/products", admin, map[string]any{
		"name": uniq("OAuthProd"), "slug": uniq("oaprod"), "category": "Engineering",
		"version": "1.0.0", "contextPath": ctxPath, "description": "",
		"tags": []string{}, "icon": "", "upstreamUrl": "echo:8080",
		"authType": "oauth2", "published": true,
	}, &product); code != http.StatusCreated {
		h.t.Fatalf("create oauth2 product: got %d want 201", code)
	}
	h.t.Cleanup(func() {
		_ = h.gw.DeleteRoute(context.Background(), "prod_"+itoa(product.ID))
		_ = h.api(http.MethodDelete, "/api/admin/products/"+itoa(product.ID), admin, nil, nil)
	})

	dev := h.devToken("oadev")
	appName := uniq("OAuthApp")
	var app struct {
		ID int64 `json:"id"`
	}
	if code := h.api(http.MethodPost, "/api/applications", dev, map[string]any{"name": appName, "description": ""}, &app); code != http.StatusCreated {
		h.t.Fatalf("create app: got %d want 201", code)
	}
	h.t.Cleanup(func() { _ = h.gw.DeleteConsumer(context.Background(), "app_"+itoa(app.ID)) })

	// Register the app's OIDC client id BEFORE approval — the whitelist is built
	// from active subscribers' oidc_client_id at approve time.
	if code := h.api(http.MethodPut, h.appPath(app.ID)+"/oidc-client", dev, map[string]any{"clientId": clientID}, nil); code != http.StatusOK {
		h.t.Fatalf("set oidc client %q: got %d want 200", clientID, code)
	}
	if code := h.api(http.MethodPost, h.appPath(app.ID)+"/subscriptions", dev, map[string]any{
		"productId": product.ID, "planId": planID,
	}, nil); code != http.StatusOK && code != http.StatusCreated {
		h.t.Fatalf("subscribe: got %d want 200/201", code)
	}
	h.approvePending(admin, appName)
	return ctxPath
}

// TestOAuth2TwoClients drives the portal's real OAuth2 flow against the bundled
// LemonLDAP::NG: two OAuth2 APIs, each whitelisting a DIFFERENT real client id.
// A token minted for one client is accepted on its own API (200) and rejected
// on the other (403, valid RS256 signature but client_id not on that route's
// allow-list), and no token is 401. This exercises product authType=oauth2 →
// SetOIDCClientID → subscribe → approve → EnsureOAuthRoute against real APISIX
// JWKS validation, not a hand-built route.
func TestOAuth2TwoClients(t *testing.T) {
	if os.Getenv("OIDC_ISSUER") == "" {
		t.Skip("set OIDC_ISSUER=http://auth.example.com OIDC_CLIENT_ID_CLAIM=client_id (with `make full` up) to run the OAuth2 E2E")
	}
	h := newHarness(t)
	admin := h.adminToken()

	// One shared free-ish plan. OAuth2 routes carry no limit-count, but a
	// subscription still needs a plan to reference.
	var plan struct {
		ID int64 `json:"id"`
	}
	if code := h.api(http.MethodPost, "/api/admin/plans", admin, map[string]any{
		"name": uniq("OAuthPlan"), "rateLimit": 100, "windowSeconds": 60, "currency": "USD",
	}, &plan); code != http.StatusCreated {
		t.Fatalf("create plan: got %d want 201", code)
	}
	t.Cleanup(func() { _ = h.api(http.MethodDelete, "/api/admin/plans/"+itoa(plan.ID), admin, nil, nil) })

	// Two OAuth2 APIs, each bound to a different real LemonLDAP relying party.
	pathA := h.publishOAuthAPI(admin, plan.ID, "apisix-portal-app")
	pathB := h.publishOAuthAPI(admin, plan.ID, "apisix-portal-app2")

	// Allow APISIX route creation + discovery/JWKS fetch to settle.
	time.Sleep(1200 * time.Millisecond)

	tokA := h.mintOAuthToken("apisix-portal-app", "apisix-portal-secret")
	tokB := h.mintOAuthToken("apisix-portal-app2", "apisix-portal-secret2")

	cases := []struct {
		name string
		path string
		tok  string
		want int
	}{
		{"clientA token on API-A", pathA, tokA, http.StatusOK},
		{"clientB token on API-B", pathB, tokB, http.StatusOK},
		{"clientA token on API-B (not whitelisted)", pathB, tokA, http.StatusForbidden},
		{"clientB token on API-A (not whitelisted)", pathA, tokB, http.StatusForbidden},
		{"no token on API-A", pathA, "", http.StatusUnauthorized},
		{"no token on API-B", pathB, "", http.StatusUnauthorized},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if code := h.gatewayGetBearer(c.path+"/get", c.tok); code != c.want {
				t.Fatalf("got %d want %d", code, c.want)
			}
		})
	}
}
