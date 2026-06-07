package subscriptions

import (
	"context"
	"errors"
	"fmt"

	"apisix-portal/internal/apisix"
)

// Subscription lifecycle states.
const (
	StatusPending  = "pending"
	StatusActive   = "active"
	StatusRejected = "rejected"
)

// ErrAlreadySubscribed is returned by Subscribe when the application already has
// an active subscription to the product (re-subscribing would bypass the approval gate).
var ErrAlreadySubscribed = errors.New("already subscribed")

// ErrInvalidTransition is returned when a status change is not allowed from the
// subscription's current state (e.g. approving a rejected subscription).
var ErrInvalidTransition = errors.New("invalid status transition")

// ProductInfo is what the service needs to provision a product's gateway route.
type ProductInfo struct {
	ID          int64
	ContextPath string
	Upstream    string // host:port
	Published   bool
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
	// ConsumersForPlan returns the credential (consumer identity + key) of every
	// application with an active subscription on the plan.
	ConsumersForPlan(ctx context.Context, planID int64) ([]Credential, error)
	// GetSubscription returns a single subscription's identity + status by id.
	GetSubscription(ctx context.Context, subID int64) (SubscriptionRecord, error)
	// SetSubscriptionStatus transitions a subscription to the given status.
	SetSubscriptionStatus(ctx context.Context, subID int64, status string) error
	// SubscriptionStatus returns the status of the (app, product) subscription,
	// or "" if there is none.
	SubscriptionStatus(ctx context.Context, appID, productID int64) (string, error)
	// AdminSubscriptions lists subscriptions for the admin queue. An empty
	// statusFilter returns all; otherwise only rows with that status.
	AdminSubscriptions(ctx context.Context, statusFilter string) ([]AdminSubscriptionView, error)
}

func consumerName(appID int64) string { return fmt.Sprintf("app_%d", appID) }

// RouteID is the deterministic APISIX route id for a product.
func RouteID(productID int64) string { return fmt.Sprintf("prod_%d", productID) }

// Service orchestrates subscribe/unsubscribe and the matching APISIX provisioning.
type Service struct {
	store  Store
	gw     apisix.Gateway
	genKey func() string
}

func NewService(store Store, gw apisix.Gateway, genKey func() string) *Service {
	return &Service{store: store, gw: gw, genKey: genKey}
}

// ReprovisionRoute rebuilds the product's APISIX route from its current upstream
// and the set of active subscribers' consumer names. Safe to call repeatedly.
func (s *Service) ReprovisionRoute(ctx context.Context, productID int64) error {
	return s.reprovisionRoute(ctx, productID)
}

// reprovisionRoute rebuilds the product route whitelist from the currently active
// subscribers, plus any extraConsumers (e.g. a subscription about to be marked
// active, which is not yet returned by ConsumersForProduct). Extras already in the
// active set are deduplicated.
func (s *Service) reprovisionRoute(ctx context.Context, productID int64, extraConsumers ...string) error {
	prod, err := s.store.GetProduct(ctx, productID)
	if err != nil {
		return err
	}
	allowed, err := s.store.ConsumersForProduct(ctx, productID)
	if err != nil {
		return err
	}
	for _, extra := range extraConsumers {
		present := false
		for _, a := range allowed {
			if a == extra {
				present = true
				break
			}
		}
		if !present {
			allowed = append(allowed, extra)
		}
	}
	// APISIX's consumer-restriction plugin rejects an empty whitelist
	// ("expect array to have at least 1 items"). When the last subscriber is
	// removed the route has no allowed consumers, so delete it entirely rather
	// than pushing an invalid config. A route with zero subscribers should not
	// exist; the next approval recreates it.
	if len(allowed) == 0 {
		return s.gw.DeleteRoute(ctx, RouteID(prod.ID))
	}
	return s.gw.EnsureRoute(ctx, RouteID(prod.ID), prod.ContextPath+"/*", prod.Upstream, allowed)
}

// DeprovisionRoute removes the product's APISIX route entirely.
func (s *Service) DeprovisionRoute(ctx context.Context, productID int64) error {
	return s.gw.DeleteRoute(ctx, RouteID(productID))
}

// ReprovisionPlan applies the plan's current rate limits to every active
// subscriber's APISIX consumer. Used when an admin edits a plan's limits.
func (s *Service) ReprovisionPlan(ctx context.Context, planID int64) error {
	plan, err := s.store.GetPlan(ctx, planID)
	if err != nil {
		return err
	}
	consumers, err := s.store.ConsumersForPlan(ctx, planID)
	if err != nil {
		return err
	}
	for _, c := range consumers {
		if err := s.gw.EnsureConsumer(ctx, c.ConsumerUsername, c.APIKey,
			apisix.RateLimit{Count: plan.Count, WindowSeconds: plan.WindowSeconds}); err != nil {
			return err
		}
	}
	return nil
}

