# Internationalization (i18n) — Sub-project 1: Frontend UI — Design

**Date:** 2026-07-02
**Status:** Approved, ready for planning
**Surface:** new `web/src/i18n/` module; `web/src/main.tsx` (provider wrap); `web/src/components/TopBar.tsx` (toggle); date helpers; and every component with user-facing French copy (~232 strings across ~30 files in `web/src/`).

## Problem

The portal's UI is hardcoded in French. To reach a wider audience it needs to be
bilingual (French + English) with a user-visible language toggle. This is the
first of three i18n sub-projects; it delivers the **frontend UI** in both
languages and establishes *how the active locale is chosen* — the source the
later sub-projects consume.

## Decomposition (whole i18n feature — for context)

The full "French + English" feature is three independent sub-projects, each with
its own spec/plan/build cycle:
1. **Frontend UI i18n (THIS spec).** The i18n module + language toggle + full
   string sweep of the React UI.
2. **Backend API-message i18n (later).** A Go message catalog + `Accept-Language`
   negotiation so API error messages surface in the user's language (the
   frontend sends the active locale on each request).
3. **User-preference + email i18n (later).** A stored `users.language` (so it's
   cross-device and drives emails), the toggle persisting to it when logged in,
   and localized approval-loop email templates per recipient.

Built in order 1 → 2 → 3. **This spec is sub-project 1 only.**

## Locked decisions (from brainstorming)

- **Lightweight custom i18n** — no runtime dependency (no react-i18next/react-intl).
- **Default locale: auto-detect then persist.** First visit picks from
  `navigator.language` (`en*` → `en`, else `fr`); the user's toggle choice is
  saved and wins thereafter.
- **Persistence mirrors the existing `ThemeProvider`**: a `localStorage` key +
  a `documentElement` attribute, wrapped around `<App>` in `main.tsx`.
- **Full coverage**: every user-facing French string in components is replaced
  with a `t(...)` call and added to both catalogs; done area-by-area.
- Frontend-only. Backend API errors + email templates are the later sub-projects
  (out of scope here).

## The i18n module — `web/src/i18n/`

### Catalogs

- `web/src/i18n/fr.ts` + `web/src/i18n/en.ts`, each a nested object keyed by area:
  ```ts
  // fr.ts
  export const fr = {
    nav: { apis: 'APIs', applications: 'Applications', teams: 'Équipes', admin: 'Admin', login: 'Connexion' },
    auth: { login: { submit: 'Connexion', … }, register: { submit: 'Créer le compte', … } },
    catalog: { searchPlaceholder: 'Rechercher une API, un tag, un contexte…', … },
    app: { credentials: { rotate: 'Régénérer', … }, … },
    teams: { create: 'Créer', memberCount_one: '{n} membre', memberCount_other: '{n} membres', … },
    // …one namespace per feature area
  } as const
  ```
- A shared type `export type Messages = typeof fr` (from `fr.ts`); `en.ts` is
  typed `export const en: Messages = { … }`, so a **missing or misnamed English
  key is a TypeScript compile error** — the catalogs cannot drift.

### Provider — mirrors `ThemeProvider`

`web/src/i18n/LanguageProvider.tsx`:
```ts
type Lang = 'fr' | 'en'
function detect(): Lang {
  const stored = localStorage.getItem('lang')
  if (stored === 'fr' || stored === 'en') return stored
  return navigator.language?.toLowerCase().startsWith('en') ? 'en' : 'fr'
}
// context: { lang: Lang; setLang: (l: Lang) => void; t: TFunc }
// useState(detect); useEffect → documentElement.setAttribute('lang', lang) + localStorage.setItem('lang', lang)
```
- `useLang()` → `{ lang, setLang }`; `useT()` → the `t` function bound to the
  active catalog. Wrapped in `main.tsx` alongside `ThemeProvider`
  (`<ThemeProvider><LanguageProvider><AuthProvider>…`).

### `t(key, vars?)`

- `t('catalog.searchPlaceholder')` → dot-path lookup in the active catalog.
- `t('teams.memberCount', { n: 3 })` → interpolates `{n}` placeholders; supports
  the `_one`/`_other` plural convention (chosen by a `count`/`n` var) for the
  handful of pluralized strings.
- **Fallback chain**: active-lang value → French value → the key string itself,
  with a dev-only `console.warn` on a miss — nothing renders blank.

## Language toggle — `TopBar.tsx`

A compact **FR / EN** control next to the existing theme toggle, calling
`setLang`. Switching re-renders the whole app via the context. (Same visual
weight as the theme button; French label "FR", English "EN".)

## Locale-aware formatting

The existing date helper(s) (e.g. `frDate`/`frDateTime` in
`web/src/pages/application/helpers.ts` or similar) become locale-aware:
`formatDate(iso, lang)` using `Intl.DateTimeFormat(lang === 'en' ? 'en-US' :
'fr-FR', { … })`; a `useFormatDate()` hook binds the active lang so call sites
stay terse. Numbers are almost all plain counts (left as-is); any large-number/
currency formatting uses `Intl.NumberFormat` with the active locale.

## String sweep

Every user-facing hardcoded French string in `web/src/**` components (~232
across ~30 files) is replaced with a `t('...')` call and added to **both**
catalogs, done **area-by-area**: (a) shell/nav/TopBar + auth pages (the proving
ground); then (b) catalog + product detail; (c) applications; (d) admin;
(e) teams; (f) ratings/misc. "Done" = a grep for accented French string literals
in `.tsx` (tests excluded) is clean, and every visible surface toggles.

## Testing

### Unit (vitest)

- **i18n core:** `t` resolves nested keys, interpolates `{var}`, selects
  `_one`/`_other` by count, and falls back active→fr→key (with the warn); the
  `LanguageProvider` detects from `navigator.language` when unset, persists to
  `localStorage`, and sets `<html lang>`.
- **Key parity guard:** a test asserting `en` has exactly the same key set as
  `fr` (belt-and-suspenders alongside the compile-time `Messages` type).
- **Key-surface toggle:** a component test rendering a converted surface (e.g.
  TopBar nav + the login button) in French, calling `setLang('en')`, and
  asserting the English strings appear.
- Existing component tests that assert French text keep passing (default stays
  French in tests unless they toggle); update only those whose asserted string
  moved behind `t`.

### Live (controller)

Toggle FR↔EN in the browser: the whole UI (nav, catalog, product detail, an
application page, admin, teams) switches language; reload persists the choice; a
fresh English-locale browser profile defaults to English; dates render in the
active locale. **Look at the UI in both languages.**

## Out of scope (this sub-project)

- **Backend API-error localization** (sub-project 2) and **email/user-preference
  localization** (sub-project 3).
- RTL languages; a third language; translation-management tooling / extraction;
  per-string translator context notes; server-side rendering concerns.
