package billing

import "context"

type Store interface {
	PlanPricing(ctx context.Context, planID int64) (string, int, string, error)
	TeamForApp(ctx context.Context, appID int64) (int64, error)
	EnsureAccount(ctx context.Context, teamID int64) (int64, error)
	PendingInvoiceExists(ctx context.Context, subID int64) (bool, error)
	CreateInvoice(ctx context.Context, accountID, teamID, subID int64, planName string, priceCents int, currency string) (Invoice, error)
	Get(ctx context.Context, id int64) (Invoice, error)
	MarkPaid(ctx context.Context, id int64) error
	Void(ctx context.Context, id int64) error
	ListByTeams(ctx context.Context, teamIDs []int64) ([]Invoice, error)
	ListAll(ctx context.Context, status string) ([]Invoice, error)
	TeamsForUser(ctx context.Context, userID int64) ([]int64, error)
}

type Service struct {
	store    Store
	provider BillingProvider
}

func NewService(store Store, provider BillingProvider) *Service {
	return &Service{store: store, provider: provider}
}

// SubscriptionActivated records a pending invoice for a newly-activated PAID
// subscription. Free plans (price 0) are a no-op. Idempotent per subscription.
// This IS the subscriptions.Biller method.
func (s *Service) SubscriptionActivated(ctx context.Context, appID, subID, planID int64) error {
	name, priceCents, currency, err := s.store.PlanPricing(ctx, planID)
	if err != nil {
		return err
	}
	if priceCents == 0 {
		return nil // free plan → no billing
	}
	exists, err := s.store.PendingInvoiceExists(ctx, subID)
	if err != nil {
		return err
	}
	if exists {
		return nil // idempotent: already invoiced
	}
	teamID, err := s.store.TeamForApp(ctx, appID)
	if err != nil {
		return err
	}
	accountID, err := s.store.EnsureAccount(ctx, teamID)
	if err != nil {
		return err
	}
	inv := Invoice{TeamID: teamID, SubscriptionID: &subID, PlanName: name, PriceCents: priceCents, Currency: currency}
	if _, err := s.provider.Charge(ctx, inv); err != nil {
		return err
	}
	_, err = s.store.CreateInvoice(ctx, accountID, teamID, subID, name, priceCents, currency)
	return err
}

func (s *Service) MarkPaid(ctx context.Context, id int64) error       { return s.store.MarkPaid(ctx, id) }
func (s *Service) Void(ctx context.Context, id int64) error           { return s.store.Void(ctx, id) }
func (s *Service) Get(ctx context.Context, id int64) (Invoice, error) { return s.store.Get(ctx, id) }
func (s *Service) ListAll(ctx context.Context, status string) ([]Invoice, error) {
	return s.store.ListAll(ctx, status)
}

// ListForUser returns invoices across all teams the user belongs to.
func (s *Service) ListForUser(ctx context.Context, userID int64) ([]Invoice, error) {
	teamIDs, err := s.store.TeamsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.store.ListByTeams(ctx, teamIDs)
}
