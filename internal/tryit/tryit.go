// Package tryit proxies a developer's "Try-it" request through the portal into
// the APISIX gateway, injecting the approved subscriber's API key server-side.
package tryit

import (
	"context"
	"errors"
)

// ErrNotFound is returned by Products when a published product is missing.
var ErrNotFound = errors.New("tryit: product not found")

// AppRef is an application the user may use for Try-it.
type AppRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// Products resolves a product slug to its id and gateway context path. Only
// PUBLISHED products resolve; others yield ErrNotFound.
type Products interface {
	ProductBySlug(ctx context.Context, slug string) (id int64, contextPath string, err error)
	SandboxUpstream(ctx context.Context, slug string) (bool, error)
}

// Access answers the authorization + key questions for Try-it.
type Access interface {
	OwnsApp(ctx context.Context, appID, userID int64) (bool, error)
	SubscriptionStatus(ctx context.Context, appID, productID int64) (string, error)
	APIKey(ctx context.Context, appID int64) (string, error)
	ApprovedApps(ctx context.Context, userID, productID int64) ([]AppRef, error)
	SandboxKey(ctx context.Context, appID int64) (string, error)
}
