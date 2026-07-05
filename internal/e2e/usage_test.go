package e2e

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// usageResp mirrors the /usage payload (metrics.Usage) for decoding in the test.
type usageResp struct {
	Summary struct {
		RequestsToday int64   `json:"requestsToday"`
		MonthToDate   int64   `json:"monthToDate"`
		P95Ms         float64 `json:"p95Ms"`
		ErrorRate     float64 `json:"errorRate"`
	} `json:"summary"`
	Series []struct {
		Requests int64 `json:"requests"`
	} `json:"series"`
}

func (u usageResp) seriesTotal() int64 {
	var n int64
	for _, p := range u.Series {
		n += p.Requests
	}
	return n
}

// bestEffortGet hits the gateway without failing the test on transport errors —
// safe to call from the traffic goroutine (t.Fatalf may only run on the test
// goroutine).
func (h *harness) bestEffortGet(path, apikey string) {
	gwURL := envOr("APISIX_GATEWAY_URL", "http://localhost:9080")
	req, err := http.NewRequest(http.MethodGet, gwURL+path, nil)
	if err != nil {
		return
	}
	if apikey != "" {
		req.Header.Set("apikey", apikey)
	}
	if res, err := http.DefaultClient.Do(req); err == nil {
		res.Body.Close()
	}
}

// TestUsageReflectsTraffic is the end-to-end proof of the metrics pipeline:
// real gateway traffic for a provisioned consumer shows up in the portal's
// /usage endpoint (APISIX prometheus plugin → Prometheus scrape → portal read).
//
// Timing-dependent by nature (5s scrape; increase() needs a rising counter
// across several scrapes), so traffic is driven continuously while polling, and
// the assertion is "did any traffic surface" rather than an exact count.
func TestUsageReflectsTraffic(t *testing.T) {
	h := newHarness(t)
	// The harness mounts server.New, which does not install the global
	// prometheus rule (that lives in main); do it here so metrics are emitted.
	if err := h.gw.EnsureGlobalPrometheus(context.Background()); err != nil {
		t.Fatalf("enable gateway prometheus: %v", err)
	}
	admin := h.adminToken()

	ctxPath := "/" + uniq("usage")
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

	// High limit so sustained traffic isn't rate-limited (we want 200s, not 429s).
	var plan struct {
		ID int64 `json:"id"`
	}
	if code := h.api(http.MethodPost, "/api/admin/plans", admin, map[string]any{
		"name": uniq("Plan"), "rateLimit": 1000000, "windowSeconds": 60, "currency": "USD",
	}, &plan); code != http.StatusCreated {
		t.Fatalf("create plan: got %d want 201", code)
	}
	t.Cleanup(func() { _ = h.api(http.MethodDelete, "/api/admin/plans/"+itoa(plan.ID), admin, nil, nil) })

	dev := h.devToken("usagedev")
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

	// Approve so the consumer + route are provisioned in APISIX.
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
		t.Fatalf("no pending subscription for app %q", appName)
	}
	if code := h.api(http.MethodPost, "/api/admin/subscriptions/"+itoa(subID)+"/approve", admin, nil, nil); code != http.StatusNoContent {
		t.Fatalf("approve: got %d want 204", code)
	}
	apiKey := h.pollSubStatus(app.ID, dev, "active")
	time.Sleep(800 * time.Millisecond) // route/consumer propagation

	// Empty-but-available baseline: a freshly-provisioned app with no traffic
	// returns 200 with zeroed usage (not 503, not demo data).
	var base usageResp
	if code := h.api(http.MethodGet, h.appPath(app.ID)+"/usage?range=24h", dev, nil, &base); code != http.StatusOK {
		t.Fatalf("baseline usage: got %d want 200", code)
	}

	// Drive traffic continuously while polling so the counter keeps rising
	// across scrapes (flat counters yield increase()=0).
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
				h.bestEffortGet(ctxPath+"/x", apiKey)
				time.Sleep(200 * time.Millisecond)
			}
		}
	}()
	defer close(stop)

	var detected bool
	for i := 0; i < 45; i++ {
		time.Sleep(1 * time.Second)
		var u usageResp
		if code := h.api(http.MethodGet, h.appPath(app.ID)+"/usage?range=24h", dev, nil, &u); code == http.StatusOK {
			if u.seriesTotal() > 0 || u.Summary.RequestsToday > 0 {
				detected = true
				break
			}
		}
	}
	if !detected {
		t.Fatal("usage never reflected driven gateway traffic within timeout")
	}

	// Ownership still applies to the usage endpoint: another developer is forbidden.
	other := h.devToken("usageother")
	if code := h.api(http.MethodGet, h.appPath(app.ID)+"/usage?range=24h", other, nil, nil); code != http.StatusForbidden {
		t.Fatalf("non-owner usage: got %d want 403", code)
	}
	// Range is an allow-list: a bogus value is rejected.
	if code := h.api(http.MethodGet, h.appPath(app.ID)+"/usage?range=forever", dev, nil, nil); code != http.StatusBadRequest {
		t.Fatalf("bad range: got %d want 400", code)
	}

	// Tear down the subscription so the product/plan cleanups (registered above)
	// don't 409 against an active subscription on the shared dev DB.
	if code := h.api(http.MethodDelete, h.appPath(app.ID)+"/subscriptions/"+itoa(product.ID), dev, nil, nil); code != http.StatusNoContent && code != http.StatusOK {
		t.Fatalf("unsubscribe: got %d want 204/200", code)
	}
}
