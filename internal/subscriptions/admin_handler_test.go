package subscriptions

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"apisix-portal/internal/paging"
)

type fakeAdminSvc struct {
	list       []AdminSubscriptionView
	gotFilter  string
	approved   []int64
	rejected   []int64
	approveErr error
	rejectErr  error
}

func (f *fakeAdminSvc) AdminSubscriptions(_ context.Context, statusFilter string, p paging.Params) ([]AdminSubscriptionView, int, error) {
	f.gotFilter = statusFilter
	return f.list, len(f.list), nil
}
func (f *fakeAdminSvc) Approve(_ context.Context, id int64) error {
	if f.approveErr != nil {
		return f.approveErr
	}
	f.approved = append(f.approved, id)
	return nil
}
func (f *fakeAdminSvc) Reject(_ context.Context, id int64) error {
	if f.rejectErr != nil {
		return f.rejectErr
	}
	f.rejected = append(f.rejected, id)
	return nil
}

func TestAdminListPassesStatusFilter(t *testing.T) {
	svc := &fakeAdminSvc{list: []AdminSubscriptionView{{ID: 1, Status: "pending"}}}
	h := NewAdminHandler(svc)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/subscriptions?status=pending", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}
	if svc.gotFilter != "pending" {
		t.Fatalf("status filter = %q, want pending", svc.gotFilter)
	}
	var page paging.Page[AdminSubscriptionView]
	if err := json.NewDecoder(rec.Body).Decode(&page); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if page.Total != 1 {
		t.Fatalf("total = %d, want 1", page.Total)
	}
	if page.Page != 1 {
		t.Fatalf("page = %d, want 1", page.Page)
	}
	if page.PageSize != paging.DefaultPageSize {
		t.Fatalf("pageSize = %d, want %d", page.PageSize, paging.DefaultPageSize)
	}
	if len(page.Items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(page.Items))
	}
}

func TestAdminApprove(t *testing.T) {
	svc := &fakeAdminSvc{}
	h := NewAdminHandler(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/subscriptions/7/approve", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status=%d want 204 (body %s)", rec.Code, rec.Body)
	}
	if len(svc.approved) != 1 || svc.approved[0] != 7 {
		t.Fatalf("approved = %v, want [7]", svc.approved)
	}
}

func TestAdminReject(t *testing.T) {
	svc := &fakeAdminSvc{}
	h := NewAdminHandler(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/subscriptions/9/reject", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status=%d want 204", rec.Code)
	}
	if len(svc.rejected) != 1 || svc.rejected[0] != 9 {
		t.Fatalf("rejected = %v, want [9]", svc.rejected)
	}
}

func TestAdminApproveNotFoundReturns404(t *testing.T) {
	svc := &fakeAdminSvc{approveErr: ErrNotFound}
	h := NewAdminHandler(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/subscriptions/99/approve", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", rec.Code)
	}
}

func TestAdminRejectNotFoundReturns404(t *testing.T) {
	svc := &fakeAdminSvc{rejectErr: ErrNotFound}
	h := NewAdminHandler(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/subscriptions/99/reject", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", rec.Code)
	}
}

func TestAdminApproveBadIDReturns400(t *testing.T) {
	svc := &fakeAdminSvc{}
	h := NewAdminHandler(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/subscriptions/abc/approve", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", rec.Code)
	}
}

func TestAdminApproveInvalidTransitionReturns409(t *testing.T) {
	svc := &fakeAdminSvc{approveErr: ErrInvalidTransition}
	h := NewAdminHandler(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/subscriptions/5/approve", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status=%d want 409", rec.Code)
	}
}

func TestAdminRejectInvalidTransitionReturns409(t *testing.T) {
	svc := &fakeAdminSvc{rejectErr: ErrInvalidTransition}
	h := NewAdminHandler(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/subscriptions/5/reject", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status=%d want 409", rec.Code)
	}
}
