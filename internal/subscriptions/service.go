package subscriptions

import (
	"context"
	"errors"
	"fmt"
	"log"

	"apisix-portal/internal/apisix"
	"apisix-portal/internal/events"
	"apisix-portal/internal/paging"
)

// EventLogger records application activity for the Overview feed. Implemented by
// *events.Repo; nil disables logging (used in tests). Logging is best-effort:
// the action it describes has already succeeded, so a log failure is swallowed.
type EventLogger interface {
	Log(ctx context.Context, appID int64, kind string, productID, planID *int64) error
}

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

// ErrNoCredential is returned by RotateKey when the application has no credential.
var ErrNoCredential = errors.New("subscriptions: application has no credential")

// ErrNoActiveSubscription is returned by RotateKey when the application has no
// active subscription (so no rate limit can be derived for the refreshed consumer).
var ErrNoActiveSubscription = errors.New("subscriptions: application has no active subscription")

// ErrSandboxNotConfigured is returned when EnableSandbox is called but no
// sandbox gateway has been wired into the service.
var ErrSandboxNotConfigured = errors.New("subscriptions: sandbox gateway not configured")

// ErrOIDCNotConfigured is returned when an oauth2 operation requires a trusted
// OIDC issuer but none has been wired via ConfigureOIDC.
var ErrOIDCNotConfigured = errors.New("subscriptions: oidc not configured")

// ErrInvalidClientID is returned by SetOIDCClientID when the client id contains
// characters outside the safe charset (Lua injection guard).
var ErrInvalidClientID = errors.New("subscriptions: invalid oidc client id")

// ErrNoSandboxEligibleSubscription is returned by EnableSandbox when the
// application has no active subscription to a sandbox-enabled product (or has
// no credential / active plan at all).
var ErrNoSandboxEligibleSubscription = errors.New("subscriptions: no active subscription to a sandbox-enabled product")

// ErrNoSandboxKey is returned when a sandbox operation requires the application
// to already have a sandbox key but none is stored.
var ErrNoSandboxKey = errors.New("subscriptions: application has no sandbox key")

// ProductInfo is what the service needs to provision a product's gateway route.
type ProductInfo struct {
	ID              int64
	ContextPath     string
	Upstream        string // scheme://host:port (or bare host:port, treated as http)
	SandboxUpstream string // product's sandbox backend, "" = no sandbox
	Published       bool
	AuthType        string
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
	AdminSubscriptions(ctx context.Context, statusFilter string, p paging.Params) ([]AdminSubscriptionView, int, error)
	// GetCredential returns the application's gateway credential or ErrNotFound.
	GetCredential(ctx context.Context, appID int64) (Credential, error)
	// ActivePlanForApp returns the plan for the application's most-recent active
	// subscription, or ErrNoActiveSubscription when none exists.
	ActivePlanForApp(ctx context.Context, appID int64) (PlanInfo, error)
	// UpdateCredentialKey replaces the stored API key for the application's credential.
	UpdateCredentialKey(ctx context.Context, appID int64, newKey string) error
	GetSandboxKey(ctx context.Context, appID int64) (string, error)
	UpdateSandboxKey(ctx context.Context, appID int64, key string) error
	SandboxConsumersForProduct(ctx context.Context, productID int64) ([]string, error)
	SandboxConsumersForPlan(ctx context.Context, planID int64) ([]Credential, error)
	SandboxProductsForApp(ctx context.Context, appID int64) ([]ProductInfo, error)
	// OAuthClientsForProduct returns oidc_client_ids of active subscribers whose
	// app has a non-empty oidc_client_id (the OAuth2 route whitelist).
	OAuthClientsForProduct(ctx context.Context, productID int64) ([]string, error)
	// OAuthProductsForApp returns the oauth2 products the app actively subscribes to.
	OAuthProductsForApp(ctx context.Context, appID int64) ([]ProductInfo, error)
	// GetAppOIDCClientID returns the app's current OIDC client id ("" when unset;
	// ErrNotFound if no app row).
	GetAppOIDCClientID(ctx context.Context, appID int64) (string, error)
	// SetAppOIDCClientID persists the OIDC client id for the app (ErrNotFound when
	// no app row).
	SetAppOIDCClientID(ctx context.Context, appID int64, clientID string) error
}

func consumerName(appID int64) string { return fmt.Sprintf("app_%d", appID) }

// dedup returns a new slice with duplicates removed and empty strings dropped,
// preserving the order of first occurrence.
func dedup(ss []string) []string {
	seen := make(map[string]struct{}, len(ss))
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}

