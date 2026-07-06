package admin

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeIconService implements ProductService just for the icon upload path.
// It embeds the ProductService interface (nil) so it satisfies the type; only
// SetUploadedIcon is exercised by these tests.
type fakeIconService struct {
	ProductService
	got    []byte
	setErr error
}

func (f *fakeIconService) SetUploadedIcon(_ context.Context, _ int64, png []byte) (time.Time, error) {
	f.got = png
	return time.Unix(1700000000, 0), f.setErr
}

func multipartIcon(t *testing.T, field string, data []byte) (*bytes.Buffer, string) {
	t.Helper()
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	fw, _ := w.CreateFormFile(field, "icon.png")
	fw.Write(data)
	w.Close()
	return &b, w.FormDataContentType()
}

func smallPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 48, 48))
	img.Set(1, 1, color.RGBA{10, 20, 30, 255})
	var b bytes.Buffer
	png.Encode(&b, img)
	return b.Bytes()
}

func TestUploadIconAcceptsPNG(t *testing.T) {
	svc := &fakeIconService{}
	h := NewHandler(svc, true, false)
	body, ct := multipartIcon(t, "file", smallPNG(t))
	req := httptest.NewRequest(http.MethodPost, "/api/admin/products/7/icon", body)
	req.Header.Set("Content-Type", ct)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("got %d want 204 (body=%s)", rec.Code, rec.Body)
	}
	if len(svc.got) == 0 {
		t.Fatal("service received no bytes")
	}
	if _, format, err := image.DecodeConfig(bytes.NewReader(svc.got)); err != nil || format != "png" {
		t.Fatalf("stored bytes not PNG (format=%q err=%v)", format, err)
	}
}

func TestUploadIconRejectsNonImage(t *testing.T) {
	h := NewHandler(&fakeIconService{}, true, false)
	body, ct := multipartIcon(t, "file", []byte(strings.Repeat("not-an-image ", 60)))
	req := httptest.NewRequest(http.MethodPost, "/api/admin/products/7/icon", body)
	req.Header.Set("Content-Type", ct)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("got %d want 415", rec.Code)
	}
}

func TestUploadIconProductNotFound(t *testing.T) {
	h := NewHandler(&fakeIconService{setErr: ErrNotFound}, true, false)
	body, ct := multipartIcon(t, "file", smallPNG(t))
	req := httptest.NewRequest(http.MethodPost, "/api/admin/products/999/icon", body)
	req.Header.Set("Content-Type", ct)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d want 404", rec.Code)
	}
}
