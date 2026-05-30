package admin

import (
	"context"
	"errors"
)

// ErrHasSubscriptions is returned when a product cannot be deleted because it
// still has active subscriptions.
var ErrHasSubscriptions = errors.New("admin: product has active subscriptions")

// Store is the persistence surface the service needs (satisfied by *Repo).
type Store interface {
	ListAll(ctx context.Context) ([]Product, error)
	Get(ctx context.Context, id int64) (Product, error)
	Create(ctx context.Context, p Product) (Product, error)
	Update(ctx context.Context, p Product) (Product, error)
	Delete(ctx context.Context, id int64) error
	CountActiveSubscriptions(ctx context.Context, productID int64) (int, error)
}

// Provisioner triggers APISIX route changes (satisfied by *subscriptions.Service).
type Provisioner interface {
	ReprovisionRoute(ctx context.Context, productID int64) error
	DeprovisionRoute(ctx context.Context, productID int64) error
}

// Service applies admin product operations and keeps APISIX in sync.
type Service struct {
	store Store
	prov  Provisioner
}

func NewService(store Store, prov Provisioner) *Service {
	return &Service{store: store, prov: prov}
}

func (s *Service) List(ctx context.Context) ([]Product, error)        { return s.store.ListAll(ctx) }
func (s *Service) Get(ctx context.Context, id int64) (Product, error) { return s.store.Get(ctx, id) }
func (s *Service) Create(ctx context.Context, p Product) (Product, error) {
	return s.store.Create(ctx, p)
}

// Update persists changes and, when the upstream changed on a product that has
// active subscriptions, rebuilds its APISIX route so the new upstream takes effect.
func (s *Service) Update(ctx context.Context, p Product) (Product, error) {
	old, err := s.store.Get(ctx, p.ID)
	if err != nil {
		return Product{}, err
	}
	updated, err := s.store.Update(ctx, p)
	if err != nil {
		return Product{}, err
	}
	if updated.UpstreamURL != old.UpstreamURL {
		n, err := s.store.CountActiveSubscriptions(ctx, p.ID)
		if err != nil {
			return Product{}, err
		}
		if n > 0 {
			if err := s.prov.ReprovisionRoute(ctx, p.ID); err != nil {
				return Product{}, err
			}
		}
	}
	return updated, nil
}

// Delete refuses (ErrHasSubscriptions) while active subscriptions exist; otherwise
// it removes the product and tears down its APISIX route (best effort).
func (s *Service) Delete(ctx context.Context, id int64) error {
	n, err := s.store.CountActiveSubscriptions(ctx, id)
	if err != nil {
		return err
	}
	if n > 0 {
		return ErrHasSubscriptions
	}
	if err := s.store.Delete(ctx, id); err != nil {
		return err
	}
	// Best effort: the row is already gone; a stale gateway route is harmless and
	// will be overwritten if the id is ever reused.
	_ = s.prov.DeprovisionRoute(ctx, id)
	return nil
}
