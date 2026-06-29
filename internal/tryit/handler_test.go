package tryit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"apisix-portal/internal/auth"
)

type fakeProducts struct {
	id      int64
	ctx     string
	err     error
	sandbox bool
}

func (f fakeProducts) ProductBySlug(_ context.Context, _ string) (int64, string, error) {
	return f.id, f.ctx, f.err
}
func (f fakeProducts) SandboxUpstream(_ context.Context, _ string) (bool, error) {
	return f.sandbox, nil
}

type fakeAccess struct {
	owns       bool
	status     string
	key        string
	apps       []AppRef
	sandboxKey string
}

func (f fakeAccess) OwnsApp(_ context.Context, _, _ int64) (bool, error) {
	return f.owns, nil
}
func (f fakeAccess) SubscriptionStatus(_ context.Context, _, _ int64) (string, error) {
	return f.status, nil
}
func (f fakeAccess) APIKey(_ context.Context, _ int64) (string, error) { return f.key, nil }
func (f fakeAccess) ApprovedApps(_ context.Context, _, _ int64) ([]AppRef, error) {
	return f.apps, nil
}
func (f fakeAccess) SandboxKey(_ context.Context, _ int64) (string, error) {
	return f.sandboxKey, nil
}

// withUser injects an authed user id into the request context.
func withUser(r *http.Request, id int64) *http.Request {
	return r.WithContext(auth.WithUserID(r.Context(), id))
}

func TestContextListsApprovedApps(t *testing.T) {
	h := NewHandler(fakeProducts{id: 9, ctx: "/orders"},
		fakeAccess{apps: []AppRef{{ID: 1, Name: "App A"}}}, "http://gw:9080", "")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, withUser(httptest.NewRequest(http.MethodGet, "/api/try/orders/context", nil), 7))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct{ Apps []AppRef }
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Apps) != 1 || out.Apps[0].Name != "App A" {
		t.Errorf("apps=%v", out.Apps)
	}
}

func TestProxyForwardsWithKeyInjected(t *testing.T) {
	var gotPath, gotKey, gotMethod string
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotKey, gotMethod = r.URL.Path, r.Header.Get("apikey"), r.Method
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer gw.Close()

	h := NewHandler(fakeProducts{id: 9, ctx: "/orders"},
		fakeAccess{owns: true, status: "active", key: "ax_live_k1"}, gw.URL, "")
	req := withUser(httptest.NewRequest(http.MethodPost, "/api/try/orders/3/pet/5", strings.NewReader(`{"a":1}`)), 7)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != 201 || rec.Body.String() != `{"ok":true}` {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if gotPath != "/orders/pet/5" {
		t.Errorf("gateway path=%q", gotPath)
	}
	if gotKey != "ax_live_k1" {
		t.Errorf("apikey=%q", gotKey)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method=%q", gotMethod)
	}
}

func TestProxyRejectsNotOwner(t *testing.T) {
	h := NewHandler(fakeProducts{id: 9, ctx: "/orders"},
		fakeAccess{owns: false, status: "active", key: "k"}, "http://gw:9080", "")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, withUser(httptest.NewRequest(http.MethodGet, "/api/try/orders/3/x", nil), 7))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestProxyRejectsUnapproved(t *testing.T) {
	h := NewHandler(fakeProducts{id: 9, ctx: "/orders"},
		fakeAccess{owns: true, status: "pending", key: "k"}, "http://gw:9080", "")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, withUser(httptest.NewRequest(http.MethodGet, "/api/try/orders/3/x", nil), 7))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestProxyUnknownProduct404(t *testing.T) {
	h := NewHandler(fakeProducts{err: ErrNotFound},
		fakeAccess{owns: true, status: "active", key: "k"}, "http://gw:9080", "")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, withUser(httptest.NewRequest(http.MethodGet, "/api/try/nope/3/x", nil), 7))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestSandboxProxyInjectsSandboxKeyAndTargetsSandboxGateway(t *testing.T) {
	var gotKey, gotPath string
	sandbox := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("apikey")
		gotPath = r.URL.Path
		w.WriteHeader(200)
	}))
	defer sandbox.Close()

	h := NewHandler(fakeProducts{id: 9, ctx: "/echo", sandbox: true},
		fakeAccess{owns: true, status: "active", sandboxKey: "SB"},
		"http://prod.invalid", sandbox.URL)
	req := withUser(httptest.NewRequest(http.MethodGet, "/api/try/orders/42/sandbox/ping", nil), 7)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status=%d", rec.Code)
	}
	if gotKey != "SB" {
		t.Errorf("apikey=%q want SB", gotKey)
	}
	if gotPath != "/echo/ping" {
		t.Errorf("path=%q want /echo/ping", gotPath)
	}
}
