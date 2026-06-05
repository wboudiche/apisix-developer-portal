// ─────────────────────────────────────────────────────────────────────────────
// ALL demo placeholders for data the backend does not provide yet
// (metrics pipeline, sandbox environments, key rotation, activity log).
// When a real backend feature lands, delete its constant here and wire the API.
// Values mirror /application.html so the page matches the blueprint.
// ─────────────────────────────────────────────────────────────────────────────

export type StatDir = 'up' | 'down' | 'flat'

export const DEMO_STATS: ReadonlyArray<{
  icon: 'pulse' | 'calendar' | 'clock' | 'alert'
  label: string
  value: string
  unit?: string
  delta: { dir: StatDir; arrow: 'up' | 'down' | null; text: string }
}> = [
  { icon: 'pulse', label: "Requêtes · aujourd'hui", value: '18 402', delta: { dir: 'up', arrow: 'up', text: '+12,4 % vs hier' } },
  { icon: 'calendar', label: 'Ce mois-ci', value: '421 K', delta: { dir: 'flat', arrow: null, text: 'sur 1 M inclus · 42 %' } },
  { icon: 'clock', label: 'Latence p95', value: '86', unit: 'ms', delta: { dir: 'up', arrow: 'down', text: '-9 ms · plus rapide' } },
  { icon: 'alert', label: "Taux d'erreur", value: '0,21', unit: '%', delta: { dir: 'up', arrow: 'down', text: 'sous le seuil 1 %' } },
]

export const DEMO_FEED: ReadonlyArray<{ icon: 'check' | 'rotate' | 'alert' | 'plus'; lead: string; rest: string; when: string }> = [
  { icon: 'check', lead: 'Abonnement', rest: ' à Inventory API · plan Gold', when: 'il y a 2 h' },
  { icon: 'rotate', lead: 'Clé Sandbox', rest: ' régénérée', when: 'hier · 16:41' },
  { icon: 'alert', lead: 'Pic de débit', rest: ' sur Payments — 280/300 rpm', when: 'hier · 12:08' },
  { icon: 'plus', lead: 'Application créée', rest: '', when: '12 mars 2026' },
]

export const DEMO_CHART = {
  values: [12, 19, 15, 22, 18, 9, 7, 24, 28, 21, 26, 31, 29, 34],
  labels: ['1', '2', '3', '4', '5', '6', '7', '8', '9', '10', '11', '12', '13', '14'],
}

export const DEMO_USAGE_ROWS: ReadonlyArray<{ ini: string; bg: string; name: string; requests: string; share: number; errors: string; errColor: string }> = [
  { ini: 'OR', bg: 'var(--c-marketing)', name: 'Orders API', requests: '248 910', share: 59, errors: '0,14 %', errColor: 'var(--success)' },
  { ini: 'PA', bg: 'var(--c-finance)', name: 'Payments API', requests: '142 305', share: 34, errors: '0,38 %', errColor: 'oklch(58% 0.12 70)' },
  { ini: 'IN', bg: 'var(--c-eng)', name: 'Inventory API', requests: '29 880', share: 7, errors: '0,02 %', errColor: 'var(--success)' },
]

export const DEMO_SANDBOX_KEY = 'ax_test_5e8d2c1f90ab34cd67ef01a2b3c4d5e6'
export const DEMO_ROTATION = { prod: '14 mai 2026', sbx: 'hier' }

// Deterministic demo consumption width (percent) per subscription row.
export const demoBarWidth = (productId: number) => 15 + ((productId * 37) % 80)
export const demoRpm = (productId: number) => 40 + ((productId * 113) % 600)

// Quickstart fallback when the app has no active subscription yet.
export const DEMO_QUICKSTART = { path: '/orders', key: 'ax_live_a3f9c1e7b240d8e5f6...' }

export function demoRotatedKey(prefix: 'ax_test_'): string {
  let s = ''
  for (let i = 0; i < 32; i++) s += '0123456789abcdef'[Math.floor(Math.random() * 16)]
  return prefix + s
}
