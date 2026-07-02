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

export function statusPill(status: string): { cls: string; label: string } {
  switch (status) {
    case 'active': return { cls: 'ok', label: 'Active' }
    case 'pending': return { cls: 'warn', label: 'En attente' }
    case 'rejected': return { cls: 'muted', label: 'Rejeté' }
    default: return { cls: 'muted', label: status }
  }
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
