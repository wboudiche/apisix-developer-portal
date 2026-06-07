# Admin Pages — Port of `admin-products.html` Blueprint

**Date:** 2026-06-07
**Source of truth:** `/admin-products.html` (user-authored blueprint, repo root)
**Scope:** Replace the three bare admin pages of the React app (`web/`) with the
blueprint design. All three tabs in one effort (user choice).

## Decisions (user-confirmed)

1. **All three tabs**: Produits + Plans + Abonnements.
2. **Routes kept**: `/admin/products`, `/admin/plans`, `/admin/approvals`
   each render the blueprint's pill nav + their panel. Add the missing
   `/admin` → `/admin/products` redirect (today `/admin` matches no route
   and renders blank).
3. **Catégorie field**: combo fed by the categories already present in the
   catalog (`getProducts()` distinct categories) + free entry — an
   `<input list>`/datalist styled like the blueprint select. No schema change.

## Real vs blueprint-demo map

Unlike the auth/app-detail ports, **everything here is real** — the backend
already has every endpoint this design needs. The notable deltas with the
blueprint's demo JS:

| Blueprint element | Decision |
|---|---|
| Edit (pencil) → toast "Édition … (démo)" | **REAL edit instead** — the composer opens prefilled, submit = `PUT /api/admin/products/{id}` (resp. plans). The backend exists; losing edit would regress (upstream changes re-provision the live route). |
| Delete confirm copy: "Les abonnements liés seront révoqués." | Kept as modal copy, but the backend refuses with **409** when active subscriptions exist → error toast «&nbsp;Suppression impossible : abonnements actifs&nbsp;». No fake cascade. |
| Eye toggle (publish/unpublish) with toast | REAL — `PUT` with `published` flipped. Catalog visibility only (route untouched, per admin spec 2026-05-30). |
| Tab count badges (7 / 3 / 3) | REAL — products count, plans count, **pending** subscriptions count. |
| Filter «&nbsp;Filtrer les produits…&nbsp;» | Client-side over name/ctx/category/upstream (blueprint behavior). |
| Auto-slug from name | Blueprint `slugify` (lowercase, strip trailing "api", non-alnum→`-`), editable, «&nbsp;généré depuis le nom — modifiable&nbsp;». |
| Approvals rows: app → product, plan tag, «demandé par email · date» | REAL — `AdminSubscriptionView` already carries `applicationName, ownerEmail, productName, planName, createdAt`. |
| Approve / Refuse + toasts | REAL endpoints; row leaves the list on success. |
| Toast + confirm modal | **Reuse** the app-detail `Toast` and `ConfirmModal`, promoted from `web/src/pages/application/` to `web/src/components/` (kills the duplication flagged in code review). Their styles move/extend accordingly. |
| Empty states (`.empty`) | Ported (e.g. «&nbsp;Aucun abonnement en attente&nbsp;»). |

## Architecture

### Files (new)

```
web/src/pages/admin/
  AdminShell.tsx        — pill nav (real count badges), page head (title,
                          description, action-button slot)
  Composer.tsx          — collapsible create/edit card: open/close state,
                          2-col grid, field+hint, toggle switch, foot buttons
  ProductsPage.tsx      — rows + filter + composer wiring + eye/edit/delete
  PlansPage.tsx         — rows + composer + edit/delete
  ApprovalsPage.tsx     — pending rows + approve/reject
web/src/styles/admin.css — scoped port of the blueprint styles
web/src/components/Toast.tsx, ConfirmModal.tsx — moved from pages/application/
```

### Files (changed/removed)

- `web/src/App.tsx` — `/admin` redirect; routes point at the new pages.
- `web/src/pages/Admin{Products,Plans,Approvals}Page.tsx` + tests — DELETED.
- `web/src/pages/application/*` — imports of Toast/ConfirmModal updated to
  `components/`.
- The toast/scrim/modal rules (`.appdetail-toast`, `.appdetail-scrim`,
  `.dmodal`) MOVE from `appdetail.css` into `web/src/styles/overlays.css`,
  imported by the promoted components themselves — so admin pages get the
  styles without importing the application page's stylesheet. Class names
  unchanged (no test churn).
- `web/src/components/AdminNav.tsx` (current nav) — replaced by AdminShell.

### CSS scoping (same trap as previous ports)

All rules live under `.adminpage`. Collision renames against
catalog/base/auth/appdetail (collision grep is implementation step 1):
known hits `.pill`→`.apill`, `.search`→`.afilter`, `.modal`/`.scrim`
(reuse `.appdetail-scrim`/`.dmodal` instead of porting), `.btn` variants map
to blueprint names (`.btn-primary` etc.) under the `.adminpage` scope.
Category glyph colors reuse the existing `--c-*` tokens; unknown categories
fall back to a deterministic color by hash (same idea as `glyphGradient`).

### Data flow

Each page loads its own list + the two other counts for the badges
(`getAdminProducts`, `getAdminPlans` (= `getPlans`), pending
`getAdminSubscriptions('pending')`). Mutations re-fetch the affected list.
Errors render the `.autherr` pattern with `role="alert"`; mutation failures
toast. API client gains nothing new — all endpoints exist.

## Testing

RTL suites (mock `api.*`):
- Shell: three pill tabs with real counts; active tab; `/admin` redirect.
- Products: rows render real fields; filter narrows; composer create POSTs
  (auto-slug asserted); edit opens prefilled + PUTs; eye toggles published;
  delete → modal → DELETE; 409 → error toast, row stays.
- Plans: create/edit/delete; `N req / Ns` chip; ≈ req/s line.
- Approvals: rows with plan tag + requester + date; approve/reject call API
  and remove the row; empty state.
Old admin page tests removed with the pages.

## Out of scope

- Backend changes (none needed).
- OpenAPI import, per-product auto-approve, key rotation (future specs).
- Catalog/auth/application pages: untouched except the Toast/ConfirmModal
  promotion.
