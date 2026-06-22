package plans

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"apisix-portal/internal/paging"
)

type fakeLister struct {
	items []Plan
	err   error
}

func (f fakeLister) List(_ context.Context, p paging.Params) ([]Plan, int, error) {
	return f.items, len(f.items), f.err
}

func TestPlansEndpoint(t *testing.T) {
	h := NewHandler(fakeLister{items: []Plan{{ID: 1, Name: "Free"}, {ID: 2, Name: "Gold"}}})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/plans", nil))
	if rec.Code != 200 {
		t.Fatalf("status=%d", rec.Code)
	}
	var got paging.Page[Plan]
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Total != 2 {
		t.Fatalf("want Total=2, got %d", got.Total)
	}
	if got.Page != 1 {
		t.Fatalf("want Page=1, got %d", got.Page)
	}
	if got.PageSize != 20 {
		t.Fatalf("want PageSize=20, got %d", got.PageSize)
	}
	if len(got.Items) != 2 {
		t.Fatalf("want 2 plans, got %d", len(got.Items))
	}
}
