# Product Icons — Built-in Picker + Secure Custom Upload — Design

**Date:** 2026-07-06
**Status:** Approved, ready for planning
**Surface:** `internal/db/migrations/0018_product_icons.sql`; new `internal/admin` icon
handler + repo (or additions to the product handler/repo); public icon-serve
endpoint on the catalog handler; `internal/config` (no change); frontend
`web/src/pages/admin/ProductsPage.tsx` (Composer icon field), `ApiCard.tsx`,
`ProductDetailPage.tsx`, `apiIcons.tsx`, `api/client.ts`, `types.ts`, fr/en i18n
catalogs; one new backend dep `golang.org/x/image/webp` (decode-only).

## Problem

Adding an API through the admin Composer offers **no way to choose an icon**.
The `api_products.icon` column exists and `ApiIcon` renders one of **9 built-in
SVG glyphs** (`seo`, `reviews`, `stock`, `test`, `keyword`, `people`,
`currency`, `phone`, `pizza`) or a generic fallback square — but the Composer
never sets it, so every UI-created API renders the fallback. The 9 glyphs only
appear on the seeded demo APIs because `0002_seed.sql` set the column. This adds
(1) a **built-in glyph picker** in the Composer and (2) a **secure custom icon
upload**, so admins can brand any API.

## Locked decisions (from brainstorming)

- **Upload format: raster only** — PNG / JPEG / WebP. Decode + re-encode
  server-side to a clean PNG. SVG is rejected (it can carry scripts → XSS); a
  re-encoded raster cannot execute and cannot be a polyglot.
- **Storage: blob table + serve via endpoint** (approach A). The image lives in
  its own Postgres table and is streamed by a portal endpoint. Not a data-URI in
  the catalog JSON (would bloat every list response), not filesystem/object
  storage (the portal is deliberately DB-only + ephemeral distroless).
- **WebP kept** (one small official decode-only dep `golang.org/x/image/webp`;
  PNG+JPEG are stdlib).
- **Custom upload is edit-mode-only** — a blob attaches to an existing product
  id. The built-in picker works at create; upload becomes available after the
  product is saved.

## Data model

Reuse `api_products.icon TEXT`. Its value is now one of:

- a **built-in key** — one of the 9 glyph names above;
- the empty string `''` — default fallback square (unchanged behaviour);
- the sentinel **`"upload"`** — a custom PNG is stored for this product.

New migration **`0018_product_icons.sql`**:

