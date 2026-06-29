package apisix

import "context"

// RateLimit is a per-consumer request quota.
type RateLimit struct {
	Count         int
	WindowSeconds int
}

// Gateway is the subset of APISIX provisioning the portal needs.
// Implemented by *Client (real Admin API) and *Fake (tests).
type Gateway interface {
	// EnsureConsumer creates/updates a consumer "username" with a key-auth key and a limit-count.
	EnsureConsumer(ctx context.Context, username, apiKey string, limit RateLimit) error
	// DeleteConsumer removes a consumer.
	DeleteConsumer(ctx context.Context, username string) error
	// EnsureRoute creates/updates the route routeID exposing contextPath (and its
	// subpaths) → upstreamURL, with key-auth and a consumer-restriction whitelist of
	// the given consumer usernames. upstreamURL may be a scheme://host:port URL or a
	// bare host:port (treated as http). The context prefix is stripped before the
	// request reaches the upstream (WSO2-style).
	EnsureRoute(ctx context.Context, routeID, contextPath, upstreamURL string, allowedConsumers []string) error
	// DeleteRoute removes the route routeID. Deleting a missing route is a no-op.
	DeleteRoute(ctx context.Context, routeID string) error
	// EnsureOAuthRoute creates/updates an OAuth2 product route: openid-connect
	// (bearer_only, JWKS from issuer) + a serverless-pre-function whitelisting the
	// token's claimName claim against allowedClientIDs.
	EnsureOAuthRoute(ctx context.Context, routeID, contextPath, upstreamURL, issuer, claimName string, allowedClientIDs []string) error
}