// RouteID is the deterministic APISIX route id for a product.
func RouteID(productID int64) string { return fmt.Sprintf("prod_%d", productID) }

// Service orchestrates subscribe/unsubscribe and the matching APISIX provisioning.
type Service struct {
	store      Store
	gw         apisix.Gateway
	sandboxGW  apisix.Gateway
	genKey     func() string
	events     EventLogger
	oidcIssuer string
	oidcClaim  string
}

// ConfigureOIDC wires the trusted issuer + client-id claim for oauth2 product
// routes. Empty issuer leaves OAuth2 provisioning disabled.
func (s *Service) ConfigureOIDC(issuer, claim string) { s.oidcIssuer, s.oidcClaim = issuer, claim }

func NewService(store Store, gw, sandboxGW apisix.Gateway, genKey func() string, eventLog EventLogger) *Service {
	return &Service{store: store, gw: gw, sandboxGW: sandboxGW, genKey: genKey, events: eventLog}
}

func (s *Service) sandboxEnabled() bool { return s.sandboxGW != nil }

// logEvent appends an activity event, best-effort: a failure is logged but never
// propagated, so the feed can never break the action it merely records.
func (s *Service) logEvent(ctx context.Context, appID int64, kind string, productID, planID *int64) {
	if s.events == nil {
		return
	}
	if err := s.events.Log(ctx, appID, kind, productID, planID); err != nil {
		log.Printf("activity log %q for app %d: %v", kind, appID, err)
	}
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

	if prod.AuthType == "oauth2" {
		allowed, err := s.store.OAuthClientsForProduct(ctx, productID)
		if err != nil {
			return err
		}
		for _, e := range extraConsumers { // skip "" (app has no client id yet) to preserve the delete-when-empty lifecycle invariant
			if e != "" {
				allowed = append(allowed, e)
			}
		}
		if len(allowed) == 0 || s.oidcIssuer == "" {
			return s.gw.DeleteRoute(ctx, RouteID(prod.ID))
		}
		return s.gw.EnsureOAuthRoute(ctx, RouteID(prod.ID), prod.ContextPath, prod.Upstream, s.oidcIssuer, s.oidcClaim, dedup(allowed))
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
	return s.gw.EnsureRoute(ctx, RouteID(prod.ID), prod.ContextPath, prod.Upstream, allowed)
}

// DeprovisionRoute removes the product's APISIX route entirely.
func (s *Service) DeprovisionRoute(ctx context.Context, productID int64) error {
	return s.gw.DeleteRoute(ctx, RouteID(productID))
}

// SetOIDCClientID stores the OIDC client id for an application and immediately
// re-provisions every oauth2 product route the app has an active subscription
// on, so the updated client id takes effect without a manual reprovision.
func (s *Service) SetOIDCClientID(ctx context.Context, appID int64, clientID string) error {
	if clientID != "" && !apisix.ValidClientID(clientID) {
		return ErrInvalidClientID
	}
	if err := s.store.SetAppOIDCClientID(ctx, appID, clientID); err != nil {
		return err
	}
	prods, err := s.store.OAuthProductsForApp(ctx, appID)
	if err != nil {
		return err
	}
	for _, p := range prods {
		if err := s.reprovisionRoute(ctx, p.ID); err != nil {
			return err
		}
	}
	return nil
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
	if s.sandboxEnabled() {
		sbConsumers, err := s.store.SandboxConsumersForPlan(ctx, planID)
		if err != nil {
			return err
		}
		for _, c := range sbConsumers {
			if err := s.sandboxGW.EnsureConsumer(ctx, c.ConsumerUsername, c.APIKey,
				apisix.RateLimit{Count: plan.Count, WindowSeconds: plan.WindowSeconds}); err != nil {
				return err
			}
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
	if prod.AuthType == "oauth2" {
		if err := s.store.SaveSubscription(ctx, appID, productID, planID); err != nil {
			return Credential{}, err
		}
		s.logEvent(ctx, appID, events.KindSubscribed, &productID, &planID)
		return Credential{}, nil // no key for oauth2 apps
	}
	cred, err := s.store.GetOrCreateCredential(ctx, appID, s.genKey)
	if err != nil {
		return Credential{}, err
	}
	if err := s.store.SaveSubscription(ctx, appID, productID, planID); err != nil {
		return Credential{}, err
	}
	s.logEvent(ctx, appID, events.KindSubscribed, &productID, &planID)
	return cred, nil
}

// Unsubscribe removes the subscription and updates the route whitelist.
func (s *Service) Unsubscribe(ctx context.Context, appID, productID int64) error {
	if err := s.store.DeleteSubscription(ctx, appID, productID); err != nil {
		return err
	}
	// Log after the durable delete but before reprovision: the subscription is
	// already gone, so the event reflects the true DB state even if the gateway
	// sync below fails (and a retry would otherwise never re-log it).
	s.logEvent(ctx, appID, events.KindUnsubscribed, &productID, nil)
	if err := s.ReprovisionRoute(ctx, productID); err != nil {
		return err
	}
	return s.reprovisionSandboxRoute(ctx, productID)
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
	prod, err := s.store.GetProduct(ctx, rec.ProductID)
	if err != nil {
		return err
	}
	if prod.AuthType == "oauth2" {
		cid, _ := s.store.GetAppOIDCClientID(ctx, rec.AppID)
		if err := s.reprovisionRoute(ctx, rec.ProductID, cid); err != nil {
			return err
		}
		if err := s.store.SetSubscriptionStatus(ctx, subID, StatusActive); err != nil {
			return err
		}
		s.logEvent(ctx, rec.AppID, events.KindApproved, &rec.ProductID, &rec.PlanID)
		return nil
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
	if err := s.store.SetSubscriptionStatus(ctx, subID, StatusActive); err != nil {
		return err
	}
	if s.sandboxEnabled() {
		if sk, err := s.store.GetSandboxKey(ctx, rec.AppID); err == nil && sk != "" {
			if err := s.reprovisionSandboxRoute(ctx, rec.ProductID, cred.ConsumerUsername); err != nil {
				return err
			}
		}
	}
	s.logEvent(ctx, rec.AppID, events.KindApproved, &rec.ProductID, &rec.PlanID)
	return nil
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
	// Log after the durable status change (see Unsubscribe): the event reflects
	// the persisted "rejected" state regardless of the gateway sync outcome.
	s.logEvent(ctx, rec.AppID, events.KindRejected, &rec.ProductID, nil)
	if err := s.ReprovisionRoute(ctx, rec.ProductID); err != nil {
		return err
	}
	return s.reprovisionSandboxRoute(ctx, rec.ProductID)
}

// AdminSubscriptions lists subscriptions for the admin queue (see Store).
func (s *Service) AdminSubscriptions(ctx context.Context, statusFilter string, p paging.Params) ([]AdminSubscriptionView, int, error) {
	return s.store.AdminSubscriptions(ctx, statusFilter, p)
}

// reprovisionSandboxRoute rebuilds the product's route on the SANDBOX gateway
// from its sandbox upstream and the set of active subscribers that have a
// sandbox key (plus any extras not yet reflected in the store). No-op when
// sandbox is disabled or the product has no sandbox upstream.
func (s *Service) reprovisionSandboxRoute(ctx context.Context, productID int64, extra ...string) error {
	if !s.sandboxEnabled() {
		return nil
	}
	prod, err := s.store.GetProduct(ctx, productID)
	if err != nil {
		return err
	}
	if prod.SandboxUpstream == "" {
		return s.sandboxGW.DeleteRoute(ctx, RouteID(prod.ID))
	}
	allowed, err := s.store.SandboxConsumersForProduct(ctx, productID)
	if err != nil {
		return err
	}
	for _, e := range extra {
		present := false
		for _, a := range allowed {
			if a == e {
				present = true
				break
			}
		}
		if !present {
			allowed = append(allowed, e)
		}
	}
	if len(allowed) == 0 {
		return s.sandboxGW.DeleteRoute(ctx, RouteID(prod.ID))
	}
	return s.sandboxGW.EnsureRoute(ctx, RouteID(prod.ID), prod.ContextPath, prod.SandboxUpstream, allowed)
}

// EnableSandbox opts an application into sandbox: it generates (once) the app's
// sandbox key, provisions the sandbox consumer with the app's active plan limit
// on the sandbox gateway, and grants it on the sandbox route of every
// sandbox-enabled product the app actively subscribes to. Idempotent — a second
// call returns the existing key and re-asserts the gateway state.
func (s *Service) EnableSandbox(ctx context.Context, appID int64) (string, error) {
	if !s.sandboxEnabled() {
		return "", ErrSandboxNotConfigured
	}
	cred, err := s.store.GetCredential(ctx, appID)
	if errors.Is(err, ErrNotFound) {
		return "", ErrNoSandboxEligibleSubscription
	}
	if err != nil {
		return "", err
	}
	plan, err := s.store.ActivePlanForApp(ctx, appID)
	if errors.Is(err, ErrNoActiveSubscription) {
		return "", ErrNoSandboxEligibleSubscription
	}
	if err != nil {
		return "", err
	}
	prods, err := s.store.SandboxProductsForApp(ctx, appID)
	if err != nil {
		return "", err
	}
	if len(prods) == 0 {
		return "", ErrNoSandboxEligibleSubscription
	}
	key, err := s.store.GetSandboxKey(ctx, appID)
	if err != nil {
		return "", err
	}
	if key == "" {
		key = s.genKey()
	}
	if err := s.sandboxGW.EnsureConsumer(ctx, cred.ConsumerUsername, key,
		apisix.RateLimit{Count: plan.Count, WindowSeconds: plan.WindowSeconds}); err != nil {
		return "", err
	}
	if err := s.store.UpdateSandboxKey(ctx, appID, key); err != nil {
		return "", err
	}
	for _, p := range prods {
		if err := s.reprovisionSandboxRoute(ctx, p.ID, cred.ConsumerUsername); err != nil {
			return "", err
		}
	}
	s.logEvent(ctx, appID, events.KindSandboxEnabled, nil, nil)
	return key, nil
}

// ReprovisionSandboxRoute rebuilds a product's sandbox route (used by admin on a
// sandbox-upstream change). No-op when sandbox is disabled.
func (s *Service) ReprovisionSandboxRoute(ctx context.Context, productID int64) error {
	return s.reprovisionSandboxRoute(ctx, productID)
}

// RotateSandboxKey issues a fresh sandbox key, installs it on the sandbox
// gateway consumer (old key 401s immediately), then persists it (gateway before
// DB). The limit is preserved from the app's active plan. 409 if the app has no
// sandbox key (ErrNoSandboxKey) or sandbox is disabled (ErrSandboxNotConfigured).
func (s *Service) RotateSandboxKey(ctx context.Context, appID int64) (string, error) {
	if !s.sandboxEnabled() {
		return "", ErrSandboxNotConfigured
	}
	cred, err := s.store.GetCredential(ctx, appID)
	if errors.Is(err, ErrNotFound) {
		return "", ErrNoSandboxKey
	}
	if err != nil {
		return "", err
	}
	existing, err := s.store.GetSandboxKey(ctx, appID)
	if err != nil {
		return "", err
	}
	if existing == "" {
		return "", ErrNoSandboxKey
	}
	plan, err := s.store.ActivePlanForApp(ctx, appID)
	if err != nil {
		return "", err
	}
	newKey := s.genKey()
	if err := s.sandboxGW.EnsureConsumer(ctx, cred.ConsumerUsername, newKey,
		apisix.RateLimit{Count: plan.Count, WindowSeconds: plan.WindowSeconds}); err != nil {
		return "", err
	}
	if err := s.store.UpdateSandboxKey(ctx, appID, newKey); err != nil {
		return "", err
	}
	s.logEvent(ctx, appID, events.KindSandboxKeyRotated, nil, nil)
	return newKey, nil
}

// RotateKey issues a fresh key-auth key for the application, installs it on the
// APISIX consumer (the old key 401s immediately), then persists it. The gateway
// call precedes the DB write so a gateway failure leaves the old, still-live key
// in the DB. The consumer's rate limit is preserved from the app's most-recent
// active subscription plan. Returns the new key.
func (s *Service) RotateKey(ctx context.Context, appID int64) (string, error) {
	cred, err := s.store.GetCredential(ctx, appID)
	if errors.Is(err, ErrNotFound) {
		return "", ErrNoCredential
	}
	if err != nil {
		return "", err
	}
	plan, err := s.store.ActivePlanForApp(ctx, appID)
	if err != nil {
		return "", err // ErrNoActiveSubscription bubbles to a 409 at the handler
	}
	newKey := s.genKey()
	if err := s.gw.EnsureConsumer(ctx, cred.ConsumerUsername, newKey,
		apisix.RateLimit{Count: plan.Count, WindowSeconds: plan.WindowSeconds}); err != nil {
		return "", err
	}
	if err := s.store.UpdateCredentialKey(ctx, appID, newKey); err != nil {
		return "", err
	}
	s.logEvent(ctx, appID, events.KindKeyRotated, nil, nil)
	return newKey, nil
}
