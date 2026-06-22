package catalog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"apisix-portal/internal/paging"
)

// fakeLister implements the Lister interface without a database.
type fakeLister struct {
	items []Product
	err   error
}

func (f fakeLister) List(_ context.Context, q Query, p paging.Params) ([]Product, int, error) {
	if f.err != nil {
		return nil, 0, f.err
	}
	if q.Category == "Finance" {
		items := f.items[:1]
		return items, len(items), nil
	}
	return f.items, len(f.items), nil
}
func (f fakeLister) GetBySlug(_ context.Context, slug string) (Product, error) {
	for _, p := range f.items {
		if p.Slug == slug {
			return p, nil
		}
	}
	return Product{}, ErrNotFound
}

func TestProductsEndpointReturnsJSON(t *testing.T) {
	f := fakeLister{items: []Product{{Name: "A", Slug: "a"}, {Name: "B", Slug: "b"}}}
	h := NewHandler(f)
	req := httptest.NewRequest(http.MethodGet, "/api/products", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got paging.Page[Product]
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Total != len(f.items) || got.Page != 1 || got.PageSize != 20 {
		t.Fatalf("envelope meta wrong: %+v", got)
	}
	if len(got.Items) != len(f.items) {
		t.Fatalf("got %d items, want %d", len(got.Items), len(f.items))
	}
}

func TestProductsEndpointFiltersByCategory(t *testing.T) {
	f := fakeLister{items: []Product{{Slug: "a"}, {Slug: "b"}}}
	h := NewHandler(f)
	req := httptest.NewRequest(http.MethodGet, "/api/products?category=Finance", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var got paging.Page[Product]
	_ = json.NewDecoder(rec.Body).Decode(&got)
	if len(got.Items) != 1 {
		t.Fatalf("got %d, want 1 filtered", len(got.Items))
	}
}

func TestProductBySlugReturnsProduct(t *testing.T) {
	h := NewHandler(fakeLister{items: []Product{{Name: "Pizza", Slug: "pizza"}}})
	req := httptest.NewRequest(http.MethodGet, "/api/products/pizza", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got Product
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.Slug != "pizza" {
		t.Fatalf("unexpected body: %s err=%v", rec.Body, err)
	}
}

func TestProductBySlugNotFound(t *testing.T) {
	h := NewHandler(fakeLister{items: []Product{{Slug: "pizza"}}})
	req := httptest.NewRequest(http.MethodGet, "/api/products/missing", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
