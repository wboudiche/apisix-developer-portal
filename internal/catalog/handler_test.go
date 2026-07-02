package catalog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"apisix-portal/internal/paging"
)

// fakeLister implements the Lister interface without a database.
type fakeLister struct {
	items     []Product
	err       error
	specs     map[string]string
	changelog map[string][]ChangelogEntry
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
func (f fakeLister) GetSpecBySlug(_ context.Context, slug string) (string, error) {
	s, ok := f.specs[slug]
	if !ok {
		return "", ErrNotFound
	}
	return s, nil
}
func (f fakeLister) ListChangelogBySlug(_ context.Context, slug string) ([]ChangelogEntry, error) {
	entries, ok := f.changelog[slug]
	if !ok {
		return nil, ErrNotFound
	}
	return entries, nil
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

func TestGetSpecBySlug(t *testing.T) {
	f := fakeLister{specs: map[string]string{"orders": `{"openapi":"3.0.0"}`}}
	h := NewHandler(f)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/products/orders/spec", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != `{"openapi":"3.0.0"}` {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type=%q", ct)
	}

	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/api/products/missing/spec", nil))
	if rec2.Code != http.StatusNotFound {
		t.Fatalf("missing slug: status=%d", rec2.Code)
	}
}

func TestChangelogEndpoint(t *testing.T) {
	h := NewHandler(fakeLister{changelog: map[string][]ChangelogEntry{
		"cl-slug": {{Version: "v1.1", Kind: "fixed", Notes: "patch", Date: "2026-02-01"}},
	}})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/products/cl-slug/changelog", nil))
	if rec.Code != 200 {
		t.Fatalf("code=%d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "v1.1") {
		t.Errorf("body missing entry: %s", rec.Body.String())
	}
}
