package billing_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"apisix-portal/internal/auth"
	"apisix-portal/internal/billing"
)

// withUser attaches uid to the request context via auth.WithUserID, as
// RequireAuth/RequireAdmin would after parsing a real bearer token.
func withUser(r *http.Request, uid int64) *http.Request {
	return r.WithContext(auth.WithUserID(r.Context(), uid))
}

func TestTeamHandlerListMineScopedToCallerTeams(t *testing.T) {
	pool := dial(t)
	ctx := context.Background()
	_, appID, subID := seedTeamAppSub(t, pool)
	planID := seedPlan(t, pool, planName("Gold"), 2900, "EUR")
	linkSubToPlan(t, pool, subID, planID)

	svc := billing.NewService(billing.NewRepo(pool), billing.ManualProvider{})
	if err := svc.SubscriptionActivated(ctx, appID, subID, planID); err != nil {
		t.Fatalf("activate: %v", err)
	}
	inv := onlyInvoiceForSub(t, pool, subID)

	var memberUID int64
	if err := pool.QueryRow(ctx, `SELECT owner_id FROM applications WHERE id=$1`, appID).Scan(&memberUID); err != nil {
		t.Fatalf("find app owner: %v", err)
	}

	// A user with no team membership at all (real row, but no team_members row).
	var outsiderUID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users(email,password_hash,name) VALUES($1,'x','Outsider') RETURNING id`,
		"billing-outsider+"+time.Now().Format("150405.000000000")+"@example.com").Scan(&outsiderUID); err != nil {
		t.Fatalf("seed outsider user: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, outsiderUID) })

	h := billing.NewTeamHandler(svc)

	// A user with no team membership sees an empty list, not an error.
	outsiderReq := withUser(httptest.NewRequest(http.MethodGet, "/api/billing/invoices", nil), outsiderUID)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, outsiderReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("outsider status=%d body=%s", rec.Code, rec.Body)
	}
	var outsiderInvoices []billing.Invoice
	if err := json.NewDecoder(rec.Body).Decode(&outsiderInvoices); err != nil {
		t.Fatalf("decode outsider body: %v", err)
	}
	if len(outsiderInvoices) != 0 {
		t.Fatalf("outsider invoices = %+v, want []", outsiderInvoices)
	}

	// The seeded team's member sees its invoice.
	memberReq := withUser(httptest.NewRequest(http.MethodGet, "/api/billing/invoices", nil), memberUID)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, memberReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("member status=%d body=%s", rec.Code, rec.Body)
	}
	var memberInvoices []billing.Invoice
	if err := json.NewDecoder(rec.Body).Decode(&memberInvoices); err != nil {
		t.Fatalf("decode member body: %v", err)
	}
	var found bool
	for _, v := range memberInvoices {
		if v.ID == inv.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("member invoices = %+v, want to include %d", memberInvoices, inv.ID)
	}
}

func TestTeamHandlerUnauthenticatedReturns401(t *testing.T) {
	pool := dial(t)
	svc := billing.NewService(billing.NewRepo(pool), billing.ManualProvider{})
	h := billing.NewTeamHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/billing/invoices", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401 (body %s)", rec.Code, rec.Body)
	}
}

func TestAdminHandlerListAllAndStatusFilter(t *testing.T) {
	pool := dial(t)
	ctx := context.Background()
	_, appID, subID := seedTeamAppSub(t, pool)
	planID := seedPlan(t, pool, planName("Gold"), 2900, "EUR")
	linkSubToPlan(t, pool, subID, planID)

	svc := billing.NewService(billing.NewRepo(pool), billing.ManualProvider{})
	if err := svc.SubscriptionActivated(ctx, appID, subID, planID); err != nil {
		t.Fatalf("activate: %v", err)
	}
	inv := onlyInvoiceForSub(t, pool, subID)

	h := billing.NewAdminHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/invoices", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}
	var all []billing.Invoice
	if err := json.NewDecoder(rec.Body).Decode(&all); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	var found bool
	for _, v := range all {
		if v.ID == inv.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("admin list = %+v, want to include %d", all, inv.ID)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/admin/invoices?status=pending", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}
	var pending []billing.Invoice
	if err := json.NewDecoder(rec.Body).Decode(&pending); err != nil {
		t.Fatalf("decode filtered body: %v", err)
	}
	found = false
	for _, v := range pending {
		if v.ID == inv.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("admin list(status=pending) = %+v, want to include %d", pending, inv.ID)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/admin/invoices?status=void", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}
	var void []billing.Invoice
	if err := json.NewDecoder(rec.Body).Decode(&void); err != nil {
		t.Fatalf("decode void-filtered body: %v", err)
	}
	for _, v := range void {
		if v.ID == inv.ID {
			t.Fatalf("admin list(status=void) unexpectedly includes pending invoice %d", inv.ID)
		}
	}
}

func TestAdminHandlerPayThenSecondPayConflicts(t *testing.T) {
	pool := dial(t)
	ctx := context.Background()
	_, appID, subID := seedTeamAppSub(t, pool)
	planID := seedPlan(t, pool, planName("Gold"), 2900, "EUR")
	linkSubToPlan(t, pool, subID, planID)

	svc := billing.NewService(billing.NewRepo(pool), billing.ManualProvider{})
	if err := svc.SubscriptionActivated(ctx, appID, subID, planID); err != nil {
		t.Fatalf("activate: %v", err)
	}
	inv := onlyInvoiceForSub(t, pool, subID)

	h := billing.NewAdminHandler(svc)
	path := "/api/admin/invoices/" + strconv.FormatInt(inv.ID, 10) + "/pay"

	req := httptest.NewRequest(http.MethodPost, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first pay status=%d body=%s", rec.Code, rec.Body)
	}
	got, err := svc.Get(ctx, inv.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != billing.StatusPaid {
		t.Fatalf("status = %q, want paid", got.Status)
	}

	req = httptest.NewRequest(http.MethodPost, path, nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("second pay status=%d want 409 (body %s)", rec.Code, rec.Body)
	}
}

func TestAdminHandlerVoid(t *testing.T) {
	pool := dial(t)
	ctx := context.Background()
	_, appID, subID := seedTeamAppSub(t, pool)
	planID := seedPlan(t, pool, planName("Gold"), 2900, "EUR")
	linkSubToPlan(t, pool, subID, planID)

	svc := billing.NewService(billing.NewRepo(pool), billing.ManualProvider{})
	if err := svc.SubscriptionActivated(ctx, appID, subID, planID); err != nil {
		t.Fatalf("activate: %v", err)
	}
	inv := onlyInvoiceForSub(t, pool, subID)

	h := billing.NewAdminHandler(svc)
	path := "/api/admin/invoices/" + strconv.FormatInt(inv.ID, 10) + "/void"

	req := httptest.NewRequest(http.MethodPost, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("void status=%d body=%s", rec.Code, rec.Body)
	}
	got, err := svc.Get(ctx, inv.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != billing.StatusVoid {
		t.Fatalf("status = %q, want void", got.Status)
	}
}

func TestAdminHandlerPayNonexistentReturns404(t *testing.T) {
	pool := dial(t)
	svc := billing.NewService(billing.NewRepo(pool), billing.ManualProvider{})
	h := billing.NewAdminHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/invoices/9999999/pay", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404 (body %s)", rec.Code, rec.Body)
	}
}

func TestAdminHandlerPayBadIDReturns400(t *testing.T) {
	pool := dial(t)
	svc := billing.NewService(billing.NewRepo(pool), billing.ManualProvider{})
	h := billing.NewAdminHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/invoices/abc/pay", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 (body %s)", rec.Code, rec.Body)
	}
}
