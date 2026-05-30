package apisix

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestIntegrationProvisionAndCall(t *testing.T) {
	if os.Getenv("RUN_APISIX_IT") != "1" {
		t.Skip("set RUN_APISIX_IT=1 with the compose stack up to run")
	}
	adminURL := envOr("APISIX_ADMIN_URL", "http://localhost:19180")
	adminKey := envOr("APISIX_ADMIN_KEY", "edd1c9f034335f136f87ad84b625c8f1")
	gwURL := envOr("APISIX_GATEWAY_URL", "http://localhost:9080")

	ctx := context.Background()
	c := NewClient(adminURL, adminKey)

	user := fmt.Sprintf("it_app_%d", time.Now().UnixNano())
	key := fmt.Sprintf("itkey-%d", time.Now().UnixNano())
	routeID := "it_route"
	uri := "/itecho/*"

	if err := c.EnsureConsumer(ctx, user, key, RateLimit{Count: 3, WindowSeconds: 60}); err != nil {
		t.Fatalf("EnsureConsumer: %v", err)
	}
	t.Cleanup(func() { _ = c.DeleteConsumer(ctx, user) })
	if err := c.EnsureRoute(ctx, routeID, uri, "echo:8080", []string{user}); err != nil {
		t.Fatalf("EnsureRoute: %v", err)
	}
	t.Cleanup(func() { _ = c.do(ctx, http.MethodDelete, "/apisix/admin/routes/"+routeID, nil) })

	time.Sleep(500 * time.Millisecond)

	if code := call(t, gwURL+"/itecho/x", ""); code != http.StatusUnauthorized {
		t.Fatalf("no key: got %d want 401", code)
	}
	for i := 0; i < 3; i++ {
		if code := call(t, gwURL+"/itecho/x", key); code != http.StatusOK {
			t.Fatalf("call %d: got %d want 200", i+1, code)
		}
	}
	if code := call(t, gwURL+"/itecho/x", key); code != http.StatusTooManyRequests {
		t.Fatalf("over limit: got %d want 429", code)
	}
}

func call(t *testing.T, url, key string) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	if key != "" {
		req.Header.Set("apikey", key)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("call %s: %v", url, err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
