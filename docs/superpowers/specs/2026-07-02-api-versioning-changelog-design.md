# API Versioning / Changelog — Design

**Date:** 2026-07-02
**Status:** Approved, ready for planning
**Surface:** migration (`api_products.lifecycle_status`/`sunset_date` + `changelog_entries`); `internal/catalog` (Product fields + changelog read endpoint); `internal/subscriptions` (deprecation enforcement); `internal/admin` (changelog CRUD + status fields); `internal/server` (routes); `web/` (badges, notices, changelog timeline, admin editor, disabled-subscribe).

## Problem

A product today is a single API with one `version` string and no history or
lifecycle state. Developers can't see **what changed** in an API over time, and
admins have no way to signal that an API is **deprecated** or being **retired**.
This adds a per-product **changelog** and a **lifecycle status** — the standard
WSO2/Gravitee capability — without turning products into full multi-version
entities.

## Locked decisions (from brainstorming)

- **Scope: changelog + lifecycle status only.** The product stays
  single-version (the existing `version` string is unchanged). No first-class
  multiple versions, no subscribe-to-a-version, no per-version routing.
- **Lifecycle status: `active` | `deprecated` | `sunset`.** `deprecated` **and**
  `sunset` both **block new subscriptions** (existing subscriptions keep working
  — approve/rotate/use/unsubscribe all unaffected); `sunset` additionally carries
  a **date** shown in the notice. No auto-retirement/unpublish at the sunset date
  in V1.
- **Changelog entry = version + date + kind + notes.** `kind` is a keep-a-
  changelog enum (`added|changed|fixed|removed|deprecated|security`) rendered as a
  colored tag. Entries are **admin-authored**; developers see a newest-first
  timeline on the product detail page. Edit = delete + re-add in V1.
- Deprecated/sunset products **remain listed** in the catalog (with a badge),
  not hidden.
- Decomposed into **Plan A (backend)** + **Plan B (frontend)**; backend first.

## Data model — migration `0014_versioning.sql`

```sql
ALTER TABLE api_products
  ADD COLUMN lifecycle_status TEXT NOT NULL DEFAULT 'active'
    CHECK (lifecycle_status IN ('active','deprecated','sunset')),
  ADD COLUMN sunset_date DATE;   -- nullable; only meaningful when status='sunset'

CREATE TABLE changelog_entries (
    id         BIGSERIAL PRIMARY KEY,
    product_id BIGINT NOT NULL REFERENCES api_products(id) ON DELETE CASCADE,
    version    TEXT NOT NULL,
    kind       TEXT NOT NULL CHECK (kind IN ('added','changed','fixed','removed','deprecated','security')),
    notes      TEXT NOT NULL DEFAULT '',
    entry_date DATE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_changelog_product ON changelog_entries(product_id, entry_date DESC);
```

## Catalog — `internal/catalog`

- The `Product` view (and the admin product view) gain `lifecycleStatus string`
  and `sunsetDate *string` (ISO `YYYY-MM-DD`, null when unset). Populated by the
  existing product queries (select the two new columns).
- New **public** read endpoint `GET /api/products/{slug}/changelog` → the
  product's entries newest-first as `[{version, kind, notes, date}]` (mirrors the
  existing public `GET /api/products/{slug}/spec`; no auth). A `ChangelogEntry`
  repo type + `ListChangelogBySlug(ctx, slug)`.
- The catalog listing/detail queries are otherwise unchanged; deprecated/sunset
  products are still returned (the frontend badges them).

## Subscribe enforcement — `internal/subscriptions`

- `Subscribe` reads the product's `lifecycle_status` (already fetched, or an
  added select) and, when it is `deprecated` or `sunset`, returns a new
  `ErrProductDeprecated` **before** creating the subscription — mapped by the
  handler to **HTTP 409** with the message *"This API no longer accepts new
  subscriptions."* Both the key-auth and oauth2 subscribe paths are gated.
- Nothing else changes: `Approve`, `Reject`, `Unsubscribe`, key/sandbox rotation,
  and gateway routing are all status-agnostic — existing subscribers are never
  affected.

