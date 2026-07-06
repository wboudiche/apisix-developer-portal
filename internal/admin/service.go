package admin

import (
	"context"
	"errors"
	"log"
	"time"

	"apisix-portal/internal/paging"
)

// ErrHasSubscriptions is returned when a product cannot be deleted because it
// still has active subscriptions.
var ErrHasSubscriptions = errors.New("admin: product has active subscriptions")

// Store is the persistence surface the service needs (satisfied by *Repo).
type Store interface {
	ListAll(ctx context.Context, p paging.Params) ([]Product, int, error)
	Get(ctx context.Context, id int64) (Product, error)
	Create(ctx context.Context, p Product) (Product, error)
	Update(ctx context.Context, p Product) (Product, error)
	Delete(ctx context.Context, id int64) error
	CountActiveSubscriptions(ctx context.Context, productID int64) (int, error)
	ContextPathOverlaps(ctx context.Context, p string, exceptID int64) (bool, error)
	AddChangelog(ctx context.Context, productID int64, e ChangelogEntry) (ChangelogEntry, error)
	ListChangelog(ctx context.Context, productID int64) ([]ChangelogEntry, error)
	DeleteChangelog(ctx context.Context, productID, entryID int64) error
	SetUploadedIcon(ctx context.Context, productID int64, png []byte) (time.Time, error)
	DeleteIcon(ctx context.Context, productID int64) error
}

// Provisioner triggers APISIX route changes (satisfied by *subscriptions.Service).
type Provisioner interface {
	ReprovisionRoute(ctx context.Context, productID int64) error
	ReprovisionSandboxRoute(ctx context.Context, productID int64) error
	DeprovisionRoute(ctx context.Context, productID int64) error
}

// Service applies admin product operations and keeps APISIX in sync.
// Upstream/contextPath validation (including the SSRF allowPrivate flag)
// lives in the handler, at the request boundary.
type Service struct {
	store Store
	prov  Provisioner
}

func NewService(store Store, prov Provisioner) *Service {
	return &Service{store: store, prov: prov}
}

func (s *Service) List(ctx context.Context, p paging.Params) ([]Product, int, error) {
	return s.store.ListAll(ctx, p)
}
func (s *Service) Get(ctx context.Context, id int64) (Product, error) { return s.store.Get(ctx, id) }
func (s *Service) Create(ctx context.Context, p Product) (Product, error) {
	overlaps, err := s.store.ContextPathOverlaps(ctx, p.ContextPath, 0)
	if err != nil {
		return Product{}, err
	}
	if overlaps {
		return Product{}, ErrContextPathTaken
	}
	return s.store.Create(ctx, p)
}

// Update persists changes and, when the upstream or auth_type changed on a
// product that has active subscriptions, rebuilds its APISIX route so the new
// configuration takes effect.
func (s *Service) Update(ctx context.Context, p Product) (Product, error) {
	old, err := s.store.Get(ctx, p.ID)
	if err != nil {
		return Product{}, err
	}
	if p.ContextPath != old.ContextPath {
		overlaps, err := s.store.ContextPathOverlaps(ctx, p.ContextPath, p.ID)
		if err != nil {
			return Product{}, err
		}
		if overlaps {
			return Product{}, ErrContextPathTaken
		}
	}
	updated, err := s.store.Update(ctx, p)
	if err != nil {
		return Product{}, err
	}
	if updated.Icon != "upload" {
		if err := s.store.DeleteIcon(ctx, p.ID); err != nil {
			return Product{}, err
		}
	}
	if updated.UpstreamURL != old.UpstreamURL || updated.AuthType != old.AuthType {
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
	if updated.SandboxUpstreamURL != old.SandboxUpstreamURL {
		if err := s.prov.ReprovisionSandboxRoute(ctx, p.ID); err != nil {
			return Product{}, err
		}
	}
	return updated, nil
}

// SetUploadedIcon stores a re-encoded PNG icon for a product.
func (s *Service) SetUploadedIcon(ctx context.Context, productID int64, png []byte) (time.Time, error) {
	return s.store.SetUploadedIcon(ctx, productID, png)
}

// AddChangelog records a changelog entry for a product.
func (s *Service) AddChangelog(ctx context.Context, productID int64, e ChangelogEntry) (ChangelogEntry, error) {
	return s.store.AddChangelog(ctx, productID, e)
}

// ListChangelog returns all changelog entries for a product, including ones
// on unpublished/draft products (unlike the public catalog listing).
func (s *Service) ListChangelog(ctx context.Context, productID int64) ([]ChangelogEntry, error) {
	return s.store.ListChangelog(ctx, productID)
}

// DeleteChangelog removes a changelog entry from a product.
func (s *Service) DeleteChangelog(ctx context.Context, productID, entryID int64) error {
	return s.store.DeleteChangelog(ctx, productID, entryID)
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
	// Best effort: the product row is already gone. A leftover gateway route is
	// harmless (its whitelist was already rebuilt to empty by the last unsubscribe,
	// so it admits no traffic), but log a failure so an operator can clean it up.
	if err := s.prov.DeprovisionRoute(ctx, id); err != nil {
		log.Printf("admin: deprovision route for product %d: %v", id, err)
	}
	return nil
}
