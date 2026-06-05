# Application Detail Page — Port of `application.html` Blueprint

**Date:** 2026-06-05
**Source of truth:** `/application.html` (user-authored blueprint, repo root)
**Scope:** Replace the Applications page of the React app (`web/`) with a
per-application detail page matching the blueprint.

## Decisions (user-confirmed)

1. **Full UI, demo data.** The whole blueprint is ported pixel-faithful. Real
   data is used wherever the backend has it; everything else shows the
   blueprint's demo values, isolated in one `demo.ts` module so the
   real/demo boundary is explicit and later backend work replaces one import.
2. **Per-app routing.** `/applications/:id` is the page; `/applications`
   redirects to the first app. The blueprint's "Changer d'application"
   switcher navigates between apps and creates new ones.
3. **Current APISIX TopBar kept.** The blueprint's topbar is the pre-rebrand
   Atlas variant — superseded by the existing `TopBar` component.

## Real vs demo data map

| Blueprint element | Source |
|---|---|
| App name, ID (`app_<id>`), glyph initials | REAL — `getApplications` / `getApplicationDetail` |
| Status pill next to name | REAL — derived: any active sub → `Active`; any pending → `En attente`; else `Sans abonnement` (muted) |
| Meta "N abonnements" + tab badge | REAL — count of subscriptions |
| Meta "Créée le …" | OMITTED (backend has no createdAt) — meta shows ID + count + switcher only |
| App switcher entries | REAL — app list; "+ Nouvelle application" opens a create modal (real `createApplication`, navigates to the new app) |
| Abonnements table: API name, context path, plan name | REAL — detail subscriptions |
| Abonnements table: Débit (e.g. `1 000 / min`) | REAL — plan `rate`/`window` via `getPlans` lookup |
| Abonnements table: status pills | REAL — active/pending/rejected → ok/warn/muted |
| Résilier | REAL — confirm modal (blueprint copy) → `unsubscribe` → row fade-out + toast + badge refresh |
| "Gérer" link in rows | OMITTED (no per-subscription management exists; a dead link teaches users it's broken) |
| Abonner une API (header + subs tab) | REAL — navigates to catalog `/` |
| Production key card | REAL key — blueprint mask (`first8 + ••• + last2`), reveal toggle, copy + toast. **"Régénérer" opens the modal but confirm shows toast "Rotation des clés à venir"** — no visual fake on a real credential |
| Sandbox key card | DEMO — fake `ax_test_…` key; reveal/copy/rotate fully functional visually (blueprint behavior) |
| Aperçu: 4 stat cards, quickstart curl block, activity feed | DEMO — blueprint values; the curl block shows the REAL gateway path of the first active subscription when one exists (`http://localhost:9080/<contextPath>`) with the real key, else blueprint sample |
| Utilisation: 14-day chart + per-API table | DEMO — blueprint data; chart animates as in blueprint |
| Consommation (rpm) bars in subs table | DEMO — deterministic per-row widths |
| Paramètres: name/description/env form | Form renders; name prefilled REAL; "Enregistrer" → toast "Modifications enregistrées (démo)" (no update endpoint) |
| Danger zone "Supprimer l'application" | Blueprint's own demo behavior: confirm modal → toast "Application supprimée (démo)" |
| Toast + confirm modal | Ported as reusable components within the page scope |
| Tab persistence | localStorage `app:tab` per blueprint |

## Architecture

### Files (new)

```
web/src/pages/application/
  AppDetailPage.tsx     — shell: data loading, header, tabs, panel switching
  AppSwitcher.tsx       — dropdown (apps list, current, + Nouvelle application)
  OverviewTab.tsx       — stats grid + quickstart card + activity feed
  CredentialsTab.tsx    — Production (real) + Sandbox (demo) key cards + security card
  SubscriptionsTab.tsx  — subscriptions table (real) + résilier flow
  UsageTab.tsx          — chart + per-API table (demo)
  SettingsTab.tsx       — details form + danger zone
  ConfirmModal.tsx      — blueprint .dmodal (title/body/confirm/danger/onConfirm)
  Toast.tsx             — blueprint toast (bottom-center, auto-hide ~1.9s)
  demo.ts               — ALL demo constants (stats, chart data, feed, sandbox key, rpm widths)
web/src/styles/appdetail.css — scoped port of the blueprint styles
```

### Files (changed/removed)

- `web/src/App.tsx` — routes: `/applications` → redirect component;
  `/applications/:id` → `AppDetailPage`. Both auth-gated as today.
- `web/src/pages/ApplicationsPage.tsx` + `.test.tsx` — DELETED (replaced).
- `web/src/styles/tokens.css` — add `--success-soft`, `--warn-soft`,
  `--danger-soft` (light + dark variants).
- `web/src/api/client.ts` — no new endpoints needed.

### CSS scoping (same trap as auth port)

CSS is bundled globally; catalog.css/base.css own `.card` (opacity:0 trap),
`.pill`, `.modal`, `.tag`, `.user`. Blueprint classes are renamed/scoped:
all rules live under `.appdetail` AND colliding class NAMES are renamed —
`.card`→`.dcard`, `.pill`→`.stpill`, `.modal`→`.dmodal`, `.tag`→`.envtag`.
Implementation starts with a collision grep of every blueprint class name
against catalog.css/base.css/auth.css; any further hit gets a rename.
The vitrine-style literals (code block dark `--ink` background, status
colors) follow the blueprint; theme-dependent colors use tokens so dark
mode works.

### Data flow

`AppDetailPage` loads on `:id` change: `getApplications(token)` (switcher +
redirect validation) + `getApplicationDetail(token, id)` + `getPlans()`
(rate lookup). Unknown id → redirect to first app (or empty state). All
mutations (unsubscribe, create app) re-fetch the detail. Errors render the
existing `.autherr` pattern with `role="alert"`.

## Testing

RTL suites for the new page (mock `api.*` as elsewhere):
- `/applications` redirects to first app; empty state when no apps.
- Header shows real name/ID/status pill; tab switching shows panels; badge
  shows real count.
- Key: masked by default, reveal toggles, copy calls clipboard + toast;
  prod rotate → modal → "à venir" toast (key text unchanged).
- Résilier: modal → confirm → `unsubscribe` called → row removed.
- Switcher: lists apps, navigates; create modal calls `createApplication`.
- Settings save + delete app show their demo toasts.
Old ApplicationsPage tests removed with the page; everything else stays green.

## Out of scope

- Backend: key rotation, app update/delete endpoints, metrics pipeline,
  sandbox environments, activity log (all future work; demo.ts marks the seams).
- Admin pages, catalog, auth pages: untouched.
