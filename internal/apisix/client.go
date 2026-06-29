package apisix

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
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

func (c *Client) EnsureRoute(ctx context.Context, routeID, contextPath, upstreamURL string, allowed []string) error {
	body, err := routeBody(contextPath, upstreamURL, allowed)
	if err != nil {
		return err
	}
	return c.do(ctx, http.MethodPut, "/apisix/admin/routes/"+routeID, body)
}

// routeBody builds the APISIX route definition for a product. It exposes both
// the context root and its subpaths ("/headers" and "/headers/*"), reaches the
// upstream over the scheme parsed from upstreamURL (a schemeless host:port
// defaults to http, preserving the legacy echo demo), and strips the context
// prefix via proxy-rewrite so /headers/get reaches the backend at /get — the
// same routing behavior as WSO2 API Manager.
func routeBody(contextPath, upstreamURL string, allowed []string) (map[string]any, error) {
	scheme, node, err := parseUpstream(upstreamURL)
	if err != nil {
		return nil, err
	}
	if allowed == nil {
		allowed = []string{}
	}
	prefix := regexp.QuoteMeta(strings.TrimRight(contextPath, "/"))
	return map[string]any{
		"uris": []string{contextPath, contextPath + "/*"},
		"upstream": map[string]any{
			"type":   "roundrobin",
			"scheme": scheme,
			"nodes":  map[string]int{node: 1},
		},
		"plugins": map[string]any{
			"key-auth":             map[string]any{},
			"consumer-restriction": map[string]any{"type": "consumer_name", "whitelist": allowed},
			"proxy-rewrite":        map[string]any{"regex_uri": []string{"^" + prefix + "/?(.*)$", "/$1"}},
		},
	}, nil
}

// parseUpstream returns the upstream scheme and host:port node. It accepts a
// full URL (https://host[:port]) or a bare host:port (treated as http for
// backward compatibility). A missing port defaults from the scheme.
func parseUpstream(upstreamURL string) (scheme, node string, err error) {
	upstreamURL = strings.TrimSpace(upstreamURL)
	scheme = "http"
	hostPort := upstreamURL
	if strings.Contains(upstreamURL, "://") {
		u, perr := url.Parse(upstreamURL)
		if perr != nil || u.Host == "" {
			return "", "", fmt.Errorf("bad upstream url %q", upstreamURL)
		}
		scheme = u.Scheme
		hostPort = u.Host
	}
	host, portStr, ok := strings.Cut(hostPort, ":")
	if !ok {
		port := "80"
		if scheme == "https" {
			port = "443"
		}
		host, portStr = hostPort, port
	}
	if host == "" {
		return "", "", fmt.Errorf("upstream must include a host, got %q", upstreamURL)
	}
	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
		return "", "", fmt.Errorf("bad upstream port %q: %w", portStr, err)
	}
	return scheme, fmt.Sprintf("%s:%d", host, port), nil
}

var clientIDRe = regexp.MustCompile(`^[A-Za-z0-9._:@-]{1,200}$`)

// ValidClientID guards every OIDC client id before it is embedded (as a Lua
// table key) in a route's serverless-pre-function. Strict charset = no Lua
// injection is possible from a client id.
func ValidClientID(s string) bool { return clientIDRe.MatchString(s) }

func (c *Client) EnsureOAuthRoute(ctx context.Context, routeID, contextPath, upstreamURL, issuer, claimName string, allowedClientIDs []string) error {
	body, err := oauthRouteBody(contextPath, upstreamURL, issuer, claimName, allowedClientIDs)
	if err != nil {
		return err
	}
	return c.do(ctx, http.MethodPut, "/apisix/admin/routes/"+routeID, body)
}

// oauthRouteBody builds an OAuth2 product route: openid-connect validates the
// bearer JWT against the issuer's JWKS (bearer_only); a serverless-pre-function
// then 403s unless the token's claimName claim is in the allow-list of the
// product's active subscribers' client ids. Same context-prefix strip as routeBody.
func oauthRouteBody(contextPath, upstreamURL, issuer, claimName string, allowed []string) (map[string]any, error) {
	scheme, node, err := parseUpstream(upstreamURL)
	if err != nil {
		return nil, err
	}
	if !clientIDRe.MatchString(claimName) { // claim name is config-controlled; guard it too
		return nil, fmt.Errorf("bad oidc claim name %q", claimName)
	}
	var b strings.Builder
	for _, cid := range allowed {
		if !ValidClientID(cid) {
			return nil, fmt.Errorf("bad client id %q", cid)
		}
		fmt.Fprintf(&b, "[%q]=true,", cid)
	}
	lua := `return function(conf, ctx)
  local core = require("apisix.core")
  local hdr = core.request.header(ctx, "Authorization")
  if not hdr then return end
  local tok = hdr:match("[Bb]earer%s+(.+)")
  if not tok then return end
  local payload = tok:match("^[^.]+%.([^.]+)")
  if not payload then return 403, {message="forbidden"} end
  payload = payload:gsub("-","+"):gsub("_","/")
  local pad = #payload % 4
  if pad > 0 then payload = payload .. string.rep("=", 4 - pad) end
  local raw = ngx.decode_base64(payload)
  if not raw then return 403, {message="forbidden"} end
  local claims = core.json.decode(raw)
  if not claims then return 403, {message="forbidden"} end
  local allow = {` + b.String() + `}
  local cid = claims["` + claimName + `"]
  if not cid or not allow[cid] then return 403, {message="not subscribed"} end
end`
	prefix := regexp.QuoteMeta(strings.TrimRight(contextPath, "/"))
	return map[string]any{
		"uris": []string{contextPath, contextPath + "/*"},
		"upstream": map[string]any{
			"type": "roundrobin", "scheme": scheme, "nodes": map[string]int{node: 1},
		},
		"plugins": map[string]any{
			"openid-connect": map[string]any{
				"bearer_only": true,
				"discovery":   strings.TrimRight(issuer, "/") + "/.well-known/openid-configuration",
				"use_jwks":    true,
			},
			"serverless-pre-function": map[string]any{
				"phase":     "access",
				"functions": []string{lua},
			},
			"proxy-rewrite": map[string]any{"regex_uri": []string{"^" + prefix + "/?(.*)$", "/$1"}},
		},
	}, nil
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
