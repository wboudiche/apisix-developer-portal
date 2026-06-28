# Real Ratings & Reviews — Design

**Date:** 2026-06-28
**Status:** Approved, ready for planning
**Surface:** Catalog cards + product detail (`web/src/components/ApiCard.tsx`, `web/src/pages/ProductDetailPage.tsx`), new `internal/ratings`, `internal/catalog`.

## Problem

The catalog shows a static, seeded `rating` (e.g. 4.5) — a fake surface. Replace
it with real developer ratings + short reviews, a community signal WSO2 and
Gravitee both have.

## Locked decisions (from brainstorming)

- **Who can rate:** approved subscribers only (a developer with an approved
  subscription to that API). Others see ratings read-only.
- **Scope:** 1–5 stars + an optional short comment, **one rating per user per
  API** (editable/upserted). Detail page shows average, count, and the review
  list. No threaded comments / admin replies (deferred).
- **Seed ratings:** reset the seeded `rating` values to 0 — the displayed
  average/count reflect **only real ratings**; products with none show "Pas
  encore noté".

## Data model

- New table **`product_ratings`**:
  `id BIGSERIAL PK`, `api_product_id BIGINT NOT NULL REFERENCES api_products(id) ON DELETE CASCADE`,
  `user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE`,
  `stars SMALLINT NOT NULL CHECK (stars BETWEEN 1 AND 5)`,
  `comment TEXT NOT NULL DEFAULT ''`,
  `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`, `updated_at TIMESTAMPTZ NOT NULL DEFAULT now()`,
  `UNIQUE (api_product_id, user_id)`.
- Denormalized cache on `api_products`: keep the existing `rating` column as the
  **real average** and add `rating_count INT NOT NULL DEFAULT 0`. Both are
  recomputed in a transaction on every rating write:
  `UPDATE api_products SET rating = COALESCE((SELECT AVG(stars) FROM product_ratings WHERE api_product_id=$1),0), rating_count = (SELECT count(*) FROM product_ratings WHERE api_product_id=$1) WHERE id=$1`.
  The catalog list/sort hot path keeps reading the cached `rating` (no change).
- Migration also `UPDATE api_products SET rating = 0` (reset seeded values;
  `rating_count` defaults to 0).

## Backend — `internal/ratings`

New package + handler mounted at **`/api/ratings/`** (separate from the
`/api/products/` catalog mount so one resource can mix a public read with an
authed write).

### `GET /api/ratings/{slug}` (public)

Returns `RatingsView`:
```
{ average float64, count int, items []Review, mine *Review, canRate bool }
```
- `Review`: `{ stars int, comment string, author string, createdAt time.Time }`
  (`author` = the rater's `users.name`, fallback "Développeur").
- `items`: all reviews for the product, newest first.
- When the request carries a valid token: `mine` = the user's own rating (nil if
  none) and `canRate` = the user is an approved subscriber of this product.
  When anonymous: `mine=nil`, `canRate=false`.
- Product resolved by slug (published only) → 404 if missing/unpublished.

### `PUT /api/ratings/{slug}` (authed)

Auth applied to just this route via chi `r.With(requireAuth)`. Body
`{ stars int, comment string }`.
1. `auth.UserID` from context.
2. Resolve product by slug (published) → 404.
3. Require an approved subscription: `ApprovedAppsForProduct(userID, productID)`
   non-empty → else **403** ("abonnez-vous pour noter").
4. Validate `1 <= stars <= 5` → else **400**; trim/cap the comment (e.g. ≤ 500
   chars).
5. Upsert `product_ratings` (`ON CONFLICT (api_product_id, user_id) DO UPDATE`
   stars/comment/updated_at) and recompute the cache, in one transaction.
6. Return `200` `RatingsView` (the refreshed summary + items + mine).

### Dependencies (small interfaces, injected; testable with fakes)

- `Products`: `ProductBySlug(ctx, slug) (id int64, err error)` — published-only;
  satisfied by an adapter over `catalog.Repo.ProductBySlug`.
- `Subscribers`: `IsApprovedSubscriber(ctx, userID, productID int64) (bool, error)`
  — adapter over `subscriptions.Repo.ApprovedAppsForProduct` (non-empty).
- `Store` (the ratings repo): upsert+recompute, list reviews (join `users.name`),
  get-one (mine), summary.
- `requireAuth` (the `auth` middleware) passed to the handler for the PUT route.

Mount: `mux.Handle("/api/ratings/", ratingsH)` (no outer auth; GET public, PUT
auth-wrapped per-route).

## Frontend

### Catalog cards + detail summary

- `Product` (types) gains `ratingCount: number`; catalog `baseSelect` returns
  `rating_count`.
- `ApiCard` shows the average stars + `(N)` count, or **"Pas encore noté"** when
  `ratingCount === 0`. The product detail header shows the same.

### Product detail — Reviews section

On `/catalog/:slug`, below the docs (or in a clear section):
- Summary: average (stars) + "N avis".
- Review list: each `{stars, comment, author, relative date}` (reuse
  `formatRelative`).
- Rating form: a 1–5 star picker + optional comment + submit, shown when
  `canRate`; pre-filled from `mine` (so re-rating edits in place). When the user
  is authed-but-not-subscribed → "Abonnez-vous pour noter"; when anonymous → a
  "Connectez-vous" prompt. On submit, call the client fn and refresh the view.
- New client fns: `getRatings(slug, token?)` and `submitRating(token, slug, {stars, comment})`.
  `getRatings` sends the bearer token when present (so `mine`/`canRate` populate);
  it remains usable anonymously.

## Testing

- **Go**
  - Ratings repo (DB): upsert is one-per-user (second PUT updates, not inserts);
    recompute sets `api_products.rating` = avg and `rating_count` = count; list
    returns reviews with author name newest-first; get-mine returns the user's row.
  - Handler GET: public summary/list; with a valid token, `mine` + `canRate`
    reflect the user; unpublished/missing slug → 404.
  - Handler PUT: approved subscriber → 200 + upsert + recomputed summary;
    non-subscriber → 403; `stars` out of 1–5 → 400; anonymous → 401 (the
    per-route `requireAuth`).
  - Migration: seeded `rating` reset to 0; `rating_count` present default 0.
- **Frontend (vitest)**
  - `ApiCard`: shows avg+count; "Pas encore noté" when count 0.
  - Detail Reviews: renders list + summary; form shown when `canRate`, prompt
    otherwise; submit calls `submitRating` and the view refreshes; `mine`
    pre-fills the form.
  - `getRatings`/`submitRating` client fns hit the right URLs (token optional on GET).
- **Live (controller):** as an approved subscriber, rate the try-it/echo product
  → the average + count update on the detail and the catalog card; a second rate
  edits in place (count stays 1).

## Out of scope (deferred)

- Threaded comments / admin replies / moderation.
- Per-version ratings (ratings are per product).
- Helpfulness votes, sorting/filtering reviews.
- Deleting a rating (editable via re-submit is enough for V1).
