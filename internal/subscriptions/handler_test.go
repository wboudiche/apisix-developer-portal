package subscriptions

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"apisix-portal/internal/apisix"
	"apisix-portal/internal/auth"
	"apisix-portal/internal/events"
	"apisix-portal/internal/metrics"
)

type fakeReader struct {
	cred    Credential
	has     bool
	subs    []SubscriptionView
	plan    PlanInfo
	planErr error
}

func (f fakeReader) GetCredential(_ context.Context, _ int64) (Credential, error) {
	if !f.has {
		return Credential{}, ErrNotFound
	}
	return f.cred, nil
}
func (f fakeReader) SubscriptionsForApp(_ context.Context, _ int64) ([]SubscriptionView, error) {
	return f.subs, nil
}
func (f fakeReader) ActivePlanForApp(_ context.Context, _ int64) (PlanInfo, error) {
	if f.planErr != nil {
		return PlanInfo{}, f.planErr
	}
	return f.plan, nil
}

// fakeUsageReader implements UsageReader for handler tests.
type fakeUsageReader struct {
	used    int64
	usedErr error
}

func (f fakeUsageReader) Usage(_ context.Context, _ string, _ metrics.Range) (metrics.Usage, error) {
	return metrics.Usage{}, nil
}
func (f fakeUsageReader) RequestsInWindow(_ context.Context, _ string, _ int) (int64, error) {
	if f.usedErr != nil {
		return 0, f.usedErr
	}
	return f.used, nil
}

// fakeEvents returns a fixed feed so the detail endpoint's activity wiring is
// exercised without a database. A non-nil err simulates a feed read failure.
type fakeEvents struct {
	feed []events.View
	err  error
}

func (f fakeEvents) Recent(_ context.Context, _ int64, _ int) ([]events.View, error) {
	return f.feed, f.err
}

func newTestHandler() (*Handler, *apisix.Fake) {
	store := newMemStore()
	gw := apisix.NewFake()
	svc := NewService(store, gw, func() string { return "key-xyz" }, nil)
	owns := func(_ context.Context, appID, userID int64) (bool, error) { return appID == 1 && userID == 5, nil }
	reader := fakeReader{has: true, cred: Credential{ApplicationID: 1, APIKey: "key-xyz", ConsumerUsername: "app_1"},
		subs: []SubscriptionView{{ProductID: 3, ProductName: "PizzaShackAPI", PlanID: 2, PlanName: "Silver"}}}
	return NewHandler(svc, reader, fakeEvents{}, owns), gw
}

func TestSubscribeEndpointReturnsKeyWithoutProvisioning(t *testing.T) {
	h, gw := newTestHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/applications/1/subscriptions", strings.NewReader(`{"productId":3,"planId":2}`))
	req = req.WithContext(auth.WithUserID(req.Context(), 5))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"apiKey":"key-xyz"`) {
		t.Fatalf("missing api key in body: %s", rec.Body)
	}
	if len(gw.Consumers) != 0 {
		t.Fatalf("subscribe must not provision a consumer (pending), got %v", gw.Consumers)
	}
}

func TestSubscribeEndpointRejectsNonOwner(t *testing.T) {
	h, _ := newTestHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/applications/1/subscriptions", strings.NewReader(`{"productId":3,"planId":2}`))
	req = req.WithContext(auth.WithUserID(req.Context(), 999))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d want 403", rec.Code)
	}
}

func TestSubscribeEndpointValidatesBody(t *testing.T) {
	h, _ := newTestHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/applications/1/subscriptions", strings.NewReader(`{"productId":3}`))
	req = req.WithContext(auth.WithUserID(req.Context(), 5))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 (missing planId)", rec.Code)
	}
}

func TestUnsubscribeEndpoint(t *testing.T) {
	h, _ := newTestHandler()
	// first subscribe
	sub := httptest.NewRequest(http.MethodPost, "/api/applications/1/subscriptions", strings.NewReader(`{"productId":3,"planId":2}`))
	h.ServeHTTP(httptest.NewRecorder(), sub.WithContext(auth.WithUserID(sub.Context(), 5)))
	// then unsubscribe
	del := httptest.NewRequest(http.MethodDelete, "/api/applications/1/subscriptions/3", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, del.WithContext(auth.WithUserID(del.Context(), 5)))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status=%d want 204", rec.Code)
	}
}

func TestAppDetailReturnsKeyAndSubscriptions(t *testing.T) {
	h, _ := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/applications/1", nil)
	req = req.WithContext(auth.WithUserID(req.Context(), 5))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"apiKey":"key-xyz"`) || !strings.Contains(body, `"productName":"PizzaShackAPI"`) {
		t.Fatalf("unexpected detail body: %s", body)
	}
}

