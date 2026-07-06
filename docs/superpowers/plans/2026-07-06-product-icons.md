# Product Icons Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let admins pick a built-in glyph or upload a custom (securely validated) raster icon when creating/editing an API product.

**Architecture:** The `api_products.icon` column holds a built-in glyph key, `''` (default), or the sentinel `"upload"`. A custom icon is stored as a portal-re-encoded PNG in a new `product_icons` blob table, written by an admin upload endpoint and streamed by a public slug-based serve endpoint. Uploads are decoded and re-encoded server-side (raster only) to neutralize SVG-XSS and polyglots.

**Tech Stack:** Go 1.25 (chi, pgx/pgxpool, stdlib `image`/`image/png`/`image/jpeg`, new dep `golang.org/x/image/webp` decode-only), React 19 + TS + Vite + vitest.

## Global Constraints

- Raster only: accept PNG/JPEG/WebP; **reject SVG**. Always re-encode to PNG before storing.
- Upload size cap **256 KiB** (`256<<10`); dimensions must be within **16–512 px** on both axes.
- Upload endpoint is **admin-only** (`POST /api/admin/products/{id}/icon`, id-based); serve endpoint is **public** (`GET /api/products/{slug}/icon`, slug-based — chi requires the same wildcard name as the sibling `/spec` and `/changelog` routes).
- Stored blob is always `image/png`; the serve endpoint always sets `Content-Type: image/png`.
- Custom upload is **edit-mode only** (a blob needs an existing product id); the built-in picker works at create.
- All user-facing strings/errors go through the fr/en i18n catalogs (backend `internal/i18n/catalog_{fr,en}.go`, frontend `web/src/i18n/{fr,en}.ts`), with byte-identical key sets.
- `icon` sentinel/keys: built-in keys are `seo reviews stock test keyword people currency phone pizza`; `''` = default; `"upload"` = custom PNG present.
- The real typecheck for `web/` is `pnpm build` (NOT `tsc --noEmit`).

---

### Task 1: Image validation + re-encode pipeline (pure)

**Files:**
- Create: `internal/admin/icon.go`
- Test: `internal/admin/icon_test.go`
- Modify: `go.mod` / `go.sum` (add `golang.org/x/image/webp`)

**Interfaces:**
- Produces: `admin.DecodeAndReencode(raw []byte) ([]byte, error)`; sentinels `admin.ErrIconType`, `admin.ErrIconUndecodable`.

- [ ] **Step 1: Add the WebP decoder dependency**

Run: `go get golang.org/x/image/webp@latest`
Expected: `go.mod` gains a `golang.org/x/image vX.Y.Z` require line.

- [ ] **Step 2: Write the failing test**

Create `internal/admin/icon_test.go`:

```go
package admin

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

// a 32x32 red WebP (Go's x/image can decode but not encode WebP).
const redWebP32 = "UklGRkoAAABXRUJQVlA4ID4AAAAwAwCdASogACAAPm00lkekIyIhKAgAgA2JZQDMSoAAQFBQAP7vKUf43m81s4//7B3/6Dv/0Hf7Jtvb2AAAAA=="

func rasterPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{200, 40, 40, 255})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func rasterJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestDecodeAndReencodeAcceptsRaster(t *testing.T) {
	webp, _ := base64.StdEncoding.DecodeString(redWebP32)
	for name, raw := range map[string][]byte{
		"png":  rasterPNG(t, 64, 64),
		"jpeg": rasterJPEG(t, 64, 64),
		"webp": webp,
	} {
		out, err := DecodeAndReencode(raw)
		if err != nil {
			t.Fatalf("%s: unexpected error %v", name, err)
		}
		if _, format, err := image.DecodeConfig(bytes.NewReader(out)); err != nil || format != "png" {
			t.Fatalf("%s: output not PNG (format=%q err=%v)", name, format, err)
		}
	}
}

func TestDecodeAndReencodeRejectsNonRaster(t *testing.T) {
	cases := map[string][]byte{
		"svg":        []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
		"text":       []byte("this is definitely not an image, padded out to beyond 512 bytes " + strings.Repeat("x", 600)),
		"fake-magic": append([]byte("\x89PNG\r\n\x1a\n"), []byte(strings.Repeat("x", 600))...),
	}
	for name, raw := range cases {
		if _, err := DecodeAndReencode(raw); err == nil {
			t.Fatalf("%s: expected rejection, got nil", name)
		}
	}
}

func TestDecodeAndReencodeRejectsOversizeDimensions(t *testing.T) {
	if _, err := DecodeAndReencode(rasterPNG(t, 600, 600)); err == nil {
		t.Fatal("600x600: expected ErrIconUndecodable, got nil")
	}
	if _, err := DecodeAndReencode(rasterPNG(t, 8, 8)); err == nil {
		t.Fatal("8x8: expected rejection (below min), got nil")
	}
}

func TestDecodeAndReencodeStripsTrailingBytes(t *testing.T) {
	polyglot := append(rasterPNG(t, 64, 64), []byte("<?php evil(); ?>")...)
	out, err := DecodeAndReencode(polyglot)
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}
	if bytes.Contains(out, []byte("evil")) {
		t.Fatal("re-encoded output still contains trailing payload")
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/admin/ -run TestDecodeAndReencode -v`
Expected: FAIL — `undefined: DecodeAndReencode`.

