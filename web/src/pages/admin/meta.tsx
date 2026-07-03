import type { ReactNode } from 'react'
import { translate } from '../../i18n/t'

// Blueprint slugify: lowercase, strip a trailing "api", non-alphanumerics → "-".
export const slugify = (s: string) =>
  s.toLowerCase().trim().replace(/api$/, '').replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '')

type TFunc = (key: string, vars?: Record<string, string | number>) => string
// Callers that render inside a LanguageProvider pass their own `t`; callers
// without one (e.g. direct unit tests) fall back to French, matching the
// pre-i18n behavior.
const defaultT: TFunc = (key, vars) => translate('fr', key, vars)

// Sustained-rate labels (blueprint: rows use 0 decimals ≥1, preview uses 1).
export const planRate = (limit: number, windowS: number, t: TFunc = defaultT) => {
  const r = limit / (windowS || 1)
  return t('admin.rateSoutenu', { rate: r >= 1 ? r.toFixed(0) : r.toFixed(2) })
}
export const planPreview = (limit: number, windowS: number, t: TFunc = defaultT) => {
  const r = limit / (windowS || 1)
  return t('admin.rateSoutenu', { rate: r >= 1 ? r.toFixed(1) : r.toFixed(2) })
}

export interface CatMeta { color: string; icon: ReactNode }

// Category swatch icons (blueprint CAT_META paths, verbatim).
export const CAT_META: Record<string, CatMeta> = {
  Finance: { color: 'var(--c-finance)', icon: <path d="M12 2v20M17 5H9.5a3.5 3.5 0 000 7h5a3.5 3.5 0 010 7H6" /> },
  Marketing: { color: 'var(--c-marketing)', icon: <path d="M3 11l18-5v12L3 14v-3zM3 11v3M11.6 16.8a3 3 0 01-5.8-1" /> },
  Engineering: { color: 'var(--c-eng)', icon: <path d="M8 9l-4 3 4 3M16 9l4 3-4 3M13 6l-2 12" /> },
  Data: { color: 'var(--c-data)', icon: <><ellipse cx="12" cy="5" rx="8" ry="3" /><path d="M4 5v6c0 1.7 3.6 3 8 3s8-1.3 8-3V5M4 11v6c0 1.7 3.6 3 8 3s8-1.3 8-3v-6" /></> },
  Administration: { color: 'var(--c-admin)', icon: <><circle cx="12" cy="8" r="3.2" /><path d="M5.5 20a6.5 6.5 0 0113 0" /></> },
}

const FALLBACK_COLORS = ['var(--c-finance)', 'var(--c-marketing)', 'var(--c-eng)', 'var(--c-data)', 'var(--c-admin)']

// Known categories use the blueprint meta; anything else gets a deterministic
// color (spec) with the Administration glyph.
export function catMeta(category: string): CatMeta {
  const m = CAT_META[category]
  if (m) return m
  let h = 0
  for (const ch of category) h = (h * 31 + ch.charCodeAt(0)) >>> 0
  return { color: FALLBACK_COLORS[h % FALLBACK_COLORS.length], icon: CAT_META.Administration.icon }
}
