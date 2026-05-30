package plans

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeLister struct{ items []Plan }

func (f fakeLister) List(_ context.Context) ([]Plan, error) { return f.items, nil }

func TestPlansEndpoint(t *testing.T) {
	h := NewHandler(fakeLister{items: []Plan{{ID: 1, Name: "Free"}, {ID: 2, Name: "Gold"}}})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/plans", nil))
	if rec.Code != 200 {
		t.Fatalf("status=%d", rec.Code)
	}
	var got []Plan
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got) != 2 {
		t.Fatalf("want 2 plans, got %d", len(got))
	}
}
