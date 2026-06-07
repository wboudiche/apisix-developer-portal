package e2e

import (
	"net/http"
	"testing"
)

func TestAuthzNegatives(t *testing.T) {
	h := newHarness(t)

	// developer A creates an application
	devA := h.devToken("devA")
	var appA struct {
		ID int64 `json:"id"`
	}
	if code := h.api(http.MethodPost, "/api/applications", devA, map[string]any{"name": uniq("AppA"), "description": ""}, &appA); code != http.StatusCreated {
		t.Fatalf("A create app: got %d want 201", code)
	}
	// capture A's apiKey via detail (no subscription yet → apiKey may be empty,
	// but the detail must still be readable by A and NOT by B)
	devB := h.devToken("devB")

	t.Run("cross-tenant read is not found", func(t *testing.T) {
		// The ownership check returns 403 ("not your application") rather than
		// 404, which is the real server behavior. Both 403 and 404 are secure
		// (neither leaks 200). Accept both.
		var body map[string]any
		code := h.api(http.MethodGet, h.appPath(appA.ID), devB, nil, &body)
		if code == http.StatusOK {
			t.Fatalf("B read A's app: got 200, want 403/404; leaked body=%v", body)
		}
		if code != http.StatusNotFound && code != http.StatusForbidden {
			t.Fatalf("B read A's app: got %d want 403/404", code)
		}
	})

	t.Run("cross-tenant subscribe is rejected", func(t *testing.T) {
		code := h.api(http.MethodPost, h.appPath(appA.ID)+"/subscriptions", devB, map[string]any{"productId": 1, "planId": 1}, nil)
		if code == http.StatusOK || code == http.StatusCreated {
			t.Fatalf("B subscribe on A's app: got %d, want 403/404", code)
		}
		if code != http.StatusForbidden && code != http.StatusNotFound {
			t.Fatalf("B subscribe on A's app: got %d want 403/404", code)
		}
	})

	t.Run("cross-tenant unsubscribe is rejected", func(t *testing.T) {
		code := h.api(http.MethodDelete, h.appPath(appA.ID)+"/subscriptions/1", devB, nil, nil)
		if code == http.StatusOK || code == http.StatusNoContent {
			t.Fatalf("B unsubscribe on A's app: got %d, want 403/404", code)
		}
	})

	t.Run("non-admin is blocked from admin endpoints", func(t *testing.T) {
		paths := []struct {
			method, path string
			body         any
		}{
			{http.MethodPost, "/api/admin/products", map[string]any{"name": "x", "slug": "x", "category": "x", "contextPath": "/x"}},
			{http.MethodGet, "/api/admin/subscriptions", nil},
			{http.MethodPost, "/api/admin/subscriptions/1/approve", nil},
		}
		for _, p := range paths {
			if code := h.api(p.method, p.path, devA, p.body, nil); code != http.StatusForbidden {
				t.Fatalf("non-admin %s %s: got %d want 403", p.method, p.path, code)
			}
		}
	})

	t.Run("no token is unauthorized", func(t *testing.T) {
		for _, path := range []string{"/api/admin/products", "/api/admin/subscriptions"} {
			if code := h.api(http.MethodGet, path, "", nil, nil); code != http.StatusUnauthorized {
				t.Fatalf("no-token %s: got %d want 401", path, code)
			}
		}
		// applications also require auth
		if code := h.api(http.MethodGet, "/api/applications", "", nil, nil); code != http.StatusUnauthorized {
			t.Fatalf("no-token list apps: got %d want 401", code)
		}
	})
}