func TestAppDetailIncludesActivityFeed(t *testing.T) {
	store := newMemStore()
	gw := apisix.NewFake()
	svc := NewService(store, gw, func() string { return "k" }, nil)
	owns := func(_ context.Context, appID, userID int64) (bool, error) { return appID == 1 && userID == 5, nil }
	reader := fakeReader{has: true, cred: Credential{ApplicationID: 1}}
	feed := fakeEvents{feed: []events.View{{Kind: events.KindSubscribed, ProductName: "Inventory API", PlanName: "Gold"}}}
	h := NewHandler(svc, reader, feed, owns)

	req := httptest.NewRequest(http.MethodGet, "/api/applications/1", nil)
	req = req.WithContext(auth.WithUserID(req.Context(), 5))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"events":[`) || !strings.Contains(body, `"kind":"subscribed"`) ||
		!strings.Contains(body, `"productName":"Inventory API"`) || !strings.Contains(body, `"planName":"Gold"`) {
		t.Fatalf("detail body must carry the activity feed: %s", body)
	}
}

func TestAppDetailSurvivesFeedReadError(t *testing.T) {
	store := newMemStore()
	gw := apisix.NewFake()
	svc := NewService(store, gw, func() string { return "k" }, nil)
	owns := func(_ context.Context, appID, userID int64) (bool, error) { return appID == 1 && userID == 5, nil }
	reader := fakeReader{has: true, cred: Credential{ApplicationID: 1, APIKey: "key-xyz"}}
	// Feed read fails — the page must still load (200) with an empty feed, not 500.
	h := NewHandler(svc, reader, fakeEvents{err: errors.New("db down")}, owns)

	req := httptest.NewRequest(http.MethodGet, "/api/applications/1", nil)
	req = req.WithContext(auth.WithUserID(req.Context(), 5))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("feed read error must not fail the detail page; got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"apiKey":"key-xyz"`) || !strings.Contains(body, `"events":[]`) {
		t.Fatalf("detail must still serve with an empty feed: %s", body)
	}
}

func TestAppDetailRejectsNonOwner(t *testing.T) {
	h, _ := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/applications/1", nil)
	req = req.WithContext(auth.WithUserID(req.Context(), 999))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d want 403", rec.Code)
	}
}

func newSeededTestHandler() (*Handler, *apisix.Fake) {
	store := newMemStore()
	// Seed app 1 with a credential and an active subscription so RotateKey succeeds.
	store.creds[1] = Credential{ApplicationID: 1, APIKey: "old-key", ConsumerUsername: "app_1"}
	store.nextID = 1
	store.records[1] = &SubscriptionRecord{ID: 1, AppID: 1, ProductID: 3, PlanID: 2, Status: StatusActive}
	gw := apisix.NewFake()
	svc := NewService(store, gw, func() string { return "rotated-key" }, nil)
	owns := func(_ context.Context, appID, userID int64) (bool, error) { return appID == 1 && userID == 5, nil }
	reader := fakeReader{has: true, cred: Credential{ApplicationID: 1, APIKey: "old-key", ConsumerUsername: "app_1"}}
	return NewHandler(svc, reader, fakeEvents{}, owns), gw
}

