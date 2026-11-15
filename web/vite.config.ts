/// <reference types="vitest" />
import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    // Proxy target defaults to :8080; override with PORTAL_PROXY when the API
    // runs elsewhere (e.g. PORTAL_PROXY=http://localhost:8090).
    proxy: { '/api': { target: process.env.PORTAL_PROXY || 'http://localhost:8080', changeOrigin: true } },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: './src/setupTests.ts',
    // e2e/ holds Playwright specs, which don't load under vitest — without
    // this they surface as failed suites in a bare `vitest run`.
    exclude: ['node_modules/**', 'dist/**', 'e2e/**'],
    // `pnpm test:coverage` (CI). Floors sit just under the levels measured
    // when they were added (87.1 / 78.8 / 83.0 / 90.9) so coverage can't
    // quietly erode; raise them as coverage improves.
    coverage: {
      provider: 'v8',
      include: ['src/**/*.{ts,tsx}'],
      exclude: ['src/**/*.test.{ts,tsx}', 'src/setupTests.ts', 'src/main.tsx', 'src/**/*.d.ts'],
      reporter: ['text-summary', 'json-summary', 'html'],
      thresholds: { statements: 85, branches: 76, functions: 81, lines: 89 },
    },
  },
})
