package catalog

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeLister implements the Lister interface without a database.
type fakeLister struct{ items []Product }

func (f fakeLister) List(_ contextStub, q Query) ([]Product, error) {
	if q.Category == "Finance" {
		return f.items[:1], nil
	}
	return f.items, nil
}
func (f fakeLister) GetBySlug(_ contextStub, slug string) (Product, error) {
	for _, p := range f.items {
		if p.Slug == slug {
			return p, nil
		}
	}
	return Product{}, ErrNotFound
}

func TestProductsEndpointReturnsJSON(t *testing.T) {
	h := NewHandler(fakeLister{items: []Product{{Name: "A", Slug: "a"}, {Name: "B", Slug: "b"}}})
	req := httptest.NewRequest(http.MethodGet, "/api/products", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got []Product
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d products, want 2", len(got))
	}
}

func TestProductsEndpointFiltersByCategory(t *testing.T) {
	h := NewHandler(fakeLister{items: []Product{{Slug: "a"}, {Slug: "b"}}})
	req := httptest.NewRequest(http.MethodGet, "/api/products?category=Finance", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var got []Product
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got) != 1 {
		t.Fatalf("got %d, want 1 filtered", len(got))
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
