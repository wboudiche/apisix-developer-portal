// ─────────────────────────────────────────────────────────────────────────────
// Demo placeholders for data the backend does not provide yet
// (sandbox environments, key rotation).
// When a real backend feature lands, delete its constant here and wire the API.
// The activity feed and the traffic metrics are now real (see activity.ts +
// AppDetail.events; useUsage.ts + the /usage endpoint).
// Values mirror /application.html so the page matches the blueprint.
// ─────────────────────────────────────────────────────────────────────────────

export const DEMO_SANDBOX_KEY = 'ax_test_5e8d2c1f90ab34cd67ef01a2b3c4d5e6'
export const DEMO_ROTATION = { prod: '14 mai 2026', sbx: 'hier' }

// Deterministic demo consumption width (percent) per subscription row.
export const demoBarWidth = (productId: number) => 15 + ((productId * 37) % 80)
export const demoRpm = (productId: number) => 40 + ((productId * 113) % 600)

// Quickstart fallback when the app has no active subscription yet.
export const DEMO_QUICKSTART = { apiName: 'Orders API', path: '/orders', key: 'ax_live_a3f9c1e7b240d8e5f6...' }

export function demoRotatedKey(prefix: 'ax_test_'): string {
  let s = ''
  for (let i = 0; i < 32; i++) s += '0123456789abcdef'[Math.floor(Math.random() * 16)]
  return prefix + s
}
