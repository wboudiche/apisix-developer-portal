package apisix

import (
	"context"
	"errors"
	"sync"
)

type FakeConsumer struct {
	APIKey string
	Limit  RateLimit
}
type FakeRoute struct {
	URI      string
	Upstream string
	Allowed  []string
	OAuth    bool // true for EnsureOAuthRoute, false for EnsureRoute
}

// Fake is an in-memory Gateway for unit tests.
type Fake struct {
	mu                 sync.Mutex
	Consumers          map[string]FakeConsumer
	Routes             map[string]FakeRoute
	FailEnsureConsumer bool
}

func NewFake() *Fake {
	return &Fake{Consumers: map[string]FakeConsumer{}, Routes: map[string]FakeRoute{}}
}

func (f *Fake) EnsureConsumer(_ context.Context, username, apiKey string, limit RateLimit) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.FailEnsureConsumer {
		return errors.New("fake: ensure consumer failed")
	}
	f.Consumers[username] = FakeConsumer{APIKey: apiKey, Limit: limit}
	return nil
}
func (f *Fake) DeleteConsumer(_ context.Context, username string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.Consumers, username)
	return nil
}
func (f *Fake) EnsureRoute(_ context.Context, routeID, contextPath, upstreamURL string, allowed []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Routes[routeID] = FakeRoute{URI: contextPath, Upstream: upstreamURL, Allowed: append([]string(nil), allowed...), OAuth: false}
	return nil
}

func (f *Fake) EnsureOAuthRoute(_ context.Context, routeID, contextPath, upstreamURL, issuer, claimName string, allowed []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Routes[routeID] = FakeRoute{URI: contextPath, Upstream: upstreamURL, Allowed: append([]string(nil), allowed...), OAuth: true}
	return nil
}

func (f *Fake) DeleteRoute(_ context.Context, routeID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.Routes, routeID)
	return nil
}

var _ Gateway = (*Fake)(nil)
