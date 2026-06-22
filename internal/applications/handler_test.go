package applications

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"apisix-portal/internal/auth"
	"apisix-portal/internal/paging"
)

type fakeStore struct {
	apps   []Application
	nextID int64
	err    error
}

func (f *fakeStore) Create(_ context.Context, ownerID int64, name, desc string) (Application, error) {
	f.nextID++
	a := Application{ID: f.nextID, OwnerID: ownerID, Name: name, Description: desc}
	f.apps = append(f.apps, a)
	return a, nil
}
func (f *fakeStore) ListByOwner(_ context.Context, ownerID int64, p paging.Params) ([]Application, int, error) {
	var filtered []Application
	for _, a := range f.apps {
		if a.OwnerID == ownerID {
			filtered = append(filtered, a)
		}
	}
	return filtered, len(filtered), f.err
}

func withUser(r *http.Request, id int64) *http.Request {
	return r.WithContext(auth.WithUserID(r.Context(), id))
}

func TestCreateApplication(t *testing.T) {
	h := NewHandler(&fakeStore{}, nil)
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/applications", strings.NewReader(`{"name":"App1"}`)), 5)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}
	var a Application
	_ = json.Unmarshal(rec.Body.Bytes(), &a)
	if a.OwnerID != 5 || a.Name != "App1" {
		t.Fatalf("bad app: %+v", a)
	}
}

func TestCreateApplicationRequiresName(t *testing.T) {
	h := NewHandler(&fakeStore{}, nil)
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/applications", strings.NewReader(`{}`)), 5)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", rec.Code)
	}
}

func TestListApplicationsScopedToUser(t *testing.T) {
	store := &fakeStore{}
	h := NewHandler(store, nil)

	// Seed one app for the authenticated user (id=5) and one for a different user (id=99).
	h.ServeHTTP(httptest.NewRecorder(), withUser(httptest.NewRequest(http.MethodPost, "/api/applications", strings.NewReader(`{"name":"A"}`)), 5))
	h.ServeHTTP(httptest.NewRecorder(), withUser(httptest.NewRequest(http.MethodPost, "/api/applications", strings.NewReader(`{"name":"OtherUserApp"}`)), 99))

	// List as user 5 — must see only their own app.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, withUser(httptest.NewRequest(http.MethodGet, "/api/applications", nil), 5))
	var got paging.Page[Application]
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(got.Items) != 1 {
		t.Fatalf("user 5 should see 1 item, got %d", len(got.Items))
	}
	if got.Total != 1 {
		t.Fatalf("Total want 1, got %d", got.Total)
	}
	if got.Items[0].OwnerID != 5 {
		t.Fatalf("item ownerID want 5, got %d", got.Items[0].OwnerID)
	}
	// Cross-tenant guard: user 99's app must not appear.
	for _, item := range got.Items {
		if item.Name == "OtherUserApp" {
			t.Fatalf("cross-tenant leak: user 99's app visible to user 5")
		}
	}
	if got.Page != 1 {
		t.Fatalf("Page want 1, got %d", got.Page)
	}
	if got.PageSize != 20 {
		t.Fatalf("PageSize want 20, got %d", got.PageSize)
	}
}