func TestRotateKeyEndpoint(t *testing.T) {
	h, gw := newSeededTestHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/applications/1/credentials/rotate", nil)
	req = req.WithContext(auth.WithUserID(req.Context(), 5))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		APIKey string `json:"apiKey"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if out.APIKey != "rotated-key" {
		t.Errorf("expected apiKey=%q, got %q", "rotated-key", out.APIKey)
	}
	if c, ok := gw.Consumers["app_1"]; !ok || c.APIKey != "rotated-key" {
		t.Errorf("gateway consumer key = %q, want rotated-key", gw.Consumers["app_1"].APIKey)
	}
}

func TestRotateKeyEndpoint_NonOwner403(t *testing.T) {
	h, _ := newSeededTestHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/applications/1/credentials/rotate", nil)
	req = req.WithContext(auth.WithUserID(req.Context(), 9))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
		t.Fatalf("non-owner status=%d (want 403/404)", rec.Code)
	}
}

func TestQuotaHappyPath(t *testing.T) {
	store := newMemStore()
	gw := apisix.NewFake()
	svc := NewService(store, gw, func() string { return "key-xyz" }, nil)
	owns := func(_ context.Context, appID, userID int64) (bool, error) { return appID == 1 && userID == 5, nil }
	reader := fakeReader{
		has:  true,
		cred: Credential{ApplicationID: 1, APIKey: "key-xyz", ConsumerUsername: "app_1"},
		subs: []SubscriptionView{{ProductID: 3, ProductName: "PizzaShackAPI", PlanID: 2, PlanName: "Silver"}},
		plan: PlanInfo{Count: 1000, WindowSeconds: 60},
	}
	h := NewHandler(svc, reader, fakeEvents{}, owns)
	h.SetUsageReader(fakeUsageReader{used: 612})

	req := httptest.NewRequest(http.MethodGet, "/api/applications/1/quota", nil)
	req = req.WithContext(auth.WithUserID(req.Context(), 5))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}
	var q Quota
	_ = json.Unmarshal(rec.Body.Bytes(), &q)
	if !q.HasQuota || !q.Available || q.Used != 612 || q.Limit != 1000 || q.WindowSeconds != 60 {
		t.Fatalf("quota = %+v", q)
	}
}

func TestQuotaNoActiveSubscription(t *testing.T) {
	store := newMemStore()
	gw := apisix.NewFake()
	svc := NewService(store, gw, func() string { return "key-xyz" }, nil)
	owns := func(_ context.Context, appID, userID int64) (bool, error) { return appID == 1 && userID == 5, nil }
	reader := fakeReader{
		has:     true,
		cred:    Credential{ApplicationID: 1, APIKey: "key-xyz", ConsumerUsername: "app_1"},
		planErr: ErrNoActiveSubscription,
	}
	h := NewHandler(svc, reader, fakeEvents{}, owns)

	req := httptest.NewRequest(http.MethodGet, "/api/applications/1/quota", nil)
	req = req.WithContext(auth.WithUserID(req.Context(), 5))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}
	var q Quota
	_ = json.Unmarshal(rec.Body.Bytes(), &q)
	if q.HasQuota {
		t.Fatalf("expected hasQuota=false, got %+v", q)
	}
}

func TestQuotaMetricsUnavailable(t *testing.T) {
	store := newMemStore()
	gw := apisix.NewFake()
	svc := NewService(store, gw, func() string { return "key-xyz" }, nil)
	owns := func(_ context.Context, appID, userID int64) (bool, error) { return appID == 1 && userID == 5, nil }
	reader := fakeReader{
		has:  true,
		cred: Credential{ApplicationID: 1, APIKey: "key-xyz", ConsumerUsername: "app_1"},
		plan: PlanInfo{Count: 1000, WindowSeconds: 60},
	}
	// Do NOT set a usage reader — h.usage stays nil
	h := NewHandler(svc, reader, fakeEvents{}, owns)

	req := httptest.NewRequest(http.MethodGet, "/api/applications/1/quota", nil)
	req = req.WithContext(auth.WithUserID(req.Context(), 5))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}
	var q Quota
	_ = json.Unmarshal(rec.Body.Bytes(), &q)
	if !q.HasQuota || q.Available || q.Limit != 1000 || q.WindowSeconds != 60 {
		t.Fatalf("expected hasQuota=true, available=false, limit=1000, windowSeconds=60; got %+v", q)
	}
}

func TestQuotaNonOwner403(t *testing.T) {
	h, _ := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/applications/1/quota", nil)
	req = req.WithContext(auth.WithUserID(req.Context(), 9))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
		t.Fatalf("non-owner status=%d (want 403/404)", rec.Code)
	}
}
