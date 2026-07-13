// Package settings_test holds the HTTP handler tests as an EXTERNAL test
// package (settings_test, not settings) — deliberately, not by convention.
//
// internal/auth already imports internal/notify (to send verification
// emails), and internal/notify imports internal/settings (its dynamic SMTP
// sender reads *settings.Effective). If this file were "package settings"
// (an internal test file, compiling as part of the settings package under
// test) and imported auth to call auth.WithUserID, that would close the
// cycle settings -> auth -> notify -> settings — which Go rejects even
// though production code never has settings import auth (see handler.go's
// NewHandler doc comment: adminID is injected instead, precisely to avoid
// that edge in the real build graph). Building this file as the external
// settings_test package sidesteps the problem: it is never itself imported by
// anything, so it can safely import both settings and auth.
//
// The price is that the DB-backed test fixtures (NewTestService, StubProber) —
// otherwise unexported, package-private helpers in service_test.go — had to
// be exported so this external package can reach them.
package settings_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"apisix-portal/internal/auth"
	"apisix-portal/internal/settings"
)

func doReq(h *settings.Handler, method, target string, body any) *httptest.ResponseRecorder {
	var rdr *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, target, rdr)
	req = req.WithContext(auth.WithUserID(req.Context(), 7))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestGetShapeAndSecretMasking(t *testing.T) {
	svc := settings.NewTestService(t, &settings.StubProber{})
	if err := svc.Set(context.Background(), map[string]string{"SMTP_PASSWORD": "s3cret", "SMTP_HOST": "h1"}, 7, false); err != nil {
		t.Fatalf("seed: %v", err)
	}
	h := settings.NewHandler(svc, auth.UserID)
	rec := doReq(h, http.MethodGet, "/api/admin/settings", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "s3cret") {
		t.Fatal("secret value leaked in GET")
	}
	var groups []settings.GroupView
	if err := json.Unmarshal(rec.Body.Bytes(), &groups); err != nil {
		t.Fatalf("decode: %v", err)
	}
	byKey := map[string]settings.ItemView{}
	for _, g := range groups {
		for _, it := range g.Items {
			byKey[it.Key] = it
		}
	}
	if len(byKey) != 24 {
		t.Fatalf("items = %d, want 24", len(byKey))
	}
	pw := byKey["SMTP_PASSWORD"]
	if pw.Value != nil || !pw.Set || pw.Source != "db" || pw.EnvDefault != nil {
		t.Fatalf("secret view wrong: %+v", pw)
	}
	host := byKey["SMTP_HOST"]
	if host.Value == nil || *host.Value != "h1" || host.Source != "db" || host.EnvDefault == nil || *host.EnvDefault != "envhost" {
		t.Fatalf("host view wrong: %+v", host)
	}
	jwt := byKey["JWT_SECRET"]
	if jwt.Editable || jwt.Value != nil {
		t.Fatalf("boot-critical secret must be read-only + valueless: %+v", jwt)
	}
}

func TestPutMatrix(t *testing.T) {
	svc := settings.NewTestService(t, &settings.StubProber{})
	h := settings.NewHandler(svc, auth.UserID)
	if rec := doReq(h, http.MethodPut, "/api/admin/settings", map[string]any{"values": map[string]string{"SMTP_HOST": "new"}}); rec.Code != http.StatusNoContent {
		t.Fatalf("valid PUT = %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := doReq(h, http.MethodPut, "/api/admin/settings", map[string]any{"values": map[string]string{"NOPE": "x"}}); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown = %d", rec.Code)
	}
	if rec := doReq(h, http.MethodPut, "/api/admin/settings", map[string]any{"values": map[string]string{"JWT_SECRET": "x"}}); rec.Code != http.StatusBadRequest {
		t.Fatalf("read-only = %d", rec.Code)
	}
	rec := doReq(h, http.MethodPut, "/api/admin/settings", map[string]any{"values": map[string]string{"SMTP_PORT": "nope"}})
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), `"fields"`) {
		t.Fatalf("invalid field = %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestPutProbeFailureAndForce(t *testing.T) {
	p := &settings.StubProber{Results: []settings.ProbeResult{{Name: "smtp", OK: false, Detail: "refused"}}}
	svc := settings.NewTestService(t, p)
	h := settings.NewHandler(svc, auth.UserID)
	rec := doReq(h, http.MethodPut, "/api/admin/settings", map[string]any{"values": map[string]string{"SMTP_HOST": "bogus"}})
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), `"probe"`) {
		t.Fatalf("probe fail = %d (%s)", rec.Code, rec.Body.String())
	}
	rec = doReq(h, http.MethodPut, "/api/admin/settings", map[string]any{"values": map[string]string{"SMTP_HOST": "bogus"}, "force": true})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("forced = %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestDeleteReset(t *testing.T) {
	svc := settings.NewTestService(t, &settings.StubProber{})
	h := settings.NewHandler(svc, auth.UserID)
	_ = svc.Set(context.Background(), map[string]string{"SMTP_HOST": "x"}, 7, false)
	if rec := doReq(h, http.MethodDelete, "/api/admin/settings/SMTP_HOST", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("reset = %d", rec.Code)
	}
	if svc.Snapshot().Source["SMTP_HOST"] != "env" {
		t.Fatal("reset must fall back to env")
	}
	if rec := doReq(h, http.MethodDelete, "/api/admin/settings/NOPE", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown reset = %d", rec.Code)
	}
	if rec := doReq(h, http.MethodDelete, "/api/admin/settings/JWT_SECRET", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("read-only reset = %d", rec.Code)
	}
}

func TestTestEndpoint(t *testing.T) {
	p := &settings.StubProber{Results: []settings.ProbeResult{{Name: "smtp", OK: true, Detail: "ok"}}}
	svc := settings.NewTestService(t, p)
	h := settings.NewHandler(svc, auth.UserID)
	rec := doReq(h, http.MethodPost, "/api/admin/settings/test", map[string]any{"values": map[string]string{"SMTP_HOST": "candidate"}})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"smtp"`) {
		t.Fatalf("test = %d (%s)", rec.Code, rec.Body.String())
	}
	if svc.Snapshot().Get("SMTP_HOST") == "candidate" {
		t.Fatal("test must not persist")
	}
}
