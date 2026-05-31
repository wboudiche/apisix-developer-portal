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
  },
})
