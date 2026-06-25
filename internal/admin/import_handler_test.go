package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newImportHandler builds a Handler with a nil service: import must not touch it.
func newImportHandler(allowPrivate bool) *Handler {
	return NewHandler(nil, allowPrivate)
}

func TestImport_SpecBody_OK(t *testing.T) {
	h := newImportHandler(false)
	body := `{"spec": "{\"openapi\":\"3.0.0\",\"info\":{\"title\":\"Imported API\",\"version\":\"3.0.0\"},\"servers\":[{\"url\":\"https://api.example.com/v3\"}]}"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/products/import", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var p Product
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if p.Name != "Imported API" || p.Version != "3.0.0" {
		t.Errorf("got %q / %q", p.Name, p.Version)
	}
	if p.ContextPath != "/v3" {
		t.Errorf("ContextPath = %q", p.ContextPath)
	}
	if p.Published {
		t.Error("imported product must be unpublished")
	}
}

func TestImport_BadSpec_422(t *testing.T) {
	h := newImportHandler(false)
	body := `{"spec": "this is not a spec"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/products/import", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestImport_NeitherField_400(t *testing.T) {
	h := newImportHandler(false)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/products/import", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestImport_BothFields_400(t *testing.T) {
	h := newImportHandler(false)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/products/import",
		strings.NewReader(`{"spec":"x","url":"http://example.com"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestImport_UnsafeURL_422(t *testing.T) {
	h := newImportHandler(false)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/products/import",
		strings.NewReader(`{"url":"http://127.0.0.1/spec.json"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