```sql
CREATE TABLE product_icons (
  product_id BIGINT PRIMARY KEY REFERENCES api_products(id) ON DELETE CASCADE,
  data       BYTEA NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

No `content_type` column: every stored blob is a portal-re-encoded **PNG**, so
the serve endpoint always sets `image/png`. The blob lives in its own table so
the wide catalog SELECT (`internal/catalog/repo.go` `baseSelect`) never carries
image bytes. `ON DELETE CASCADE` drops the icon when the product is deleted.

## Upload pipeline — security core

`POST /api/admin/products/{id}/icon`, behind `requireAdmin` (same gate as product
CRUD), `Content-Type: multipart/form-data` with a single `file` part. Steps, in
order (each failure returns a localized error; see Error Handling):

1. **Tight body cap.** Wrap the body in `http.MaxBytesReader(w, r.Body, 256<<10)`
   (256 KiB) before parsing — well under the global 1 MiB cap. Exceeding it →
   **413**.
2. **Content sniff.** `http.DetectContentType(first512)` must be one of
   `image/png`, `image/jpeg`, `image/webp`. The multipart filename and the
   part's declared `Content-Type` are **ignored** (untrusted). Otherwise → **415**.
3. **Dimension guard (before full decode).** `image.DecodeConfig` reads
   width/height without decompressing pixels. Reject `width>512 || height>512` or
   `width<16 || height<16` → **422**. This is the decompression-bomb guard: a
   tiny file can claim huge dimensions.
4. **Full decode.** `image.Decode` (with `image/png`, `image/jpeg`,
   `golang.org/x/image/webp` decoders registered) must succeed → proves a real
   raster, not a polyglot with image-looking magic bytes. Failure → **422**.
5. **Re-encode.** `png.Encode` the decoded `image.Image` into a **fresh** buffer.
   The stored bytes are entirely portal-generated; all original bytes (EXIF, ICC,
   trailing/polyglot data, any embedded payload) are discarded.
6. **Store transactionally.** In one tx: upsert `product_icons`
   (`INSERT … ON CONFLICT (product_id) DO UPDATE SET data=…, updated_at=now()`)
   **and** `UPDATE api_products SET icon='upload' WHERE id=$1`. Product-not-found
   → **404**. Success → **204**.

The pipeline is a single pure function
`decodeAndReencode([]byte) ([]byte, error)` (steps 2–5) so it is unit-testable
without HTTP, plus a thin handler for steps 1 and 6.

## Serving

`GET /api/products/{slug}/icon` — **public** (the catalog is public), mounted on
the catalog handler. Slug-based to match the sibling public sub-resources
(`/api/products/{slug}/spec`, `/api/products/{slug}/changelog`); chi requires the
same wildcard name at that position, so it is `{slug}`, not `{id}`. (Admin upload
stays id-based: `/api/admin/products/{id}/icon`, matching the admin router.)

- SELECT `pi.data, pi.updated_at FROM product_icons pi JOIN api_products p ON
  p.id = pi.product_id WHERE p.slug = $1`; **404** when absent.
- Headers: `Content-Type: image/png`; `Cache-Control: public, max-age=60`;
  `ETag: "<updated_at unix>"`. On `If-None-Match` matching the ETag → **304**.
  `X-Content-Type-Options: nosniff` is already applied globally by
  `httpx.SecurityHeaders`.
- Because the stored bytes are always a re-encoded PNG, the declared content-type
  can never disagree with the payload.

## Reverting / switching icons

Choosing a built-in glyph (or Default) in the picker and saving the product sets
`icon` to that key/`''`. The product-update path (`internal/admin` product
service) **deletes any `product_icons` row** in the same tx when the new `icon`
value is not `"upload"`, so no orphaned blob lingers. No separate delete
endpoint — reverting = pick a built-in or Default.

## UI — Composer "Icon" field

In `ProductsPage.tsx` Composer:

- A **grid of the 9 built-in glyphs** (rendered with the existing `ApiIcon`) plus
  a **Default** (empty) tile. Selecting one sets `form.icon` to that key / `''`.
- An **Upload** control (`<input type="file" accept="image/png,image/jpeg,image/webp">`).
  Enabled only in **edit mode** (product already has an id). On create, a
  one-line hint reads: *save the API first, then upload a custom icon*. On file
  select → `POST` to the icon endpoint; on success the picker shows the custom
  image as the selected icon and `form.icon` becomes `"upload"`.
- **Preview** of the current selection: built-in → `<ApiIcon name={icon}>`;
  custom → `<img src="/api/products/{slug}/icon?v={updatedAt}">` (the `?v`
  cache-busts after a replace).

`ApiCard.tsx` and `ProductDetailPage.tsx` gain the same branch: `icon==='upload'`
→ `<img src="/api/products/{slug}/icon">`; else `<ApiIcon name={icon}>`. `apiIcons.tsx`
exports the list of built-in keys so the picker and the render branch share one
source of truth.

Client (`api/client.ts`): `adminUploadProductIcon(token, id, file)` (multipart),
returning the new `updatedAt`; the icon URL is a plain path so `<img>` fetches it
directly (public, no auth header). `types.ts`: **no payload shape change** — the
`icon` string already exists, and no `iconUpdatedAt` is added to the
catalog/product responses. Cache freshness is handled two ways: catalog/detail
viewers rely on the serve endpoint's `Cache-Control: max-age=60` + ETag; the
admin Composer, which just replaced the icon, cache-busts its own preview with
`?v={updatedAt}` from the POST response.

## Error handling

All messages via the fr/en i18n catalogs (keys under `admin.icon.*`), returned by
`httpx.ErrorT`:

- **413** `admin.icon.tooLarge` — body over 256 KiB.
- **415** `admin.icon.badType` — sniffed type not PNG/JPEG/WebP.
- **422** `admin.icon.undecodable` — dimensions out of `[16,512]` or decode failed.
- **404** — product not found (upload) / no custom icon (serve).
- Frontend surfaces the localized message via `role="alert"` next to the Upload
  control, consistent with the rest of the Composer.

## Testing / acceptance

**Backend unit (pure pipeline):** `decodeAndReencode` — a real small PNG, JPEG,
and WebP each round-trip to a valid PNG; an SVG, a text file with a `.png` name,
and bytes with faked PNG magic are all rejected; a `600×600` image is rejected on
dimensions; a valid image with appended trailing bytes re-encodes to a clean PNG
whose length differs from the input (payload stripped).

**Backend handler:** upload as admin → 204 and a subsequent GET returns
`image/png` bytes; upload as non-admin → 403; upload to a missing product → 404;
GET for a product with no custom icon → 404; oversized body → 413; wrong type →
415. **Repo (DB) test:** upsert replaces on conflict; product delete cascades the
icon row; setting a built-in icon deletes the blob.

**Frontend (vitest):** the picker renders the 9 glyphs + Default and setting one
updates form state; `adminUploadProductIcon` posts multipart to the right URL;
`ApiCard`/`ProductDetailPage` render `<img>` when `icon==='upload'` and `<ApiIcon>`
otherwise; the Upload control is disabled in create mode with the hint shown.

Not requiring a full-stack E2E — the feature is CRUD + byte serving fully covered
by unit + handler + component tests.

## Out of scope

- SVG upload (rejected by the format decision; would need an ongoing-maintenance
  sanitizer).
- Image cropping/resizing UI, animated icons, per-theme icons.
- Uploading a custom icon during product **create** (edit-mode-only by decision;
  built-in picker covers create).
- A CDN / object-storage backend (portal stays DB-only).
- Bulk icon import or icon reuse across products.
