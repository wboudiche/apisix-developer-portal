package subscriptions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"apisix-portal/internal/apisix"
	"apisix-portal/internal/auth"
)

func newTestHandler() (*Handler, *apisix.Fake) {
	store := newMemStore()
	gw := apisix.NewFake()
	svc := NewService(store, gw, func() string { return "key-xyz" })
	owns := func(_ context.Context, appID, userID int64) (bool, error) { return appID == 1 && userID == 5, nil }
	return NewHandler(svc, owns), gw
}

func TestSubscribeEndpointProvisionsAndReturnsKey(t *testing.T) {
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
	if _, ok := gw.Consumers["app_1"]; !ok {
		t.Fatal("consumer not provisioned")
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
