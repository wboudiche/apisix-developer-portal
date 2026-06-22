package admin

import (
	"context"
	"errors"
	"testing"

	"apisix-portal/internal/paging"
)

type fakeStore struct {
	products map[int64]Product
	counts   map[int64]int
	deleted  map[int64]bool
}

func newFakeStore() *fakeStore {
	return &fakeStore{products: map[int64]Product{}, counts: map[int64]int{}, deleted: map[int64]bool{}}
}

func (f *fakeStore) ListAll(_ context.Context, _ paging.Params) ([]Product, int, error) {
	out := []Product{}
	for _, p := range f.products {
		out = append(out, p)
	}
	return out, len(out), nil
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
func (f *fakeStore) ContextPathOverlaps(_ context.Context, p string, exceptID int64) (bool, error) {
	for id, prod := range f.products {
		if id == exceptID {
			continue
		}
		if prod.ContextPath == p {
			return true, nil
		}
		if len(p) > len(prod.ContextPath) && p[:len(prod.ContextPath)+1] == prod.ContextPath+"/" {
			return true, nil
		}
		if len(prod.ContextPath) > len(p) && prod.ContextPath[:len(p)+1] == p+"/" {
			return true, nil
		}
	}
	return false, nil
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

func TestContextPathOverlapBlocksCreate(t *testing.T) {
	store := newFakeStore()
	// existing product at /v1
	store.products[1] = Product{ID: 1, Name: "A", Slug: "a", Category: "C", ContextPath: "/v1"}
	prov := &fakeProv{}
	svc := NewService(store, prov)

	// /v1/orders is a sub-path of /v1 — must be blocked (APISIX /v1/* would shadow /v1/orders/*)
	_, err := svc.Create(context.Background(), Product{
		Name: "B", Slug: "b", Category: "C", ContextPath: "/v1/orders",
	})
	if !errors.Is(err, ErrContextPathTaken) {
		t.Fatalf("create /v1/orders with existing /v1: got %v, want ErrContextPathTaken", err)
	}

	// /v1beta shares a prefix with /v1 in string terms but NOT on a "/" boundary — must be allowed
	_, err = svc.Create(context.Background(), Product{
		Name: "C", Slug: "c", Category: "C", ContextPath: "/v1beta",
	})
	if errors.Is(err, ErrContextPathTaken) {
		t.Fatal("create /v1beta with existing /v1: must not be blocked (no '/' boundary overlap)")
	}
}

func TestContextPathOverlapBlocksUpdate(t *testing.T) {
	store := newFakeStore()
	store.products[1] = Product{ID: 1, Name: "A", Slug: "a", Category: "C", ContextPath: "/v1"}
	store.products[2] = Product{ID: 2, Name: "B", Slug: "b", Category: "C", ContextPath: "/v2"}
	prov := &fakeProv{}
	svc := NewService(store, prov)

	// update product 2 to use /v1/orders — overlaps /v1
	updated := store.products[2]
	updated.ContextPath = "/v1/orders"
	_, err := svc.Update(context.Background(), updated)
	if !errors.Is(err, ErrContextPathTaken) {
		t.Fatalf("update to /v1/orders with existing /v1: got %v, want ErrContextPathTaken", err)
	}

	// update product 2 to keep same path — must not be blocked (exceptID is the product itself)
	same := store.products[2]
	_, err = svc.Update(context.Background(), same)
	if err != nil {
		t.Fatalf("update product with unchanged contextPath: got %v, want nil", err)
	}
}
