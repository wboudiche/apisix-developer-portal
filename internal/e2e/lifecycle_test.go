package e2e

import (
	"context"
	"net/http"
	"testing"
	"time"

	"apisix-portal/internal/auth"
)

// adminToken registers a fresh, per-run user, promotes it to admin via the repo
// (so the test does not depend on pre-seeded dev data — matters for a fresh CI
// DB), then logs in to get a token carrying the admin role. A unique email is
// used per run because a fixed admin email collides with a prior run's account
// whose password we no longer know (register 409 → login 401 on a dirty DB).
func (h *harness) adminToken() string {
	h.t.Helper()
	email := uniq("admin") + "@e2e.test"
	pw := "adminpass-e2e"
	var reg struct {
		Token string `json:"token"`
	}
	if code := h.api(http.MethodPost, "/api/auth/register", "", map[string]any{
		"email": email, "password": pw, "name": "Admin",
	}, &reg); code != http.StatusCreated {
		h.t.Fatalf("admin register: got %d want 201", code)
	}
	if err := auth.NewRepo(h.pool).EnsureAdminRole(context.Background(), email); err != nil {
		h.t.Fatalf("promote admin: %v", err)
	}
	// Re-login so the issued token carries the freshly-granted admin role.
	var out struct {
		Token string `json:"token"`
	}
	code := h.api(http.MethodPost, "/api/auth/login", "", map[string]any{
		"email": email, "password": pw,
	}, &out)
	if code != http.StatusOK || out.Token == "" {
		h.t.Fatalf("admin login: got %d token=%q", code, out.Token)
	}
	return out.Token
}

// devToken registers and logs in a fresh developer, returning the token.
func (h *harness) devToken(label string) string {
	h.t.Helper()
	email := uniq(label) + "@e2e.test"
	var out struct {
		Token string `json:"token"`
	}
	code := h.api(http.MethodPost, "/api/auth/register", "", map[string]any{
		"email": email, "password": "pw-12345", "name": label,
	}, &out)
	if code != http.StatusCreated || out.Token == "" {
		h.t.Fatalf("register %s: got %d token=%q", label, code, out.Token)
	}
	return out.Token
}

