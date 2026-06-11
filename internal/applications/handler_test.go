package applications

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"apisix-portal/internal/auth"
)

type fakeStore struct {
	apps   []Application
	nextID int64
}

func (f *fakeStore) Create(_ context.Context, ownerID int64, name, desc string) (Application, error) {
	f.nextID++
	a := Application{ID: f.nextID, OwnerID: ownerID, Name: name, Description: desc}
	f.apps = append(f.apps, a)
	return a, nil
}
func (f *fakeStore) ListByOwner(_ context.Context, ownerID int64) ([]Application, error) {
	var out []Application
	for _, a := range f.apps {
		if a.OwnerID == ownerID {
			out = append(out, a)
		}
	}
	return out, nil
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
	h.ServeHTTP(httptest.NewRecorder(), withUser(httptest.NewRequest(http.MethodPost, "/api/applications", strings.NewReader(`{"name":"A"}`)), 5))
	h.ServeHTTP(httptest.NewRecorder(), withUser(httptest.NewRequest(http.MethodPost, "/api/applications", strings.NewReader(`{"name":"B"}`)), 9))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, withUser(httptest.NewRequest(http.MethodGet, "/api/applications", nil), 5))
	var got []Application
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got) != 1 {
		t.Fatalf("user 5 should see 1 app, got %d", len(got))
	}
}
