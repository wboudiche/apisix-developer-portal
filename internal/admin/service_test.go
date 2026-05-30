package admin

import (
	"context"
	"errors"
	"testing"
)

type fakeStore struct {
	products map[int64]Product
	counts   map[int64]int
	deleted  map[int64]bool
}

func newFakeStore() *fakeStore {
	return &fakeStore{products: map[int64]Product{}, counts: map[int64]int{}, deleted: map[int64]bool{}}
}

func (f *fakeStore) ListAll(_ context.Context) ([]Product, error) {
	out := []Product{}
	for _, p := range f.products {
		out = append(out, p)
	}
	return out, nil
}
func (f *fakeStore) Get(_ context.Context, id int64) (Product, error) {
	p, ok := f.products[id]
	if !ok {
		return Product{}, ErrNotFound
	}
	return p, nil
}
func (f *fakeStore) Create(_ context.Context, p Product) (Product, error) {
	p.ID = int64(len(f.products) + 1)
	f.products[p.ID] = p
	return p, nil
}
func (f *fakeStore) Update(_ context.Context, p Product) (Product, error) {
	if _, ok := f.products[p.ID]; !ok {
		return Product{}, ErrNotFound
	}
	f.products[p.ID] = p
	return p, nil
}
func (f *fakeStore) Delete(_ context.Context, id int64) error {
	if _, ok := f.products[id]; !ok {
		return ErrNotFound
	}
	delete(f.products, id)
	f.deleted[id] = true
	return nil
}
func (f *fakeStore) CountActiveSubscriptions(_ context.Context, id int64) (int, error) {
	return f.counts[id], nil
}

type fakeProv struct {
	reprovisioned []int64
	deprovisioned []int64
}

func (f *fakeProv) ReprovisionRoute(_ context.Context, id int64) error {
	f.reprovisioned = append(f.reprovisioned, id)
	return nil
}
func (f *fakeProv) DeprovisionRoute(_ context.Context, id int64) error {
	f.deprovisioned = append(f.deprovisioned, id)
	return nil
}

func TestUpdateReprovisionsWhenUpstreamChangesAndHasSubs(t *testing.T) {
	store := newFakeStore()
	store.products[1] = Product{ID: 1, Name: "P", Slug: "p", Category: "C", ContextPath: "/p", UpstreamURL: "old:8080"}
	store.counts[1] = 2
	prov := &fakeProv{}
	svc := NewService(store, prov)

	updated := store.products[1]
	updated.UpstreamURL = "new:9090"
	if _, err := svc.Update(context.Background(), updated); err != nil {
		t.Fatal(err)
	}
	if len(prov.reprovisioned) != 1 || prov.reprovisioned[0] != 1 {
		t.Fatalf("expected reprovision of product 1, got %v", prov.reprovisioned)
	}
}

func TestUpdateNoReprovisionWhenUpstreamUnchanged(t *testing.T) {
	store := newFakeStore()
	store.products[1] = Product{ID: 1, Name: "P", Slug: "p", Category: "C", ContextPath: "/p", UpstreamURL: "same:8080"}
	store.counts[1] = 5
	prov := &fakeProv{}
	svc := NewService(store, prov)

	updated := store.products[1]
	updated.Description = "changed text only"
	if _, err := svc.Update(context.Background(), updated); err != nil {
		t.Fatal(err)
	}
	if len(prov.reprovisioned) != 0 {
		t.Fatalf("expected no reprovision, got %v", prov.reprovisioned)
	}
}

func TestUpdateNoReprovisionWhenNoSubs(t *testing.T) {
	store := newFakeStore()
	store.products[1] = Product{ID: 1, Name: "P", Slug: "p", Category: "C", ContextPath: "/p", UpstreamURL: "old:8080"}
	store.counts[1] = 0
	prov := &fakeProv{}
	svc := NewService(store, prov)

	updated := store.products[1]
	updated.UpstreamURL = "new:9090"
	if _, err := svc.Update(context.Background(), updated); err != nil {
		t.Fatal(err)
	}
	if len(prov.reprovisioned) != 0 {
		t.Fatalf("expected no reprovision (no active subs), got %v", prov.reprovisioned)
	}
}

func TestDeleteBlockedByActiveSubs(t *testing.T) {
	store := newFakeStore()
	store.products[1] = Product{ID: 1, Name: "P", Slug: "p", Category: "C", ContextPath: "/p"}
	store.counts[1] = 1
	prov := &fakeProv{}
	svc := NewService(store, prov)

	err := svc.Delete(context.Background(), 1)
	if !errors.Is(err, ErrHasSubscriptions) {
		t.Fatalf("err = %v, want ErrHasSubscriptions", err)
	}
	if store.deleted[1] {
		t.Fatal("product should not have been deleted")
	}
	if len(prov.deprovisioned) != 0 {
		t.Fatalf("should not deprovision a blocked delete, got %v", prov.deprovisioned)
	}
}

func TestDeleteTearsDownRouteWhenNoSubs(t *testing.T) {
	store := newFakeStore()
	store.products[1] = Product{ID: 1, Name: "P", Slug: "p", Category: "C", ContextPath: "/p"}
	store.counts[1] = 0
	prov := &fakeProv{}
	svc := NewService(store, prov)

	if err := svc.Delete(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if !store.deleted[1] {
		t.Fatal("product should have been deleted")
	}
	if len(prov.deprovisioned) != 1 || prov.deprovisioned[0] != 1 {
		t.Fatalf("expected deprovision of product 1, got %v", prov.deprovisioned)
	}
}
