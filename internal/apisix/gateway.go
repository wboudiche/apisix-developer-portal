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
	// EnsureRoute creates/updates the route routeID for uri→upstream with key-auth and a
	// consumer-restriction whitelist of the given consumer usernames.
	EnsureRoute(ctx context.Context, routeID, uri, upstream string, allowedConsumers []string) error
	// DeleteRoute removes the route routeID. Deleting a missing route is a no-op.
	DeleteRoute(ctx context.Context, routeID string) error
}
