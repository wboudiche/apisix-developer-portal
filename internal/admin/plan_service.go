package admin

import (
	"context"

	"apisix-portal/internal/paging"
)

// PlanStore is the persistence surface the plan service needs (satisfied by *PlanRepo).
type PlanStore interface {
	ListPlans(ctx context.Context, p paging.Params) ([]Plan, int, error)
	GetPlan(ctx context.Context, id int64) (Plan, error)
	CreatePlan(ctx context.Context, p Plan) (Plan, error)
	UpdatePlan(ctx context.Context, p Plan) (Plan, error)
	DeletePlan(ctx context.Context, id int64) error
	CountSubscriptionsForPlan(ctx context.Context, planID int64) (int, error)
}

// PlanProvisioner re-applies a plan's limits to live consumers (satisfied by
// *subscriptions.Service via ReprovisionPlan).
type PlanProvisioner interface {
	ReprovisionPlan(ctx context.Context, planID int64) error
}

// PlanService applies admin plan operations and keeps APISIX consumers in sync.
type PlanService struct {
	store PlanStore
	prov  PlanProvisioner
}

func NewPlanService(store PlanStore, prov PlanProvisioner) *PlanService {
	return &PlanService{store: store, prov: prov}
}

func (s *PlanService) List(ctx context.Context, p paging.Params) ([]Plan, int, error) {
	return s.store.ListPlans(ctx, p)
}

func (s *PlanService) Create(ctx context.Context, p Plan) (Plan, error) {
	return s.store.CreatePlan(ctx, p)
}

// Update persists changes and, when the rate limits changed, re-applies them to
// every active subscriber's consumer. A name-only edit triggers no provisioning.
func (s *PlanService) Update(ctx context.Context, p Plan) (Plan, error) {
	old, err := s.store.GetPlan(ctx, p.ID)
	if err != nil {
		return Plan{}, err
	}
	updated, err := s.store.UpdatePlan(ctx, p)
	if err != nil {
		return Plan{}, err
	}
	if updated.RateLimit != old.RateLimit || updated.WindowSeconds != old.WindowSeconds {
		if err := s.prov.ReprovisionPlan(ctx, p.ID); err != nil {
			return Plan{}, err
		}
	}
	return updated, nil
}

// Delete refuses (ErrPlanInUse) while any subscription references the plan;
// otherwise it removes the plan.
func (s *PlanService) Delete(ctx context.Context, id int64) error {
	n, err := s.store.CountSubscriptionsForPlan(ctx, id)
	if err != nil {
		return err
	}
	if n > 0 {
		return ErrPlanInUse
	}
	return s.store.DeletePlan(ctx, id)
}
