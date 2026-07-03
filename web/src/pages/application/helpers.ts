import type { Plan } from '../../api/types'
import { useLang } from '../../i18n/LanguageProvider'

export const appRef = (id: number) => `app_${id}`

export function initials(name: string): string {
  const words = name.trim().split(/\s+/).filter(Boolean)
  return words.slice(0, 2).map(w => w[0]).join('').toUpperCase() || '?'
}

export const frNum = (n: number) =>
  n.toLocaleString('fr-FR').replace(/ /g, ' ')

export function formatDate(iso: string, lang: 'fr' | 'en' = 'fr'): string {
  const d = new Date(iso)
  return isNaN(d.getTime()) ? '—' : d.toLocaleDateString(lang === 'en' ? 'en-US' : 'fr-FR', { day: 'numeric', month: 'long', year: 'numeric' })
}

export function useFormatDate() {
  const { lang } = useLang()
  return (iso: string) => formatDate(iso, lang)
}

export function rateLabel(plan: Plan | undefined): string {
  if (!plan) return '—'
  const win = plan.windowSeconds === 60 ? 'min' : `${plan.windowSeconds}s`
  return `${frNum(plan.rateLimit)} / ${win}`
}

export function maskKey(full: string): string {
  if (full.length <= 10) return full
  return full.slice(0, 8) + '•'.repeat(full.length - 10) + full.slice(-2)
}

type TFunc = (key: string, vars?: Record<string, string | number>) => string

export function statusPill(status: string, t: TFunc): { cls: string; label: string } {
  switch (status) {
    case 'active': return { cls: 'ok', label: t('app.statusActive') }
    case 'pending': return { cls: 'warn', label: t('app.statusPending') }
    case 'rejected': return { cls: 'muted', label: t('app.statusRejected') }
    default: return { cls: 'muted', label: status }
  }
}

// The "abonnement(s)" / "subscription(s)" pluralization treats 0 as singular
// (matches the pre-i18n literal `count > 1 ? 's' : ''` behavior), which does not
// match translate()'s standard n===1-only `_one`/`_other` split. Callers pick the
// full suffixed key themselves and pass `{ count }` for interpolation only.
export function subsCountKey(count: number): 'app.subsCount_one' | 'app.subsCount_other' {
  return count > 1 ? 'app.subsCount_other' : 'app.subsCount_one'
}

// Deterministic per-app glyph gradient (blueprint shows one gradient per app).
const GRADIENTS = [
  'linear-gradient(150deg,var(--c-eng),var(--accent))',
  'linear-gradient(150deg,var(--c-finance),oklch(50% 0.1 170))',
  'linear-gradient(150deg,var(--c-marketing),oklch(58% 0.13 40))',
  'linear-gradient(150deg,var(--c-admin),var(--accent-d))',
]
export const glyphGradient = (id: number) => GRADIENTS[Math.abs(id) % GRADIENTS.length]

export async function copyText(text: string): Promise<void> {
  try { await navigator.clipboard.writeText(text) } catch { /* non-secure context: best effort */ }
}