- [ ] **Step 4: Implement the pipeline**

Create `internal/admin/icon.go`:

```go
package admin

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"net/http"

	_ "image/jpeg" // register JPEG decoder for image.Decode/DecodeConfig
	_ "image/png"  // register PNG decoder

	_ "golang.org/x/image/webp" // register WebP decoder (decode-only)
)

const (
	iconMaxDim = 512
	iconMinDim = 16
)

// ErrIconType is returned when the bytes are not a supported raster image.
var ErrIconType = errors.New("admin: unsupported icon type")

// ErrIconUndecodable is returned when dimensions are out of range or the image
// cannot be decoded.
var ErrIconUndecodable = errors.New("admin: undecodable icon")

// DecodeAndReencode validates raw as a PNG/JPEG/WebP raster and returns a fresh
// re-encoded PNG. It sniffs the content type (ignoring any caller-supplied
// filename/header), guards dimensions before full decompression, fully decodes
// to prove a real raster, then re-encodes — discarding EXIF, trailing bytes,
// and polyglot tails. SVG and other non-raster inputs are rejected.
func DecodeAndReencode(raw []byte) ([]byte, error) {
	switch http.DetectContentType(raw) {
	case "image/png", "image/jpeg", "image/webp":
	default:
		return nil, ErrIconType
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, ErrIconUndecodable
	}
	if cfg.Width > iconMaxDim || cfg.Height > iconMaxDim || cfg.Width < iconMinDim || cfg.Height < iconMinDim {
		return nil, fmt.Errorf("%w: %dx%d out of [%d,%d]", ErrIconUndecodable, cfg.Width, cfg.Height, iconMinDim, iconMaxDim)
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, ErrIconUndecodable
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/admin/ -run TestDecodeAndReencode -v`
Expected: PASS (4 tests).

- [ ] **Step 6: Commit**

```bash
git add internal/admin/icon.go internal/admin/icon_test.go go.mod go.sum
git commit -m "feat(icons): raster decode+re-encode pipeline (PNG/JPEG/WebP -> clean PNG)"
```

---

### Task 2: Migration + icon storage (admin repo/service)

**Files:**
- Create: `internal/db/migrations/0018_product_icons.sql`
- Modify: `internal/admin/repo.go` (add `SetUploadedIcon`, `DeleteIcon`)
- Modify: `internal/admin/service.go` (Store interface + `SetUploadedIcon` + delete-blob-on-update)
- Test: `internal/admin/icon_repo_test.go`

**Interfaces:**
- Consumes: `admin.Repo` (has `pool *pgxpool.Pool`), `admin.Store`, `admin.Service`.
- Produces:
  - `(*Repo).SetUploadedIcon(ctx, productID int64, png []byte) (time.Time, error)`
  - `(*Repo).DeleteIcon(ctx, productID int64) error`
  - `(*Service).SetUploadedIcon(ctx, productID int64, png []byte) (time.Time, error)`
  - Store interface gains `SetUploadedIcon` and `DeleteIcon` with the same signatures.

- [ ] **Step 1: Write the migration**

Create `internal/db/migrations/0018_product_icons.sql`:

```sql
CREATE TABLE product_icons (
    product_id BIGINT PRIMARY KEY REFERENCES api_products(id) ON DELETE CASCADE,
    data       BYTEA NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

- [ ] **Step 2: Write the failing test**

Create `internal/admin/icon_repo_test.go`:

```go
package admin

