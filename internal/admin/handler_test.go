package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"apisix-portal/internal/paging"
)

// fakeService implements ProductService for handler tests.
type fakeService struct {
	products  map[int64]Product
	createErr error
	updateErr error
	deleteErr error
}

func (f *fakeService) List(_ context.Context, _ paging.Params) ([]Product, int, error) {
	out := []Product{}
	for _, p := range f.products {
		out = append(out, p)
	}
	return out, len(out), nil
}
func (f *fakeService) Get(_ context.Context, id int64) (Product, error) {
	p, ok := f.products[id]
	if !ok {
		return Product{}, ErrNotFound
	}
	return p, nil
}
func (f *fakeService) Create(_ context.Context, p Product) (Product, error) {
	if f.createErr != nil {
		return Product{}, f.createErr
	}
	p.ID = 1
	return p, nil
}
func (f *fakeService) Update(_ context.Context, p Product) (Product, error) {
	if f.updateErr != nil {
		return Product{}, f.updateErr
	}
	return p, nil
}
func (f *fakeService) Delete(_ context.Context, id int64) error { return f.deleteErr }

func newTestHandler(svc ProductService) *Handler { return NewHandler(svc, true) }

func do(h *Handler, method, target string, body any) *httptest.ResponseRecorder {
	var rdr *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, target, rdr)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestCreateValid(t *testing.T) {
	h := newTestHandler(&fakeService{products: map[int64]Product{}})
	rec := do(h, http.MethodPost, "/api/admin/products",
		Product{Name: "Pizza", Slug: "pizza", Category: "Food", ContextPath: "/pizza"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestCreateInvalidReturns400(t *testing.T) {
	h := newTestHandler(&fakeService{products: map[int64]Product{}})
	rec := do(h, http.MethodPost, "/api/admin/products", Product{Slug: "x", Category: "c", ContextPath: "/x"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestList(t *testing.T) {
	h := newTestHandler(&fakeService{products: map[int64]Product{1: {ID: 1, Name: "A"}}})
	rec := do(h, http.MethodGet, "/api/admin/products", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got paging.Page[Product]
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body not a paging envelope: %v", err)
	}
	if got.Total != 1 {
		t.Fatalf("Total = %d, want 1", got.Total)
	}
	if got.Page != 1 {
		t.Fatalf("Page = %d, want 1", got.Page)
	}
	if got.PageSize != paging.DefaultPageSize {
		t.Fatalf("PageSize = %d, want %d", got.PageSize, paging.DefaultPageSize)
	}
	if len(got.Items) != 1 {
		t.Fatalf("len(Items) = %d, want 1", len(got.Items))
	}
}

func TestGetUnknownReturns404(t *testing.T) {
	h := newTestHandler(&fakeService{products: map[int64]Product{}})
	rec := do(h, http.MethodGet, "/api/admin/products/99", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestDeleteWithActiveSubsReturns409(t *testing.T) {
	h := newTestHandler(&fakeService{products: map[int64]Product{1: {ID: 1}}, deleteErr: ErrHasSubscriptions})
	rec := do(h, http.MethodDelete, "/api/admin/products/1", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
}

func TestUpdateSlugTakenReturns409(t *testing.T) {
	h := newTestHandler(&fakeService{products: map[int64]Product{1: {ID: 1}}, updateErr: ErrSlugTaken})
	rec := do(h, http.MethodPut, "/api/admin/products/1",
		Product{Name: "Pizza", Slug: "dup", Category: "Food", ContextPath: "/pizza"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
}

func TestDeleteSuccessReturns204(t *testing.T) {
	h := newTestHandler(&fakeService{products: map[int64]Product{1: {ID: 1}}})
	rec := do(h, http.MethodDelete, "/api/admin/products/1", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
}

func TestCreateContextPathTakenReturns409(t *testing.T) {
	h := newTestHandler(&fakeService{products: map[int64]Product{}, createErr: ErrContextPathTaken})
	rec := do(h, http.MethodPost, "/api/admin/products",
		Product{Name: "Pizza", Slug: "pizza", Category: "Food", ContextPath: "/pizza"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
}

func TestUpdateContextPathTakenReturns409(t *testing.T) {
	h := newTestHandler(&fakeService{products: map[int64]Product{1: {ID: 1}}, updateErr: ErrContextPathTaken})
	rec := do(h, http.MethodPut, "/api/admin/products/1",
		Product{Name: "Pizza", Slug: "pizza", Category: "Food", ContextPath: "/pizza"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
}
