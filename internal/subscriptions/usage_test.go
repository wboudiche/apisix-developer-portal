package subscriptions

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"apisix-portal/internal/auth"
	"apisix-portal/internal/metrics"
)

// fakeUsage records the consumer/range it was asked for and returns a scripted
// result or error.
type fakeUsage struct {
	gotConsumer string
	gotRange    string
	usage       metrics.Usage
	err         error
}

func (f *fakeUsage) Usage(_ context.Context, consumer string, r metrics.Range) (metrics.Usage, error) {
	f.gotConsumer = consumer
	f.gotRange = r.Key
	return f.usage, f.err
}
func (f *fakeUsage) RequestsInWindow(_ context.Context, _ string, _ int) (int64, error) {
	return 0, f.err
}

func newUsageHandler(reader Reader, usage UsageReader) *Handler {
	store := newMemStore()
	svc := NewService(store, nil, nil, func() string { return "k" }, nil)
	owns := func(_ context.Context, appID, userID int64) (bool, error) { return appID == 1 && userID == 5, nil }
	h := NewHandler(svc, reader, fakeEvents{}, owns, "")
	h.SetUsageReader(usage)
	return h
}

func usageReq(appID, userID int64, query string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/applications/1/usage"+query, nil)
	_ = appID
	return req.WithContext(auth.WithUserID(req.Context(), userID))
}

func TestUsageReturnsDataForOwnerScopedToConsumer(t *testing.T) {
	reader := fakeReader{has: true, cred: Credential{ApplicationID: 1, ConsumerUsername: "app_1"}}
	fu := &fakeUsage{usage: metrics.Usage{Summary: metrics.Summary{RequestsToday: 7}}}
	h := newUsageHandler(reader, fu)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, usageReq(1, 5, "?range=7d"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}
	if fu.gotConsumer != "app_1" {
		t.Errorf("queried consumer %q, want app_1", fu.gotConsumer)
	}
	if fu.gotRange != "7d" {
		t.Errorf("range %q, want 7d", fu.gotRange)
	}
	var got metrics.Usage
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Summary.RequestsToday != 7 {
		t.Errorf("body = %+v", got)
	}
}

func TestUsageDefaultsRangeWhenAbsent(t *testing.T) {
	reader := fakeReader{has: true, cred: Credential{ConsumerUsername: "app_1"}}
	fu := &fakeUsage{}
	h := newUsageHandler(reader, fu)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, usageReq(1, 5, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	if fu.gotRange != "24h" {
		t.Errorf("default range = %q, want 24h", fu.gotRange)
	}
}

func TestUsageRejectsBadRange(t *testing.T) {
	reader := fakeReader{has: true, cred: Credential{ConsumerUsername: "app_1"}}
	fu := &fakeUsage{}
	h := newUsageHandler(reader, fu)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, usageReq(1, 5, "?range=all"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", rec.Code)
	}
	if fu.gotConsumer != "" {
		t.Error("bad range should be rejected before querying")
	}
}

func TestUsageForbidsNonOwner(t *testing.T) {
	reader := fakeReader{has: true, cred: Credential{ConsumerUsername: "app_1"}}
	h := newUsageHandler(reader, &fakeUsage{})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, usageReq(1, 999, "?range=24h")) // not the owner
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want 403", rec.Code)
	}
}

func TestUsageUnavailableWhenBackendErrors(t *testing.T) {
	// Prometheus down must surface explicitly, never as silent/demo data.
	reader := fakeReader{has: true, cred: Credential{ConsumerUsername: "app_1"}}
	fu := &fakeUsage{err: errors.New("dial tcp: connection refused")}
	h := newUsageHandler(reader, fu)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, usageReq(1, 5, "?range=24h"))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d, want 503", rec.Code)
	}
}

func TestUsageUnavailableWhenNotConfigured(t *testing.T) {
	reader := fakeReader{has: true, cred: Credential{ConsumerUsername: "app_1"}}
	h := newUsageHandler(reader, nil) // metrics disabled
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, usageReq(1, 5, "?range=24h"))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d, want 503", rec.Code)
	}
}

func TestUsageEmptyWhenNoCredentialYet(t *testing.T) {
	// An app with no subscription has no gateway consumer and thus no traffic;
	// that's zeroed usage, not an error.
	reader := fakeReader{has: false}
	fu := &fakeUsage{}
	h := newUsageHandler(reader, fu)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, usageReq(1, 5, "?range=24h"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}
	if fu.gotConsumer != "" {
		t.Error("should not query metrics without a consumer")
	}
	var got metrics.Usage
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Summary.RequestsToday != 0 || got.Series == nil {
		t.Errorf("want zeroed usage with non-nil series, got %+v", got)
	}
}
