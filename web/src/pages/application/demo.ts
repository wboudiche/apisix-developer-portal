// ─────────────────────────────────────────────────────────────────────────────
// Demo placeholders for data the backend does not provide yet.
// The activity feed and the traffic metrics are now real (see activity.ts +
// AppDetail.events; useUsage.ts + the /usage endpoint).
// Values mirror /application.html so the page matches the blueprint.
// ─────────────────────────────────────────────────────────────────────────────

// Deterministic demo consumption width (percent) per subscription row.
export const demoBarWidth = (productId: number) => 15 + ((productId * 37) % 80)
export const demoRpm = (productId: number) => 40 + ((productId * 113) % 600)

// Quickstart fallback when the app has no active subscription yet.
export const DEMO_QUICKSTART = { apiName: 'Orders API', path: '/orders', key: 'ax_live_a3f9c1e7b240d8e5f6...' }
