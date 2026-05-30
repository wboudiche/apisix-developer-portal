package subscriptions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeAdminSvc struct {
	list       []AdminSubscriptionView
	gotFilter  string
	approved   []int64
	rejected   []int64
	approveErr error
	rejectErr  error
}

func (f *fakeAdminSvc) AdminSubscriptions(_ context.Context, statusFilter string) ([]AdminSubscriptionView, error) {
	f.gotFilter = statusFilter
	return f.list, nil
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