// pollSubStatus waits (bounded) for the app's subscription to reach the wanted
// status, returning the apiKey from the detail. APISIX route propagation is
// handled separately by the caller before gateway calls.
func (h *harness) pollSubStatus(appID int64, token, want string) string {
	h.t.Helper()
	type sub struct {
		ProductID int64  `json:"productId"`
		Status    string `json:"status"`
	}
	type detail struct {
		APIKey        string `json:"apiKey"`
		Subscriptions []sub  `json:"subscriptions"`
	}
	for i := 0; i < 20; i++ {
		var d detail
		code := h.api(http.MethodGet, h.appPath(appID), token, nil, &d)
		if code == http.StatusOK {
			ok := len(d.Subscriptions) > 0
			for _, s := range d.Subscriptions {
				if s.Status != want {
					ok = false
				}
			}
			if ok {
				return d.APIKey
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	h.t.Fatalf("subscription never reached status %q for app %d", want, appID)
	return ""
}

func (h *harness) appPath(appID int64) string {
	return "/api/applications/" + itoa(appID)
}
func itoa(n int64) string {
	// avoid strconv import churn in callers; small helper
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func TestLifecycle(t *testing.T) {
	h := newHarness(t)
	admin := h.adminToken()

	// 1. publish a product + a low-limit plan
	ctxPath := "/" + uniq("e2e")
	var product struct {
		ID int64 `json:"id"`
	}
	if code := h.api(http.MethodPost, "/api/admin/products", admin, map[string]any{
		"name": uniq("Prod"), "slug": uniq("prod"), "category": "Engineering",
		"version": "1.0.0", "contextPath": ctxPath, "description": "",
		"tags": []string{}, "icon": "", "upstreamUrl": "echo:8080", "published": true,
	}, &product); code != http.StatusCreated {
		t.Fatalf("create product: got %d want 201", code)
	}
	t.Cleanup(func() {
		_ = h.gw.DeleteRoute(context.Background(), "prod_"+itoa(product.ID))
		_ = h.api(http.MethodDelete, "/api/admin/products/"+itoa(product.ID), admin, nil, nil)
	})

	var plan struct {
		ID int64 `json:"id"`
	}
	if code := h.api(http.MethodPost, "/api/admin/plans", admin, map[string]any{
		"name": uniq("Plan"), "rateLimit": 2, "windowSeconds": 60, "currency": "USD",
	}, &plan); code != http.StatusCreated {
		t.Fatalf("create plan: got %d want 201", code)
	}
	// Delete the plan last (after subscriptions are torn down) so the shared DB
	// isn't left with extra plans for the next run / other suites.
	t.Cleanup(func() { _ = h.api(http.MethodDelete, "/api/admin/plans/"+itoa(plan.ID), admin, nil, nil) })

	// 2. developer creates an app and subscribes
	dev := h.devToken("dev")
	appName := uniq("App")
	var app struct {
		ID int64 `json:"id"`
	}
	if code := h.api(http.MethodPost, "/api/applications", dev, map[string]any{"name": appName, "description": ""}, &app); code != http.StatusCreated {
		t.Fatalf("create app: got %d want 201", code)
	}
	t.Cleanup(func() { _ = h.gw.DeleteConsumer(context.Background(), "app_"+itoa(app.ID)) })

	if code := h.api(http.MethodPost, h.appPath(app.ID)+"/subscriptions", dev, map[string]any{
		"productId": product.ID, "planId": plan.ID,
	}, nil); code != http.StatusOK && code != http.StatusCreated {
		t.Fatalf("subscribe: got %d want 200/201", code)
	}

	// 3. before approval no route exists for this fresh context path → 404
	//    (pinning the exact code so a 500/transport error can't masquerade as success)
	if code := h.gatewayGet(ctxPath+"/x", ""); code != http.StatusNotFound {
		t.Fatalf("pre-approval gateway: got %d want 404 (no route yet)", code)
	}

	// 4. admin approves. The pending queue is GLOBAL; a dirty/shared DB may hold
	// other pending subscriptions, so filter to THIS run's subscription by the
	// generated app name (applicationName) rather than blindly taking the last.
	var queue struct {
		Items []struct {
			ID              int64  `json:"id"`
			ApplicationName string `json:"applicationName"`
		} `json:"items"`
	}
	if code := h.api(http.MethodGet, "/api/admin/subscriptions?status=pending", admin, nil, &queue); code != http.StatusOK {
		t.Fatalf("admin queue: got %d want 200", code)
	}
	var subID int64
	for _, q := range queue.Items {
		if q.ApplicationName == appName {
			subID = q.ID
			break
		}
	}
	if subID == 0 {
		t.Fatalf("no pending subscription found for app %q in queue of %d", appName, len(queue.Items))
	}
	if code := h.api(http.MethodPost, "/api/admin/subscriptions/"+itoa(subID)+"/approve", admin, nil, nil); code != http.StatusNoContent {
		t.Fatalf("approve: got %d want 204", code)
	}
	apiKey := h.pollSubStatus(app.ID, dev, "active")

	// allow APISIX route/consumer propagation
	time.Sleep(800 * time.Millisecond)

	// 5. gateway enforcement
	if code := h.gatewayGet(ctxPath+"/x", ""); code != http.StatusUnauthorized {
		t.Fatalf("no key: got %d want 401", code)
	}
	if code := h.gatewayGet(ctxPath+"/x", apiKey); code != http.StatusOK {
		t.Fatalf("with key: got %d want 200", code)
	}
	// plan limit is 2/60s; APISIX runs single-worker (deploy/apisix/config.yaml)
	// so limit-count's local counter is shared and deterministic. Keep calling
	// until the window is exhausted.
	var got429 bool
	for i := 0; i < 10; i++ {
		if h.gatewayGet(ctxPath+"/x", apiKey) == http.StatusTooManyRequests {
			got429 = true
			break
		}
	}
	if !got429 {
		t.Fatal("never observed 429 after exceeding the plan limit")
	}

	// 6. delete-409 while the subscription is still active
	if code := h.api(http.MethodDelete, "/api/admin/products/"+itoa(product.ID), admin, nil, nil); code != http.StatusConflict {
		t.Fatalf("delete product with active sub: got %d want 409", code)
	}

	// 7. unsubscribe → gateway forbids
	if code := h.api(http.MethodDelete, h.appPath(app.ID)+"/subscriptions/"+itoa(product.ID), dev, nil, nil); code != http.StatusNoContent && code != http.StatusOK {
		t.Fatalf("unsubscribe: got %d want 204/200", code)
	}
	time.Sleep(800 * time.Millisecond)
	// last subscriber gone → route deleted → 404 (pinned, not just "not 200")
	if code := h.gatewayGet(ctxPath+"/x", apiKey); code != http.StatusNotFound {
		t.Fatalf("post-unsubscribe gateway: got %d want 404 (route deleted)", code)
	}
}

// TestRejectPath: a rejected subscription never activates and the gateway never
// serves it (the route is never created because approval never happens).
func TestRejectPath(t *testing.T) {
	h := newHarness(t)
	admin := h.adminToken()

	ctxPath := "/" + uniq("e2e")
	var product struct {
		ID int64 `json:"id"`
	}
	if code := h.api(http.MethodPost, "/api/admin/products", admin, map[string]any{
		"name": uniq("Prod"), "slug": uniq("prod"), "category": "Engineering",
		"version": "1.0.0", "contextPath": ctxPath, "description": "",
		"tags": []string{}, "icon": "", "upstreamUrl": "echo:8080", "published": true,
	}, &product); code != http.StatusCreated {
		t.Fatalf("create product: got %d want 201", code)
	}
	t.Cleanup(func() {
		_ = h.gw.DeleteRoute(context.Background(), "prod_"+itoa(product.ID))
		_ = h.api(http.MethodDelete, "/api/admin/products/"+itoa(product.ID), admin, nil, nil)
	})

	var plan struct {
		ID int64 `json:"id"`
	}
	if code := h.api(http.MethodPost, "/api/admin/plans", admin, map[string]any{
		"name": uniq("Plan"), "rateLimit": 5, "windowSeconds": 60, "currency": "USD",
	}, &plan); code != http.StatusCreated {
		t.Fatalf("create plan: got %d want 201", code)
	}
	t.Cleanup(func() { _ = h.api(http.MethodDelete, "/api/admin/plans/"+itoa(plan.ID), admin, nil, nil) })

	dev := h.devToken("rej")
	appName := uniq("App")
	var app struct {
		ID int64 `json:"id"`
	}
	if code := h.api(http.MethodPost, "/api/applications", dev, map[string]any{"name": appName, "description": ""}, &app); code != http.StatusCreated {
		t.Fatalf("create app: got %d want 201", code)
	}
	t.Cleanup(func() { _ = h.gw.DeleteConsumer(context.Background(), "app_"+itoa(app.ID)) })

	if code := h.api(http.MethodPost, h.appPath(app.ID)+"/subscriptions", dev, map[string]any{
		"productId": product.ID, "planId": plan.ID,
	}, nil); code != http.StatusOK && code != http.StatusCreated {
		t.Fatalf("subscribe: got %d want 200/201", code)
	}

	// admin finds THIS run's pending subscription and rejects it
	var queue struct {
		Items []struct {
			ID              int64  `json:"id"`
			ApplicationName string `json:"applicationName"`
		} `json:"items"`
	}
	if code := h.api(http.MethodGet, "/api/admin/subscriptions?status=pending", admin, nil, &queue); code != http.StatusOK {
		t.Fatalf("admin queue: got %d want 200", code)
	}
	var subID int64
	for _, q := range queue.Items {
		if q.ApplicationName == appName {
			subID = q.ID
			break
		}
	}
	if subID == 0 {
		t.Fatalf("no pending subscription found for app %q", appName)
	}
	if code := h.api(http.MethodPost, "/api/admin/subscriptions/"+itoa(subID)+"/reject", admin, nil, nil); code != http.StatusNoContent {
		t.Fatalf("reject: got %d want 204", code)
	}

	// the subscription must report rejected (never active)
	var detail struct {
		APIKey        string `json:"apiKey"`
		Subscriptions []struct {
			Status string `json:"status"`
		} `json:"subscriptions"`
	}
	if code := h.api(http.MethodGet, h.appPath(app.ID), dev, nil, &detail); code != http.StatusOK {
		t.Fatalf("detail: got %d want 200", code)
	}
	for _, s := range detail.Subscriptions {
		if s.Status == "active" {
			t.Fatalf("rejected subscription reported active")
		}
	}

	// the gateway must never serve a rejected subscription: no route was created
	time.Sleep(400 * time.Millisecond)
	if code := h.gatewayGet(ctxPath+"/x", detail.APIKey); code != http.StatusNotFound {
		t.Fatalf("rejected gateway: got %d want 404 (no route)", code)
	}
}
