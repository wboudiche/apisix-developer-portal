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

type fakePlanService struct {
	plans     map[int64]Plan
	createErr error
	updateErr error
	deleteErr error
}

func (f *fakePlanService) List(_ context.Context, _ paging.Params) ([]Plan, int, error) {
	out := []Plan{}
	for _, p := range f.plans {
		out = append(out, p)
	}
	return out, len(out), nil
}
func (f *fakePlanService) Create(_ context.Context, p Plan) (Plan, error) {
	if f.createErr != nil {
		return Plan{}, f.createErr
	}
	p.ID = 1
	return p, nil
}
func (f *fakePlanService) Update(_ context.Context, p Plan) (Plan, error) {
	if f.updateErr != nil {
		return Plan{}, f.updateErr
	}
	return p, nil
}
func (f *fakePlanService) Delete(_ context.Context, id int64) error { return f.deleteErr }

func doPlan(h *PlanHandler, method, target string, body any) *httptest.ResponseRecorder {
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

func TestPlanCreateValid(t *testing.T) {
	h := NewPlanHandler(&fakePlanService{plans: map[int64]Plan{}})
	rec := doPlan(h, http.MethodPost, "/api/admin/plans",
		Plan{Name: "Gold", RateLimit: 500, WindowSeconds: 60, Currency: "EUR"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestPlanCreateInvalidReturns400(t *testing.T) {
	h := NewPlanHandler(&fakePlanService{plans: map[int64]Plan{}})
	rec := doPlan(h, http.MethodPost, "/api/admin/plans", Plan{Name: "Bad", RateLimit: 0, WindowSeconds: 60})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestPlanList(t *testing.T) {
	h := NewPlanHandler(&fakePlanService{plans: map[int64]Plan{1: {ID: 1, Name: "Free", RateLimit: 10, WindowSeconds: 60}}})
	rec := doPlan(h, http.MethodGet, "/api/admin/plans", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got paging.Page[Plan]
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

func TestPlanCreateNameTakenReturns409(t *testing.T) {
	h := NewPlanHandler(&fakePlanService{plans: map[int64]Plan{}, createErr: ErrPlanNameTaken})
	rec := doPlan(h, http.MethodPost, "/api/admin/plans",
		Plan{Name: "Silver", RateLimit: 100, WindowSeconds: 60, Currency: "EUR"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
}

func TestPlanUpdateNotFoundReturns404(t *testing.T) {
	h := NewPlanHandler(&fakePlanService{plans: map[int64]Plan{}, updateErr: ErrPlanNotFound})
	rec := doPlan(h, http.MethodPut, "/api/admin/plans/9",
		Plan{Name: "Silver", RateLimit: 100, WindowSeconds: 60, Currency: "EUR"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestPlanDeleteInUseReturns409(t *testing.T) {
	h := NewPlanHandler(&fakePlanService{plans: map[int64]Plan{1: {ID: 1}}, deleteErr: ErrPlanInUse})
	rec := doPlan(h, http.MethodDelete, "/api/admin/plans/1", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
}

func TestPlanDeleteSuccessReturns204(t *testing.T) {
	h := NewPlanHandler(&fakePlanService{plans: map[int64]Plan{1: {ID: 1}}})
	rec := doPlan(h, http.MethodDelete, "/api/admin/plans/1", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
}
