import { defineConfig, devices } from '@playwright/test'

// Full-stack e2e config: a real Go API (against an isolated Postgres) plus the
// Vite dev server, driven through a real browser. No request mocking — the
// pagination behaviour is exercised end to end.
//
// Ports are chosen to dodge a busy dev box: the API runs on :8090 (8080 is often
// taken) against the e2e Postgres on :5433 (see docker-compose.e2e.yml), and Vite
// proxies /api → :8090. The `make test-e2e-web` target brings that Postgres up
// (healthy) before invoking this config.
const API_PORT = process.env.E2E_API_PORT || '8090'
const WEB_PORT = process.env.E2E_WEB_PORT || '5173'
const DATABASE_URL =
  process.env.E2E_DATABASE_URL ||
  'postgres://portal:portal@localhost:5433/portal?sslmode=disable'

const baseURL = `http://localhost:${WEB_PORT}`

export default defineConfig({
  testDir: './e2e',
  globalSetup: './e2e/global-setup.ts',
  // Pagination state is shared global data; keep specs serial within a file and
  // avoid cross-file races on the single seeded DB.
  fullyParallel: false,
  workers: 1,
  forbidOnly: !!process.env.CI,
  // One retry absorbs transient slowness on a loaded dev box (the stack shares
  // the machine with other containers); a deterministic failure still fails.
  retries: 1,
  reporter: [['list'], ['html', { open: 'never' }]],
  timeout: 30_000,
  expect: { timeout: 10_000 },
  use: {
    baseURL,
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
  },
  projects: [
    { name: 'chromium', use: { ...devices['Desktop Chrome'] } },
  ],
  webServer: [
    {
      // Go API. Run from the repo root (one level up from this config).
      command: 'go run ./cmd/portal',
      cwd: '..',
      url: `http://localhost:${API_PORT}/healthz`,
      timeout: 120_000,
      reuseExistingServer: !process.env.CI,
      env: {
        PORTAL_ADDR: `:${API_PORT}`,
        PORTAL_ENV: 'dev',
        UPSTREAM_ALLOW_PRIVATE: '1',
        DATABASE_URL,
      },
    },
    {
      // Vite SPA, proxying /api to the Go API above.
      command: `npm run dev -- --port ${WEB_PORT} --strictPort`,
      url: baseURL,
      timeout: 60_000,
      reuseExistingServer: !process.env.CI,
      env: { PORTAL_PROXY: `http://localhost:${API_PORT}` },
    },
  ],
})
