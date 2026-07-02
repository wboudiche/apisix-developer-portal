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

	addChangelogErr    error
	deleteChangelogErr error
	listChangelogErr   error
	lastChangelog      ChangelogEntry
	lastChangelogPID   int64
	changelogEntries   map[int64][]ChangelogEntry
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

func (f *fakeService) AddChangelog(_ context.Context, productID int64, e ChangelogEntry) (ChangelogEntry, error) {
	if f.addChangelogErr != nil {
		return ChangelogEntry{}, f.addChangelogErr
	}
	f.lastChangelogPID = productID
	e.ID = 1
	f.lastChangelog = e
	return e, nil
}
func (f *fakeService) ListChangelog(_ context.Context, productID int64) ([]ChangelogEntry, error) {
	if f.listChangelogErr != nil {
		return nil, f.listChangelogErr
	}
	return f.changelogEntries[productID], nil
}
func (f *fakeService) DeleteChangelog(_ context.Context, productID, entryID int64) error {
	return f.deleteChangelogErr
}

func newTestHandler(svc ProductService) *Handler { return NewHandler(svc, true, false) }

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

func TestCreateProductStoresSpec(t *testing.T) {
	svc := &fakeService{products: map[int64]Product{}}
	h := NewHandler(svc, true, false)
	spec := `{"openapi":"3.0.0","info":{"title":"X","version":"1.0.0"}}`
	body, _ := json.Marshal(Product{
		Name: "X", Slug: "x", Category: "C", ContextPath: "/x", OpenAPISpec: spec,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/products", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var got Product
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.OpenAPISpec != spec {
		t.Errorf("spec not passed through: %q", got.OpenAPISpec)
	}
}

func TestCreateProductRejectsBrokenSpec(t *testing.T) {
	svc := &fakeService{products: map[int64]Product{}}
	h := NewHandler(svc, true, false)
	body, _ := json.Marshal(Product{
		Name: "X", Slug: "x", Category: "C", ContextPath: "/x", OpenAPISpec: "this is not a spec",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/products", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateOAuth2ProductReturns400WhenOIDCUnconfigured(t *testing.T) {
	svc := &fakeService{products: map[int64]Product{}}
	h := NewHandler(svc, true, false) // oidcConfigured=false
	rec := do(h, http.MethodPost, "/api/admin/products",
		Product{Name: "Orders", Slug: "orders", Category: "Commerce", ContextPath: "/orders", AuthType: "oauth2"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (OAuth2 without OIDC configured); body: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateOAuth2ProductReturns400WhenOIDCUnconfigured(t *testing.T) {
	svc := &fakeService{products: map[int64]Product{1: {ID: 1}}}
	h := NewHandler(svc, true, false) // oidcConfigured=false
	rec := do(h, http.MethodPut, "/api/admin/products/1",
		Product{Name: "Orders", Slug: "orders", Category: "Commerce", ContextPath: "/orders", AuthType: "oauth2"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (OAuth2 without OIDC configured); body: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateAcceptsLifecycleStatusAndSunsetDate(t *testing.T) {
	svc := &fakeService{products: map[int64]Product{}}
	h := newTestHandler(svc)
	sunset := "2026-12-31"
	rec := do(h, http.MethodPost, "/api/admin/products",
		Product{Name: "Pizza", Slug: "pizza", Category: "Food", ContextPath: "/pizza", LifecycleStatus: "sunset", SunsetDate: &sunset})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
	}
	var got Product
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body not a product: %v", err)
	}
	if got.LifecycleStatus != "sunset" || got.SunsetDate == nil || *got.SunsetDate != sunset {
		t.Fatalf("lifecycle fields not passed through: %+v", got)
	}
}

func TestCreateInvalidLifecycleStatusReturns400(t *testing.T) {
	h := newTestHandler(&fakeService{products: map[int64]Product{}})
	rec := do(h, http.MethodPost, "/api/admin/products",
		Product{Name: "Pizza", Slug: "pizza", Category: "Food", ContextPath: "/pizza", LifecycleStatus: "retired"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestCreateInvalidSunsetDateReturns400(t *testing.T) {
	h := newTestHandler(&fakeService{products: map[int64]Product{}})
	bad := "not-a-date"
	rec := do(h, http.MethodPost, "/api/admin/products",
		Product{Name: "Pizza", Slug: "pizza", Category: "Food", ContextPath: "/pizza", SunsetDate: &bad})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAddChangelogReturns201AndCallsService(t *testing.T) {
	svc := &fakeService{products: map[int64]Product{1: {ID: 1}}}
	h := newTestHandler(svc)
	rec := do(h, http.MethodPost, "/api/admin/products/1/changelog",
		ChangelogEntry{Version: "v2", Kind: "changed", Notes: "n", Date: "2026-03-01"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
	}
	if svc.lastChangelogPID != 1 {
		t.Fatalf("service not called with product id 1, got %d", svc.lastChangelogPID)
	}
	if svc.lastChangelog.Version != "v2" || svc.lastChangelog.Kind != "changed" {
		t.Fatalf("service called with unexpected entry: %+v", svc.lastChangelog)
	}
	var got ChangelogEntry
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body not a changelog entry: %v", err)
	}
	if got.ID == 0 {
		t.Fatalf("expected a generated id, got %+v", got)
	}
}

func TestAddChangelogInvalidKindReturns400(t *testing.T) {
	h := newTestHandler(&fakeService{products: map[int64]Product{1: {ID: 1}}})
	rec := do(h, http.MethodPost, "/api/admin/products/1/changelog",
		ChangelogEntry{Version: "v2", Kind: "bogus", Date: "2026-03-01"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAddChangelogInvalidDateReturns400(t *testing.T) {
	h := newTestHandler(&fakeService{products: map[int64]Product{1: {ID: 1}}})
	rec := do(h, http.MethodPost, "/api/admin/products/1/changelog",
		ChangelogEntry{Version: "v2", Kind: "changed", Date: "not-a-date"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestListChangelogReturnsEntriesForDraftProduct(t *testing.T) {
	entries := []ChangelogEntry{{ID: 11, Version: "1.0.0", Kind: "added", Notes: "Initial", Date: "2026-01-01"}}
	// Product is unpublished (a draft) — the admin listing must still see it,
	// unlike the public published-only changelog endpoint.
	svc := &fakeService{
		products:         map[int64]Product{1: {ID: 1, Published: false}},
		changelogEntries: map[int64][]ChangelogEntry{1: entries},
	}
	h := newTestHandler(svc)
	rec := do(h, http.MethodGet, "/api/admin/products/1/changelog", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var got []ChangelogEntry
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body not a changelog entry list: %v", err)
	}
	if len(got) != 1 || got[0].ID != 11 {
		t.Fatalf("got %+v, want the seeded entry", got)
	}
}

func TestListChangelogReturnsEmptyArrayNotNull(t *testing.T) {
	h := newTestHandler(&fakeService{products: map[int64]Product{1: {ID: 1}}})
	rec := do(h, http.MethodGet, "/api/admin/products/1/changelog", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); body != "[]\n" && body != "[]" {
		t.Fatalf("body = %q, want an empty JSON array", body)
	}
}

func TestDeleteChangelogReturns204(t *testing.T) {
	h := newTestHandler(&fakeService{products: map[int64]Product{1: {ID: 1}}})
	rec := do(h, http.MethodDelete, "/api/admin/products/1/changelog/5", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestDeleteChangelogNotFoundReturns404(t *testing.T) {
	h := newTestHandler(&fakeService{products: map[int64]Product{1: {ID: 1}}, deleteChangelogErr: ErrNotFound})
	rec := do(h, http.MethodDelete, "/api/admin/products/1/changelog/5", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
