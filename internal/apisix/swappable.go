package apisix

import (
	"context"
	"errors"
	"sync/atomic"
)

// ErrGatewayDisabled is returned by a SwappableGateway with no inner client
// (e.g. sandbox URLs unset at runtime).
var ErrGatewayDisabled = errors.New("apisix: gateway not configured")

// SwappableGateway lets runtime settings replace the underlying Admin API
// client without rewiring consumers: it implements Gateway and delegates to
// the current inner, which Swap replaces atomically.
type SwappableGateway struct {
	inner atomic.Pointer[gatewayBox]
}

type gatewayBox struct{ gw Gateway } // box so a nil Gateway is representable

func NewSwappable(inner Gateway) *SwappableGateway {
	s := &SwappableGateway{}
	s.Swap(inner)
	return s
}

func (s *SwappableGateway) Swap(inner Gateway) { s.inner.Store(&gatewayBox{gw: inner}) }
func (s *SwappableGateway) Enabled() bool      { return s.inner.Load().gw != nil }

func (s *SwappableGateway) get() (Gateway, error) {
	if gw := s.inner.Load().gw; gw != nil {
		return gw, nil
	}
	return nil, ErrGatewayDisabled
}

func (s *SwappableGateway) EnsureConsumer(ctx context.Context, username, apiKey string, limit RateLimit) error {
	gw, err := s.get()
	if err != nil {
		return err
	}
	return gw.EnsureConsumer(ctx, username, apiKey, limit)
}

func (s *SwappableGateway) DeleteConsumer(ctx context.Context, username string) error {
	gw, err := s.get()
	if err != nil {
		return err
	}
	return gw.DeleteConsumer(ctx, username)
}

func (s *SwappableGateway) EnsureRoute(ctx context.Context, routeID, contextPath, upstreamURL string, allowedConsumers []string) error {
	gw, err := s.get()
	if err != nil {
		return err
	}
	return gw.EnsureRoute(ctx, routeID, contextPath, upstreamURL, allowedConsumers)
}

func (s *SwappableGateway) DeleteRoute(ctx context.Context, routeID string) error {
	gw, err := s.get()
	if err != nil {
		return err
	}
	return gw.DeleteRoute(ctx, routeID)
}

func (s *SwappableGateway) EnsureOAuthRoute(ctx context.Context, routeID, contextPath, upstreamURL, issuer, claimName string, allowedClientIDs []string) error {
	gw, err := s.get()
	if err != nil {
		return err
	}
	return gw.EnsureOAuthRoute(ctx, routeID, contextPath, upstreamURL, issuer, claimName, allowedClientIDs)
}

var _ Gateway = (*SwappableGateway)(nil)
