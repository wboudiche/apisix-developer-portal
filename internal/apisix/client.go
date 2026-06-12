package apisix

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client talks to the APISIX Admin API.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) do(ctx context.Context, method, path string, body any) error {
	var buf io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		buf = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, buf)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-KEY", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("apisix admin %s %s: %d %s", method, path, resp.StatusCode, string(msg))
	}
	return nil
}

func (c *Client) EnsureConsumer(ctx context.Context, username, apiKey string, limit RateLimit) error {
	body := map[string]any{
		"username": username,
		"plugins": map[string]any{
			"key-auth": map[string]any{"key": apiKey},
			"limit-count": map[string]any{
				"count": limit.Count, "time_window": limit.WindowSeconds,
				"rejected_code": 429, "key_type": "var", "key": "consumer_name", "policy": "local",
			},
		},
	}
	return c.do(ctx, http.MethodPut, "/apisix/admin/consumers", body)
}

func (c *Client) DeleteConsumer(ctx context.Context, username string) error {
	return c.do(ctx, http.MethodDelete, "/apisix/admin/consumers/"+username, nil)
}

func (c *Client) EnsureRoute(ctx context.Context, routeID, uri, upstream string, allowed []string) error {
	host, portStr, ok := strings.Cut(upstream, ":")
	if !ok {
		return fmt.Errorf("upstream must be host:port, got %q", upstream)
	}
	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
		return fmt.Errorf("bad upstream port %q: %w", portStr, err)
	}
	if allowed == nil {
		allowed = []string{}
	}
	body := map[string]any{
		"uri": uri,
		"upstream": map[string]any{
			"type":  "roundrobin",
			"nodes": map[string]int{fmt.Sprintf("%s:%d", host, port): 1},
		},
		"plugins": map[string]any{
			"key-auth":             map[string]any{},
			"consumer-restriction": map[string]any{"type": "consumer_name", "whitelist": allowed},
		},
	}
	return c.do(ctx, http.MethodPut, "/apisix/admin/routes/"+routeID, body)
}

func (c *Client) DeleteRoute(ctx context.Context, routeID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+"/apisix/admin/routes/"+routeID, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-KEY", c.apiKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// 404 means the route is already gone — treat as success (idempotent delete).
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusNotFound {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("apisix delete route %s: %d %s", routeID, resp.StatusCode, string(msg))
	}
	return nil
}

// EnsureGlobalPrometheus installs a global rule that enables the prometheus
// plugin on every route, so the gateway emits per-route/per-consumer request
// metrics for the portal's KPI cards. Idempotent (PUT with a fixed id); called
// best-effort at startup. The metrics are scraped by Prometheus from
// apisix:9091 (see deploy/apisix/config.yaml + deploy/prometheus).
func (c *Client) EnsureGlobalPrometheus(ctx context.Context) error {
	body := map[string]any{"plugins": map[string]any{"prometheus": map[string]any{}}}
	return c.do(ctx, http.MethodPut, "/apisix/admin/global_rules/portal-prometheus", body)
}

var _ Gateway = (*Client)(nil)
