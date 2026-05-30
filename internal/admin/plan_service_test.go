package admin

import (
	"context"
	"errors"
	"testing"
)

type fakePlanStore struct {
	plans   map[int64]Plan
	counts  map[int64]int
	deleted map[int64]bool
}

func newFakePlanStore() *fakePlanStore {
	return &fakePlanStore{plans: map[int64]Plan{}, counts: map[int64]int{}, deleted: map[int64]bool{}}
}

func (f *fakePlanStore) ListPlans(_ context.Context) ([]Plan, error) {
	out := []Plan{}
	for _, p := range f.plans {
		out = append(out, p)
	}
	return out, nil
}
func (f *fakePlanStore) GetPlan(_ context.Context, id int64) (Plan, error) {
	p, ok := f.plans[id]
	if !ok {
		return Plan{}, ErrPlanNotFound
	}
	return p, nil
}
func (f *fakePlanStore) CreatePlan(_ context.Context, p Plan) (Plan, error) {
	p.ID = int64(len(f.plans) + 1)
	f.plans[p.ID] = p
	return p, nil
}
func (f *fakePlanStore) UpdatePlan(_ context.Context, p Plan) (Plan, error) {
	if _, ok := f.plans[p.ID]; !ok {
		return Plan{}, ErrPlanNotFound
	}
	f.plans[p.ID] = p
	return p, nil
}
func (f *fakePlanStore) DeletePlan(_ context.Context, id int64) error {
	if _, ok := f.plans[id]; !ok {
		return ErrPlanNotFound
	}
	delete(f.plans, id)
	f.deleted[id] = true
	return nil
}
func (f *fakePlanStore) CountSubscriptionsForPlan(_ context.Context, id int64) (int, error) {
	return f.counts[id], nil
}

type fakePlanProv struct{ reprovisioned []int64 }

func (f *fakePlanProv) ReprovisionPlan(_ context.Context, id int64) error {
	f.reprovisioned = append(f.reprovisioned, id)
	return nil
}

func TestPlanUpdateReprovisionsWhenLimitsChange(t *testing.T) {
	store := newFakePlanStore()
	store.plans[1] = Plan{ID: 1, Name: "Silver", RateLimit: 100, WindowSeconds: 60}
	prov := &fakePlanProv{}
	svc := NewPlanService(store, prov)

	upd := Plan{ID: 1, Name: "Silver", RateLimit: 200, WindowSeconds: 60}
	if _, err := svc.Update(context.Background(), upd); err != nil {
		t.Fatal(err)
	}
	if len(prov.reprovisioned) != 1 || prov.reprovisioned[0] != 1 {
		t.Fatalf("expected reprovision of plan 1, got %v", prov.reprovisioned)
	}
}

func TestPlanUpdateNoReprovisionWhenOnlyNameChanges(t *testing.T) {
	store := newFakePlanStore()
	store.plans[1] = Plan{ID: 1, Name: "Silver", RateLimit: 100, WindowSeconds: 60}
	prov := &fakePlanProv{}
	svc := NewPlanService(store, prov)

	upd := Plan{ID: 1, Name: "Argent", RateLimit: 100, WindowSeconds: 60}
	if _, err := svc.Update(context.Background(), upd); err != nil {
		t.Fatal(err)
	}
	if len(prov.reprovisioned) != 0 {
		t.Fatalf("expected no reprovision on name-only change, got %v", prov.reprovisioned)
	}
}

func TestPlanDeleteBlockedWhenInUse(t *testing.T) {
	store := newFakePlanStore()
	store.plans[1] = Plan{ID: 1, Name: "Silver", RateLimit: 100, WindowSeconds: 60}
	store.counts[1] = 3
	prov := &fakePlanProv{}
	svc := NewPlanService(store, prov)

	err := svc.Delete(context.Background(), 1)
	if !errors.Is(err, ErrPlanInUse) {
		t.Fatalf("err = %v, want ErrPlanInUse", err)
	}
	if store.deleted[1] {
		t.Fatal("plan should not have been deleted")
	}
}

func TestPlanDeleteSucceedsWhenUnused(t *testing.T) {
	store := newFakePlanStore()
	store.plans[1] = Plan{ID: 1, Name: "Silver", RateLimit: 100, WindowSeconds: 60}
	store.counts[1] = 0
	prov := &fakePlanProv{}
	svc := NewPlanService(store, prov)

	if err := svc.Delete(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if !store.deleted[1] {
		t.Fatal("plan should have been deleted")
	}
}
