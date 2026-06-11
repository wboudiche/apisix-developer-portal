package subscriptions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"apisix-portal/internal/apisix"
	"apisix-portal/internal/auth"
	"apisix-portal/internal/events"
)

type fakeReader struct {
	cred Credential
	has  bool
	subs []SubscriptionView
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

// fakeEvents returns a fixed feed so the detail endpoint's activity wiring is
// exercised without a database.
type fakeEvents struct{ feed []events.View }

func (f fakeEvents) Recent(_ context.Context, _ int64, _ int) ([]events.View, error) {
	return f.feed, nil
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
