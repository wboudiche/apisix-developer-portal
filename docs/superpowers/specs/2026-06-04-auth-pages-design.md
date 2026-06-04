# Auth Pages Redesign — Split-Screen per `login.html` Blueprint

**Date:** 2026-06-04
**Source of truth:** `/login.html` (user-authored Atlas/APISIX blueprint)
**Scope:** `/login` and `/register` pages of the React app (`web/`)

## Goal

Replace the bare `.authcard` login/register forms with the split-screen design
the user built in `login.html`: a crimson APISIX "vitrine" panel on the left,
the form card on the right. Both pages share the layout; only the form differs.

## Decisions (user-confirmed)

1. **Scope:** Login **and** Register both get the new design.
2. **Dead UI omitted:** no "Rester connecté", no "Mot de passe oublié ?",
   no "Se connecter via votre entreprise" (SSO), no legal line
   (its Conditions/Confidentialité links are equally dead). All easy to add
   back when real features exist.
3. **Stats are live:** vitrine stats show real counts from the public catalog
   (`N API publiées`, `M catégories` = distinct categories). `99.9 %
   disponibilité` stays static. On fetch failure, fall back to the blueprint's
   `9 / 4 / 99.9%`.

## Architecture (Approach A — shared shell)

### New: `web/src/components/AuthShell.tsx`

Owns the split-screen layout. Renders:

- **Left vitrine** (`aside`): crimson gradient + grid pattern + halo
  (blueprint `.aside` with its `::before`/`::after`), brand block (white
  rounded square + APISIX triangle SVG, name "APISIX", sub "Portail
  Développeur"), eyebrow pill (`Tous les services · 100 % disponibles`),
  `h1` "Vos API, un seul portail.", tagline paragraph, stats row (live).
- **Right side** (`main`): centers `children` (the page's form card).
- **Mobile brand** is part of the shell's right side, shown < 860px when the
  vitrine is hidden (blueprint `.m-brand` / media query behavior).
- Fetches the catalog once on mount (public `getProducts({})`) to compute
  stats; no token required; failure → fallback constants.

### New: `web/src/styles/auth.css`

Port of the blueprint's `<style>` block with two changes:

1. **Tokens, not hardcoded values.** The blueprint `:root` values are the
   light-theme values of `web/src/styles/tokens.css`; use `var(--bg)`,
   `var(--surface)`, `var(--accent)`, etc. so dark mode works without extra
   rules. The vitrine's fixed dark-crimson colors stay literal (it is dark in
   both themes, by design).
2. **Scoping.** Blueprint class names that collide with `catalog.css` /
   `base.css` (at minimum `.card`, `.name`, `.sub`; verify the full list by
   grepping both stylesheets during implementation) are scoped under
   the shell root class (e.g. `.auth-shell .card { … }`) or renamed with an
   `a-` prefix where the blueprint already does so (`.a-brand`, `.a-mark`…).
   No auth rule may leak into the catalog and vice versa.

Old `.authcard` styles in `base.css` are deleted once both pages stop using
them.

### Changed: `web/src/pages/LoginPage.tsx`

Form card inside `AuthShell`, blueprint markup:

- Head: `h2` "Bon retour", sub-line "Pas encore de compte ? *Créer un compte*"
  (router `Link` to `/register`).
- Fields: email, password — blueprint `.field` structure (label, 46px input,
  focus ring). Password has the **eye show/hide toggle** (blueprint icons,
  `aria-label` swaps Afficher/Masquer, keeps focus in the input).
- Submit: gradient `.submit` button, **loading state** (spinner + disabled)
  while the `login()` promise is in flight; label "Se connecter".
- Errors: server errors (bad credentials) render in the blueprint's `.err`
  style under the form head, `role="alert"`. Client-side: empty/invalid
  fields get `.field.invalid` treatment on submit.

### Changed: `web/src/pages/RegisterPage.tsx`

Same shell + card:

- Head: `h2` "Créer un compte", sub-line "Déjà inscrit ? *Se connecter*".
- Fields: nom (optional), email, password (with eye toggle).
- Existing min-8-chars rule becomes a field-level `.invalid` error on the
  password field ("Mot de passe : 8 caractères minimum").
- Submit label "Créer le compte", same loading state.

## Behavior preserved

- Auth API calls, redirects (`nav('/')` on success), and `useAuth()` usage
  unchanged.
- Accessibility: existing `aria-label`s (`Email`, `Mot de passe`, `Nom`) and
  button roles preserved so `AuthPages.test.tsx` queries keep working.

## Testing

- `AuthPages.test.tsx` continues to pass (queries by label/role unchanged).
- New assertions: password toggle flips input `type` and `aria-label`;
  submit button disables and shows loading while the promise is pending;
  register's short-password error renders.
- Stats: unit-light — assert fallback values render when fetch rejects
  (jsdom), live values when it resolves.

## Out of scope

- Password-reset, remember-me, SSO flows (future features).
- Any change to catalog/applications/admin pages.
- Backend changes: none.