// Subscribe records a PENDING subscription and issues the application's gateway
// credential, but performs NO provisioning — the key will not pass the gateway
// until an admin approves the subscription. Returns the credential.
//
// An unpublished product or unknown plan is rejected with ErrNotFound (unpublished
// products are treated as non-existent to callers, so their existence is not leaked).
//
// Re-subscribing a product the app is ALREADY ACTIVE on is refused with
// ErrAlreadySubscribed — this prevents both the approval-gate bypass and an
// involuntary mid-flight revocation of a live subscription.
//
// Re-subscribing a pending or rejected subscription is allowed and resets it to
// pending (e.g. to request a plan change).
func (s *Service) Subscribe(ctx context.Context, appID, productID, planID int64) (Credential, error) {
	prod, err := s.store.GetProduct(ctx, productID)
	if err != nil {
		return Credential{}, err
	}
	if !prod.Published {
		return Credential{}, ErrNotFound // unpublished products are not subscribable; don't leak existence
	}
	if _, err := s.store.GetPlan(ctx, planID); err != nil {
		return Credential{}, err
	}
	existing, err := s.store.SubscriptionStatus(ctx, appID, productID)
	if err != nil {
		return Credential{}, err
	}
	if existing == StatusActive {
		return Credential{}, ErrAlreadySubscribed
	}
	cred, err := s.store.GetOrCreateCredential(ctx, appID, s.genKey)
	if err != nil {
		return Credential{}, err
	}
	if err := s.store.SaveSubscription(ctx, appID, productID, planID); err != nil {
		return Credential{}, err
	}
	return cred, nil
}

// Unsubscribe removes the subscription and updates the route whitelist.
func (s *Service) Unsubscribe(ctx context.Context, appID, productID int64) error {
	if err := s.store.DeleteSubscription(ctx, appID, productID); err != nil {
		return err
	}
	return s.ReprovisionRoute(ctx, productID)
}

// Approve activates a pending subscription: it provisions the application's
// consumer with the plan's limits and rebuilds the product route whitelist to
// include it, and ONLY THEN marks the subscription active. If any gateway call
// fails the status stays pending and the error is returned, so the invariant
// "active in DB ⇒ provisioned in gateway" always holds. Idempotent — approving
// an already-active subscription re-asserts the gateway state and converges.
// Returns ErrInvalidTransition if the subscription is rejected.
func (s *Service) Approve(ctx context.Context, subID int64) error {
	rec, err := s.store.GetSubscription(ctx, subID)
	if err != nil {
		return err
	}
	if rec.Status == StatusRejected {
		return ErrInvalidTransition // a rejected subscription cannot be approved
	}
	plan, err := s.store.GetPlan(ctx, rec.PlanID)
	if err != nil {
		return err
	}
	cred, err := s.store.GetOrCreateCredential(ctx, rec.AppID, s.genKey)
	if err != nil {
		return err
	}
	if err := s.gw.EnsureConsumer(ctx, cred.ConsumerUsername, cred.APIKey,
		apisix.RateLimit{Count: plan.Count, WindowSeconds: plan.WindowSeconds}); err != nil {
		return err
	}
	// Provision the route whitelist INCLUDING the about-to-be-active consumer
	// before marking the subscription active. If this gateway call fails the
	// status stays pending, so the invariant "active in DB ⇒ provisioned in
	// gateway" holds and a later re-approve converges (idempotent).
	if err := s.reprovisionRoute(ctx, rec.ProductID, cred.ConsumerUsername); err != nil {
		return err
	}
	return s.store.SetSubscriptionStatus(ctx, subID, StatusActive)
}

// Reject marks a subscription rejected and rebuilds the product route whitelist
// so the application is excluded (a no-op for a still-pending subscription,
// which was never in the whitelist). Returns ErrInvalidTransition if already rejected.
func (s *Service) Reject(ctx context.Context, subID int64) error {
	rec, err := s.store.GetSubscription(ctx, subID)
	if err != nil {
		return err
	}
	if rec.Status == StatusRejected {
		return ErrInvalidTransition // already rejected
	}
	if err := s.store.SetSubscriptionStatus(ctx, subID, StatusRejected); err != nil {
		return err
	}
	return s.ReprovisionRoute(ctx, rec.ProductID)
}

// AdminSubscriptions lists subscriptions for the admin queue (see Store).
func (s *Service) AdminSubscriptions(ctx context.Context, statusFilter string) ([]AdminSubscriptionView, error) {
	return s.store.AdminSubscriptions(ctx, statusFilter)
}
