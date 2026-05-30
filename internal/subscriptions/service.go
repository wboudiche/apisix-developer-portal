package subscriptions

import (
	"context"
	"fmt"

	"apisix-portal/internal/apisix"
)

// ProductInfo is what the service needs to provision a product's gateway route.
type ProductInfo struct {
	ID          int64
	ContextPath string
	Upstream    string // host:port
}

// PlanInfo is the rate limit for a subscription.
type PlanInfo struct {
	ID            int64
	Count         int
	WindowSeconds int
}

// Credential is an application's gateway identity.
type Credential struct {
	ApplicationID    int64  `json:"applicationId"`
	APIKey           string `json:"apiKey"`
	ConsumerUsername string `json:"consumerUsername"`
}

// Store persists credentials/subscriptions and answers provisioning queries.
type Store interface {
	GetOrCreateCredential(ctx context.Context, appID int64, genKey func() string) (Credential, error)
	GetProduct(ctx context.Context, productID int64) (ProductInfo, error)
	GetPlan(ctx context.Context, planID int64) (PlanInfo, error)
	SaveSubscription(ctx context.Context, appID, productID, planID int64) error
	DeleteSubscription(ctx context.Context, appID, productID int64) error
	// ConsumersForProduct returns the consumer usernames of every application
	// currently subscribed to the product (used to rebuild the route whitelist).
	ConsumersForProduct(ctx context.Context, productID int64) ([]string, error)
}

func consumerName(appID int64) string { return fmt.Sprintf("app_%d", appID) }
func routeID(productID int64) string  { return fmt.Sprintf("prod_%d", productID) }

// Service orchestrates subscribe/unsubscribe and the matching APISIX provisioning.
type Service struct {
	store  Store
	gw     apisix.Gateway
	genKey func() string
}

func NewService(store Store, gw apisix.Gateway, genKey func() string) *Service {
	return &Service{store: store, gw: gw, genKey: genKey}
}

// Subscribe provisions APISIX and persists the subscription, returning the app's credential.
func (s *Service) Subscribe(ctx context.Context, appID, productID, planID int64) (Credential, error) {
	prod, err := s.store.GetProduct(ctx, productID)
	if err != nil {
		return Credential{}, err
	}
	plan, err := s.store.GetPlan(ctx, planID)
	if err != nil {
		return Credential{}, err
	}
	cred, err := s.store.GetOrCreateCredential(ctx, appID, s.genKey)
	if err != nil {
		return Credential{}, err
	}
	if err := s.gw.EnsureConsumer(ctx, cred.ConsumerUsername, cred.APIKey,
		apisix.RateLimit{Count: plan.Count, WindowSeconds: plan.WindowSeconds}); err != nil {
		return Credential{}, err
	}
	if err := s.store.SaveSubscription(ctx, appID, productID, planID); err != nil {
		return Credential{}, err
	}
	allowed, err := s.store.ConsumersForProduct(ctx, productID)
	if err != nil {
		return Credential{}, err
	}
	if err := s.gw.EnsureRoute(ctx, routeID(prod.ID), prod.ContextPath+"/*", prod.Upstream, allowed); err != nil {
		return Credential{}, err
	}
	return cred, nil
}

// Unsubscribe removes the subscription and updates the route whitelist.
func (s *Service) Unsubscribe(ctx context.Context, appID, productID int64) error {
	prod, err := s.store.GetProduct(ctx, productID)
	if err != nil {
		return err
	}
	if err := s.store.DeleteSubscription(ctx, appID, productID); err != nil {
		return err
	}
	allowed, err := s.store.ConsumersForProduct(ctx, productID)
	if err != nil {
		return err
	}
	return s.gw.EnsureRoute(ctx, routeID(prod.ID), prod.ContextPath+"/*", prod.Upstream, allowed)
}