import (
	"context"
	"os"
	"testing"

	"apisix-portal/internal/db"

	"github.com/jackc/pgx/v5/pgxpool"
)

func iconTestRepo(t *testing.T) (context.Context, *Repo, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://portal:portal@localhost:5432/portal?sslmode=disable"
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, url)
	if err != nil {
		t.Skipf("no database: %v", err)
	}
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(pool.Close)
	return ctx, NewRepo(pool), pool
}

func seedIconProduct(t *testing.T, ctx context.Context, pool *pgxpool.Pool) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(ctx,
		`INSERT INTO api_products (name, slug, category, version, context_path, description, tags, icon)
		 VALUES ('IconProd', 'iconprod-'||floor(random()*1e9)::text, 'Engineering', '1.0.0',
		         '/iconprod'||floor(random()*1e9)::text, '', '{}', '') RETURNING id`).Scan(&id)
	if err != nil {
		t.Fatalf("seed product: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM api_products WHERE id=$1`, id) })
	return id
}

func TestSetUploadedIconUpsertsAndFlagsProduct(t *testing.T) {
	ctx, repo, pool := iconTestRepo(t)
	id := seedIconProduct(t, ctx, pool)

	if _, err := repo.SetUploadedIcon(ctx, id, []byte("PNGBYTES-1")); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	var icon string
	var data []byte
	if err := pool.QueryRow(ctx, `SELECT p.icon, i.data FROM api_products p JOIN product_icons i ON i.product_id=p.id WHERE p.id=$1`, id).
		Scan(&icon, &data); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if icon != "upload" || string(data) != "PNGBYTES-1" {
		t.Fatalf("got icon=%q data=%q", icon, data)
	}

	// upsert replaces on conflict
	if _, err := repo.SetUploadedIcon(ctx, id, []byte("PNGBYTES-2")); err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	_ = pool.QueryRow(ctx, `SELECT data FROM product_icons WHERE product_id=$1`, id).Scan(&data)
	if string(data) != "PNGBYTES-2" {
		t.Fatalf("upsert did not replace: %q", data)
	}
}

func TestDeleteIconRemovesRow(t *testing.T) {
	ctx, repo, pool := iconTestRepo(t)
	id := seedIconProduct(t, ctx, pool)
	if _, err := repo.SetUploadedIcon(ctx, id, []byte("X")); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteIcon(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	var n int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM product_icons WHERE product_id=$1`, id).Scan(&n)
	if n != 0 {
		t.Fatalf("row not deleted, count=%d", n)
	}
}

func TestProductDeleteCascadesIcon(t *testing.T) {
	ctx, repo, pool := iconTestRepo(t)
	id := seedIconProduct(t, ctx, pool)
	if _, err := repo.SetUploadedIcon(ctx, id, []byte("X")); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM api_products WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM product_icons WHERE product_id=$1`, id).Scan(&n)
	if n != 0 {
		t.Fatalf("cascade failed, count=%d", n)
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run (stack up): `go test ./internal/admin/ -run 'Icon' -v`
Expected: FAIL — `repo.SetUploadedIcon undefined`.

- [ ] **Step 4: Implement the repo methods**

Add to `internal/admin/repo.go` (imports: add `"time"`):

```go
// SetUploadedIcon stores the re-encoded PNG for a product and flags the product
// as using an uploaded icon, in one transaction. Returns the icon's updated_at.
func (r *Repo) SetUploadedIcon(ctx context.Context, productID int64, png []byte) (time.Time, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return time.Time{}, err
	}
	defer tx.Rollback(ctx)

	var updatedAt time.Time
	err = tx.QueryRow(ctx,
		`INSERT INTO product_icons (product_id, data, updated_at)
		 VALUES ($1, $2, now())
		 ON CONFLICT (product_id) DO UPDATE SET data = EXCLUDED.data, updated_at = now()
		 RETURNING updated_at`, productID, png).Scan(&updatedAt)
	if err != nil {
		return time.Time{}, err
	}
	tag, err := tx.Exec(ctx, `UPDATE api_products SET icon='upload' WHERE id=$1`, productID)
	if err != nil {
		return time.Time{}, err
	}
	if tag.RowsAffected() == 0 {
		return time.Time{}, ErrNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return time.Time{}, err
	}
	return updatedAt, nil
}

// DeleteIcon removes any stored custom icon for a product (idempotent).
func (r *Repo) DeleteIcon(ctx context.Context, productID int64) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM product_icons WHERE product_id=$1`, productID)
	return err
}
```

- [ ] **Step 5: Extend the Store interface + Service**

In `internal/admin/service.go`, add to the `Store` interface (with `"time"` imported):

```go
	SetUploadedIcon(ctx context.Context, productID int64, png []byte) (time.Time, error)
	DeleteIcon(ctx context.Context, productID int64) error
```

Add a Service method:

```go
// SetUploadedIcon stores a re-encoded PNG icon for a product.
func (s *Service) SetUploadedIcon(ctx context.Context, productID int64, png []byte) (time.Time, error) {
	return s.store.SetUploadedIcon(ctx, productID, png)
}
```

In `(*Service).Update`, after the successful `s.store.Update(ctx, p)` returns `updated`, drop any stale blob when the product no longer uses an uploaded icon. Insert immediately after `updated, err := s.store.Update(ctx, p)` error check:

```go
	if updated.Icon != "upload" {
		if err := s.store.DeleteIcon(ctx, p.ID); err != nil {
			return Product{}, err
		}
	}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run (stack up): `go test ./internal/admin/ -run 'Icon' -count=1 -v`
Expected: PASS (3 DB tests; they Skip if no DATABASE_URL).
Also run `go build ./...` to confirm the Store interface still compiles against `*Repo`.

- [ ] **Step 7: Commit**

```bash
git add internal/db/migrations/0018_product_icons.sql internal/admin/repo.go internal/admin/service.go internal/admin/icon_repo_test.go
git commit -m "feat(icons): product_icons table + upsert/delete + drop-blob-on-icon-switch"
```

---

### Task 3: Admin upload endpoint

**Files:**
- Modify: `internal/admin/handler.go` (route + `uploadIcon` method + `ProductService` interface)
- Test: `internal/admin/handler_icon_test.go`

**Interfaces:**
- Consumes: `admin.DecodeAndReencode`, `(*Service).SetUploadedIcon`, `admin.ErrIconType`, `admin.ErrIconUndecodable`, `admin.ErrNotFound`.
- Produces: route `POST /api/admin/products/{id}/icon`; `ProductService` interface gains `SetUploadedIcon(ctx, int64, []byte) (time.Time, error)`.

- [ ] **Step 1: Write the failing test**

Create `internal/admin/handler_icon_test.go`:

```go
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/admin/ -run TestUploadIcon -v`
Expected: FAIL — `SetUploadedIcon` not in `ProductService`, route 404.

- [ ] **Step 3: Add the interface method + route**

In `internal/admin/handler.go`, add to the `ProductService` interface (import `"time"`):

```go
	SetUploadedIcon(ctx context.Context, productID int64, png []byte) (time.Time, error)
```

Register the route in `NewHandler` (after the changelog routes):

```go
	h.router.Post("/api/admin/products/{id}/icon", h.uploadIcon)
```

- [ ] **Step 4: Implement the handler**

Add to `internal/admin/handler.go` (imports: add `"errors"` if not present — it is; add `"io"`):

```go
const iconMaxUpload = 256 << 10 // 256 KiB

func (h *Handler) uploadIcon(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, iconMaxUpload)
	file, _, err := r.FormFile("file")
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			httpx.ErrorT(w, r, http.StatusRequestEntityTooLarge, "admin.icon.tooLarge")
			return
		}
		httpx.ErrorT(w, r, http.StatusBadRequest, "admin.icon.badBody")
		return
	}
	defer file.Close()
	raw, err := io.ReadAll(file)
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			httpx.ErrorT(w, r, http.StatusRequestEntityTooLarge, "admin.icon.tooLarge")
			return
		}
		httpx.ErrorT(w, r, http.StatusBadRequest, "admin.icon.badBody")
		return
	}
	png, err := DecodeAndReencode(raw)
	if errors.Is(err, ErrIconType) {
		httpx.ErrorT(w, r, http.StatusUnsupportedMediaType, "admin.icon.badType")
		return
	} else if err != nil {
		httpx.ErrorT(w, r, http.StatusUnprocessableEntity, "admin.icon.undecodable")
		return
	}
	if _, err := h.svc.SetUploadedIcon(r.Context(), id, png); errors.Is(err, ErrNotFound) {
		httpx.ErrorT(w, r, http.StatusNotFound, "catalog.productNotFound")
		return
	} else if err != nil {
		log.Printf("upload icon (product=%d): %v", id, err)
		httpx.ErrorT(w, r, http.StatusInternalServerError, "catalog.list.failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/admin/ -run TestUploadIcon -v`
Expected: PASS (3 tests).

- [ ] **Step 6: Commit**

```bash
git add internal/admin/handler.go internal/admin/handler_icon_test.go
git commit -m "feat(icons): admin POST /api/admin/products/{id}/icon upload endpoint"
```

---

### Task 4: Public serve endpoint

**Files:**
- Modify: `internal/catalog/repo.go` (add `GetIconBySlug`)
- Modify: `internal/catalog/handler.go` (Lister interface + route + `getIcon`)
- Test: `internal/catalog/handler_icon_test.go`

**Interfaces:**
- Consumes: `catalog.Repo` (`pool *pgxpool.Pool`), `catalog.ErrNotFound`, `catalog.Lister`.
- Produces: `(*Repo).GetIconBySlug(ctx, slug string) (data []byte, updatedAt time.Time, err error)`; route `GET /api/products/{slug}/icon`; Lister gains `GetIconBySlug`.

- [ ] **Step 1: Write the failing test**

Create `internal/catalog/handler_icon_test.go`:

```go
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
func (f fakeIconLister) GetBySlug(context.Context, string) (Product, error) { return Product{}, nil }
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/catalog/ -run TestGetIcon -v`
Expected: FAIL — `GetIconBySlug` not in Lister, route 404.

- [ ] **Step 3: Implement the repo method**

Add to `internal/catalog/repo.go` (import `"time"`):

```go
// GetIconBySlug returns a product's stored custom-icon PNG and its updated_at.
// Returns ErrNotFound when the product has no uploaded icon.
func (r *Repo) GetIconBySlug(ctx context.Context, slug string) ([]byte, time.Time, error) {
	var data []byte
	var updatedAt time.Time
	err := r.pool.QueryRow(ctx,
		`SELECT i.data, i.updated_at FROM product_icons i
		 JOIN api_products p ON p.id = i.product_id WHERE p.slug = $1`, slug).
		Scan(&data, &updatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, time.Time{}, ErrNotFound
		}
		return nil, time.Time{}, err
	}
	return data, updatedAt, nil
}
```

(If `pgx` is not yet imported in repo.go, import `"github.com/jackc/pgx/v5"`.)

- [ ] **Step 4: Add the Lister method + route + handler**

In `internal/catalog/handler.go`, add to the `Lister` interface (import `"time"`, `"strconv"`, `"fmt"`):

```go
	GetIconBySlug(ctx context.Context, slug string) ([]byte, time.Time, error)
```

Register the route in `NewHandler`:

```go
	h.router.Get("/api/products/{slug}/icon", h.getIcon)
```

Add the handler method:

```go
func (h *Handler) getIcon(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	data, updatedAt, err := h.repo.GetIconBySlug(r.Context(), slug)
	if err == ErrNotFound {
		httpx.ErrorT(w, r, http.StatusNotFound, "catalog.productNotFound")
		return
	}
	if err != nil {
		httpx.ErrorT(w, r, http.StatusInternalServerError, "catalog.list.failed")
		return
	}
	etag := fmt.Sprintf("%q", strconv.FormatInt(updatedAt.Unix(), 10))
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=60")
	w.Header().Set("ETag", etag)
	w.Write(data)
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/catalog/ -run TestGetIcon -v`
Expected: PASS (3 tests).

- [ ] **Step 6: Commit**

```bash
git add internal/catalog/repo.go internal/catalog/handler.go internal/catalog/handler_icon_test.go
git commit -m "feat(icons): public GET /api/products/{slug}/icon serve (PNG + ETag/304)"
```

---

### Task 5: Frontend — i18n keys, client fn, display branch

**Files:**
- Modify: `internal/i18n/catalog_en.go`, `internal/i18n/catalog_fr.go` (add `admin.icon.*` keys)
- Modify: `web/src/i18n/en.ts`, `web/src/i18n/fr.ts` (mirror keys)
- Modify: `web/src/api/client.ts` (add `adminUploadProductIcon`)
- Modify: `web/src/components/apiIcons.tsx` (export `BUILTIN_ICON_KEYS`)
- Modify: `web/src/components/ApiCard.tsx`, `web/src/pages/ProductDetailPage.tsx` (render branch)
- Test: `web/src/components/ApiCard.test.tsx` (extend), `web/src/api/client.icon.test.ts` (new)

**Interfaces:**
- Consumes: serve route `GET /api/products/{slug}/icon`; the `icon==='upload'` sentinel.
- Produces: `adminUploadProductIcon(token: string, id: number, file: File): Promise<{ updatedAt: string }>`; `BUILTIN_ICON_KEYS: string[]`; `iconSrc(slug: string, v?: string|number): string` helper in `apiIcons.tsx`.

- [ ] **Step 1: Add backend i18n keys**

In `internal/i18n/catalog_en.go` add:

```go
	"admin.icon.tooLarge":    "image is too large (max 256 KB)",
	"admin.icon.badType":     "unsupported image type (use PNG, JPEG, or WebP)",
	"admin.icon.undecodable": "could not read the image (must be 16–512 px square-ish)",
	"admin.icon.badBody":     "invalid upload",
```

In `internal/i18n/catalog_fr.go` add (same keys):

```go
	"admin.icon.tooLarge":    "image trop volumineuse (max 256 Ko)",
	"admin.icon.badType":     "type d'image non pris en charge (PNG, JPEG ou WebP)",
	"admin.icon.undecodable": "image illisible (doit faire 16 à 512 px)",
	"admin.icon.badBody":     "envoi invalide",
```

- [ ] **Step 2: Verify backend catalog parity**

Run: `go test ./internal/i18n/ -run TestParity -v`
Expected: PASS (fr/en key sets identical).

- [ ] **Step 3: Add the client fn + apiIcons exports (frontend failing test first)**

Create `web/src/api/client.icon.test.ts`:

```ts
import { describe, it, expect, vi, afterEach } from 'vitest'
import { adminUploadProductIcon } from './client'

afterEach(() => vi.restoreAllMocks())

describe('adminUploadProductIcon', () => {
  it('POSTs multipart form-data with the file and bearer token', async () => {
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({ updatedAt: '2026-07-06T00:00:00Z' }), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    const file = new File([new Uint8Array([1, 2, 3])], 'icon.png', { type: 'image/png' })
    const res = await adminUploadProductIcon('tok', 7, file)
    expect(res.updatedAt).toBe('2026-07-06T00:00:00Z')
    const [url, opts] = fetchMock.mock.calls[0]
    expect(url).toBe('/api/admin/products/7/icon')
    expect(opts.method).toBe('POST')
    expect((opts.headers as Record<string, string>).Authorization).toBe('Bearer tok')
    expect(opts.body).toBeInstanceOf(FormData)
  })
})
```

- [ ] **Step 4: Run the client test to verify it fails**

Run: `cd web && npx vitest run src/api/client.icon.test.ts`
Expected: FAIL — `adminUploadProductIcon` is not exported.

- [ ] **Step 5: Implement client fn + apiIcons exports**

In `web/src/api/client.ts` add (do NOT set `Content-Type` — the browser sets the multipart boundary; only add Authorization + Accept-Language):

```ts
export async function adminUploadProductIcon(token: string, id: number, file: File): Promise<{ updatedAt: string }> {
  const form = new FormData()
  form.append('file', file)
  const url = `/api/admin/products/${id}/icon`
  const headers: Record<string, string> = { Authorization: `Bearer ${token}` }
  const lang = localStorage.getItem('lang')
  if (lang) headers['Accept-Language'] = lang
  const res = await fetch(url, { method: 'POST', headers, body: form })
  if (!res.ok) {
    let msg = `HTTP ${res.status}`
    try { msg = (await res.json()).error ?? msg } catch { /* keep */ }
    throw new ApiError(res.status, msg)
  }
  return res.json()
}
```

(Reuse the existing `ApiError` class already exported from `client.ts`.)

In `web/src/components/apiIcons.tsx`, export the built-in key list and a src helper (place after the `ICONS` map):

```tsx
export const BUILTIN_ICON_KEYS = Object.keys(ICONS)

// iconSrc builds the public custom-icon URL for a product slug, with an optional
// cache-busting version token used after an admin replaces the icon.
export function iconSrc(slug: string, v?: string | number): string {
  const base = `/api/products/${encodeURIComponent(slug)}/icon`
  return v ? `${base}?v=${encodeURIComponent(String(v))}` : base
}
```

- [ ] **Step 6: Add the display branch (extend ApiCard test)**

In `web/src/components/ApiCard.test.tsx`, add a test asserting that when `icon: 'upload'`, an `<img>` with the slug URL renders instead of the SVG. Use the existing test's render helper; add:

```tsx
it('renders an <img> for an uploaded icon', () => {
  renderCard({ ...baseProduct, slug: 'pizza-api', icon: 'upload' })
  const img = document.querySelector('img.ico-img') as HTMLImageElement
  expect(img).toBeTruthy()
  expect(img.getAttribute('src')).toBe('/api/products/pizza-api/icon')
})
```

(Match `baseProduct`/`renderCard` to whatever the existing test file defines; if it builds the product inline, mirror that shape.)

- [ ] **Step 7: Implement the display branch**

In `web/src/components/ApiCard.tsx`, replace the icon span:

```tsx
        <span className="ico">
          {p.icon === 'upload'
            ? <img className="ico-img" src={iconSrc(p.slug)} alt="" width={24} height={24} />
            : <ApiIcon name={p.icon} />}
        </span>
```

Add `iconSrc` to the existing `apiIcons` import. Apply the same branch in `web/src/pages/ProductDetailPage.tsx` where `<ApiIcon name={product.icon} />` is rendered (it is inside a `.glyph` span; use `iconSrc(product.slug)`).

- [ ] **Step 8: Run the frontend tests + build**

Run: `cd web && npx vitest run src/api/client.icon.test.ts src/components/ApiCard.test.tsx && pnpm build`
Expected: PASS; `pnpm build` (tsc -b && vite build) succeeds.

- [ ] **Step 9: Commit**

```bash
git add internal/i18n/catalog_en.go internal/i18n/catalog_fr.go web/src/api/client.ts web/src/api/client.icon.test.ts web/src/components/apiIcons.tsx web/src/components/ApiCard.tsx web/src/components/ApiCard.test.tsx web/src/pages/ProductDetailPage.tsx web/src/i18n/en.ts web/src/i18n/fr.ts
git commit -m "feat(icons): frontend display of uploaded icons + upload client fn + i18n keys"
```

---

### Task 6: Composer icon picker + upload UI

**Files:**
- Modify: `web/src/pages/admin/ProductsPage.tsx` (Composer icon field: glyph grid + Default + upload)
- Modify: `web/src/i18n/en.ts`, `web/src/i18n/fr.ts` (picker/upload labels)
- Test: `web/src/pages/admin/ProductsPage.test.tsx` (extend)

**Interfaces:**
- Consumes: `BUILTIN_ICON_KEYS`, `ApiIcon`, `iconSrc` from `apiIcons.tsx`; `adminUploadProductIcon` from `client.ts`; the Composer `form.icon` state + `set('icon', …)` (existing pattern from other fields).

- [ ] **Step 1: Add frontend i18n labels**

In `web/src/i18n/en.ts` under the `admin` namespace add an `icon` group:

```ts
    icon: {
      label: 'Icon',
      defaultOption: 'Default',
      builtinHeading: 'Built-in',
      uploadCta: 'Upload custom…',
      uploadHint: 'Save the API first, then upload a custom icon.',
      uploading: 'Uploading…',
    },
```

In `web/src/i18n/fr.ts` mirror (French):

```ts
    icon: {
      label: 'Icône',
      defaultOption: 'Par défaut',
      builtinHeading: 'Intégrées',
      uploadCta: 'Importer une icône…',
      uploadHint: 'Enregistrez d’abord l’API, puis importez une icône personnalisée.',
      uploading: 'Import…',
    },
```

- [ ] **Step 2: Write the failing test**

In `web/src/pages/admin/ProductsPage.test.tsx`, add tests (mirror the file's existing render/open-composer helpers):

```tsx
it('shows the built-in icon grid and selects a glyph', async () => {
  // open the Composer in create mode (reuse the file's helper, e.g. openCreate())
  await openCreate()
  const grid = screen.getByTestId('icon-picker')
  expect(grid).toBeTruthy()
  // 9 built-in glyphs + 1 Default tile
  expect(grid.querySelectorAll('button.icon-tile').length).toBe(10)
  fireEvent.click(grid.querySelector('button.icon-tile[data-key="pizza"]')!)
  expect(grid.querySelector('button.icon-tile[data-key="pizza"]')!.getAttribute('aria-pressed')).toBe('true')
})

it('disables custom upload in create mode with a hint', async () => {
  await openCreate()
  expect((screen.getByTestId('icon-upload') as HTMLInputElement).disabled).toBe(true)
  expect(screen.getByTestId('icon-upload-hint')).toBeTruthy()
})
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `cd web && npx vitest run src/pages/admin/ProductsPage.test.tsx`
Expected: FAIL — no `icon-picker` testid.

- [ ] **Step 4: Implement the Composer icon field**

In `web/src/pages/admin/ProductsPage.tsx`, import `BUILTIN_ICON_KEYS, ApiIcon, iconSrc` from `'../../components/apiIcons'` and `adminUploadProductIcon` from `'../../api/client'`, plus `useState`. Add an upload-status state near the other Composer state:

```tsx
const [iconV, setIconV] = useState<number>(0)      // cache-bust after replace
const [iconErr, setIconErr] = useState<string>('')
const [iconBusy, setIconBusy] = useState(false)
```

Render the field inside the Composer form (near the other fields; `editing` is the product being edited or `undefined` on create; `token` is the admin token already used by the page):

```tsx
<label>{t('admin.icon.label')}</label>
<div className="icon-picker" data-testid="icon-picker">
  <button type="button" className="icon-tile" data-key="" aria-pressed={form.icon === ''}
    title={t('admin.icon.defaultOption')} onClick={() => set('icon', '')}>
    <span className="icon-default">—</span>
  </button>
  {BUILTIN_ICON_KEYS.map(k => (
    <button type="button" key={k} className="icon-tile" data-key={k} aria-pressed={form.icon === k}
      title={k} onClick={() => set('icon', k)}>
      <ApiIcon name={k} />
    </button>
  ))}
  {form.icon === 'upload' && editing && (
    <span className="icon-tile is-upload" aria-pressed="true">
      <img className="ico-img" src={iconSrc(editing.slug, iconV || undefined)} alt="" width={24} height={24} />
    </span>
  )}
</div>
<input
  type="file" data-testid="icon-upload" accept="image/png,image/jpeg,image/webp"
  disabled={!editing || iconBusy}
  onChange={async e => {
    const f = e.target.files?.[0]
    if (!f || !editing) return
    setIconErr(''); setIconBusy(true)
    try {
      const { updatedAt } = await adminUploadProductIcon(token, editing.id, f)
      set('icon', 'upload')
      setIconV(new Date(updatedAt).getTime())
    } catch (err) {
      setIconErr(err instanceof Error ? err.message : String(err))
    } finally {
      setIconBusy(false)
      e.target.value = ''
    }
  }}
/>
{!editing && <p className="hint" data-testid="icon-upload-hint">{t('admin.icon.uploadHint')}</p>}
{iconBusy && <p className="hint">{t('admin.icon.uploading')}</p>}
{iconErr && <p className="err" role="alert">{iconErr}</p>}
```

Notes for the implementer:
- `set(field, value)` is the Composer's existing state setter (used by every other field). `form.icon` already exists in the form state (initialized from `editing ?? { …, icon: '' }`).
- `editing` may be named differently in the file (e.g. the product being edited). Use whatever the file already binds; on create it is falsy.
- Minimal CSS: add to `web/src/styles/` where the Composer styles live — an `.icon-picker` flex-wrap grid and `.icon-tile` square buttons (~32px) with an `[aria-pressed="true"]` outline in the accent color. Match the existing Composer field styling.

- [ ] **Step 5: Run the tests + build**

Run: `cd web && npx vitest run src/pages/admin/ProductsPage.test.tsx && pnpm build`
Expected: PASS; build succeeds.

- [ ] **Step 6: Full frontend + backend suites**

Run: `cd web && npx vitest run` then `cd .. && go test ./internal/... ./cmd/...`
Expected: all green (DB/e2e-gated tests skip without their env).

- [ ] **Step 7: Commit**

```bash
git add web/src/pages/admin/ProductsPage.tsx web/src/pages/admin/ProductsPage.test.tsx web/src/i18n/en.ts web/src/i18n/fr.ts web/src/styles
git commit -m "feat(icons): Composer icon picker (built-in grid + Default) + custom upload"
```

---

## Notes for live verification (after all tasks)

With `make full` up: edit a product in `/admin/products`, pick a built-in glyph → catalog card shows it; upload a PNG/JPEG/WebP → card shows the custom image; upload an SVG or a 2 MB file → localized error; `GET /api/products/<slug>/icon` returns `image/png` with an `ETag`; switch the product back to a built-in glyph → the blob row is gone (`SELECT count(*) FROM product_icons WHERE product_id=<id>` = 0).
