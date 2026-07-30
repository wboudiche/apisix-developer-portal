package catalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"apisix-portal/internal/paging"
)

type fakeIconLister struct {
	data      []byte
	updatedAt time.Time
	err       error
}

func (f fakeIconLister) List(context.Context, Query, paging.Params) ([]Product, int, error) {
	return nil, 0, nil
}
func (f fakeIconLister) GetBySlug(context.Context, string) (Product, error)    { return Product{}, nil }
func (f fakeIconLister) GetSpecBySlug(context.Context, string) (string, error) { return "", nil }
func (f fakeIconLister) ListChangelogBySlug(context.Context, string) ([]ChangelogEntry, error) {
	return nil, nil
}
func (f fakeIconLister) GetIconBySlug(context.Context, string) ([]byte, time.Time, error) {
	return f.data, f.updatedAt, f.err
}

func TestGetIconServesPNG(t *testing.T) {
	h := NewHandler(fakeIconLister{data: []byte("PNGBYTES"), updatedAt: time.Unix(1700000000, 0)})
	req := httptest.NewRequest(http.MethodGet, "/api/products/foo/icon", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("content-type %q want image/png", ct)
	}
	if rec.Body.String() != "PNGBYTES" {
		t.Fatalf("body=%q", rec.Body.String())
	}
	if rec.Header().Get("ETag") == "" {
		t.Fatal("missing ETag")
	}
}

func TestGetIconNotModified(t *testing.T) {
	h := NewHandler(fakeIconLister{data: []byte("PNGBYTES"), updatedAt: time.Unix(1700000000, 0)})
	req := httptest.NewRequest(http.MethodGet, "/api/products/foo/icon", nil)
	req.Header.Set("If-None-Match", `"1700000000"`)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotModified {
		t.Fatalf("got %d want 304", rec.Code)
	}
}

func TestGetIconMissing(t *testing.T) {
	h := NewHandler(fakeIconLister{err: ErrNotFound})
	req := httptest.NewRequest(http.MethodGet, "/api/products/foo/icon", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d want 404", rec.Code)
	}
}
