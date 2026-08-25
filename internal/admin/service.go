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

// ErrOAuthMigrationBlocked is returned when switching a product's auth method
// to oauth2 would leave one or more of its active subscribers without an
// eligible consumer (they haven't registered an OIDC client id yet), which
// would otherwise cause reprovisioning to either drop them from the route's
// whitelist silently, or — if none are eligible — delete the route outright
// (see reprovisionRoute).
var ErrOAuthMigrationBlocked = errors.New("admin: cannot switch to oauth2 until every active subscriber has an OIDC client id registered")

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
	GetIcon(ctx context.Context, productID int64) ([]byte, time.Time, error)
}

// Provisioner triggers APISIX route changes (satisfied by *subscriptions.Service).
type Provisioner interface {
	ReprovisionRoute(ctx context.Context, productID int64) error
	ReprovisionSandboxRoute(ctx context.Context, productID int64) error
	DeprovisionRoute(ctx context.Context, productID int64) error
	// OAuthReadyConsumerCount returns how many of productID's active
	// subscribers have registered an OIDC client id.
	OAuthReadyConsumerCount(ctx context.Context, productID int64) (int, error)
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
	// activeCount is loaded at most once and reused below by both the oauth2
	// migration guard and the reprovision decision, so a single Update never
	// issues the same COUNT query twice.
	var activeCount int
	var activeCountLoaded bool
	loadActiveCount := func() (int, error) {
		if !activeCountLoaded {
			n, err := s.store.CountActiveSubscriptions(ctx, p.ID)
			if err != nil {
				return 0, err
			}
			activeCount, activeCountLoaded = n, true
		}
		return activeCount, nil
	}
	if p.AuthType == "oauth2" && old.AuthType != "oauth2" {
		// Migrating an in-use product to oauth2 reprovisions the route with only
		// its OIDC-registered subscribers as the allowed whitelist. Any active
		// subscriber (e.g. still on key-auth) who hasn't registered a client id
		// yet would be silently dropped from that whitelist — or, if none are
		// ready, the whole route is deleted outright (see reprovisionRoute).
		// Refuse the change before it's persisted unless EVERY active
		// subscriber is already OAuth2-ready, so the migration never strands
		// any of them, and the product and its route never disagree about
		// auth_type.
		n, err := loadActiveCount()
		if err != nil {
			return Product{}, err
		}
		if n > 0 {
			ready, err := s.prov.OAuthReadyConsumerCount(ctx, p.ID)
			if err != nil {
				return Product{}, err
			}
			if ready < n {
				return Product{}, ErrOAuthMigrationBlocked
			}
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
		n, err := loadActiveCount()
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

// GetIcon returns a product's stored custom icon (admin preview; any publish state).
func (s *Service) GetIcon(ctx context.Context, productID int64) ([]byte, time.Time, error) {
	return s.store.GetIcon(ctx, productID)
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