## Admin — `internal/admin`

- The product create/update payload + admin `Product` gain `lifecycleStatus`
  (validated against the enum) + `sunsetDate` (optional; validated as a date when
  present). Existing product upsert extends to persist them.
- **Changelog CRUD (admin-only):**
  - `POST /api/admin/products/{id}/changelog` `{version, kind, notes, date}` →
    inserts an entry (validates `kind` enum + `date`), returns the created entry.
  - `DELETE /api/admin/products/{id}/changelog/{entryId}` → removes it (404 if
    not found under that product).

## Frontend (Plan B)

- **Catalog card** (`ApiCard`): a muted status pill ("Déprécié" / "Sunset") for
  non-active products.
- **Product detail**: the status badge in the header + a **notice banner** —
  deprecated: *"Cette API est dépréciée et n'accepte plus de nouveaux
  abonnements."*; sunset: *"Cette API sera retirée le {sunsetDate}."* — plus a
  **Changelog** section: a newest-first timeline, each entry with a colored kind
  tag, version, date, and notes.
- **Subscribe control**: disabled with the reason shown when the product is
  deprecated/sunset (mirrors the 409); active products subscribe normally.
- **Admin**: the product Composer/admin gains a **lifecycle status** select
  (active/deprecated/sunset) + a **sunset-date** input shown only for `sunset`,
  and a **changelog editor** (list existing entries with delete + an add form:
  version, kind select, date, notes).
- **Types/client:** `Product.lifecycleStatus` + `Product.sunsetDate?`;
  `ChangelogEntry {version; kind; notes; date}`; `getChangelog(slug)`; admin
  `addChangelogEntry(token, productId, entry)` + `deleteChangelogEntry(token,
  productId, entryId)`; the two new fields on the admin product payload.

## Testing

### Backend (Go)

- **Migration:** the two columns exist with the CHECK constraint + default
  `active`; `changelog_entries` exists with its CHECK + FK cascade + index.
- **Catalog:** `Product` carries `lifecycleStatus`/`sunsetDate`;
  `ListChangelogBySlug` returns a product's entries newest-first; the endpoint is
  public and 404s an unknown slug.
- **Subscribe:** subscribing to a `deprecated` or `sunset` product returns
  `ErrProductDeprecated` (→ 409) on both the key-auth and oauth2 paths; an
  `active` product subscribes normally; an EXISTING subscriber's approve/rotate/
  unsubscribe are unaffected when a product is later deprecated.
- **Admin:** create/update round-trips `lifecycleStatus` + `sunsetDate` (invalid
  enum/date rejected); add-changelog inserts + returns the entry; delete-changelog
  removes it (404 for a wrong product/entry).

### Frontend (vitest)

- Catalog card badges only non-active products.
- Product detail: badge + the correct notice per status; the changelog timeline
  renders entries with kind tags/dates/notes; the subscribe control is disabled
  with the reason when deprecated/sunset.
- Admin: the status select + conditional sunset-date input; the changelog editor
  add/delete call the right endpoints and refresh.

### Live (controller)

As admin, set a product to **deprecated** and add two changelog entries → as a
developer, the catalog card + detail show the badge, the detail shows the
"n'accepte plus de nouveaux abonnements" notice + the changelog timeline, and
**subscribing returns 409 / the button is disabled**. Set another product to
**sunset** with a date → the detail shows the dated retirement notice. An
**active** product still subscribes normally. **Look at the UI.**

## Out of scope (deferred)

- Full first-class multi-version APIs (subscribe-to-a-version, per-version specs/
  upstreams/routing).
- Auto-retirement / auto-unpublish at the sunset date (no scheduled job; status
  is enforced on new-subscribe and displayed only).
- Email notifications to existing subscribers on deprecation (a natural
  fast-follow using the `notify` feature).
- Changelog edit-in-place (V1 is add + delete).
- Markdown rendering of changelog notes (plaintext in V1).
- Changelog RSS / webhook feeds.
