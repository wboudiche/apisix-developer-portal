import path from 'node:path'
import { fileURLToPath } from 'node:url'

// Shared between global-setup (which creates these accounts) and the login smoke
// spec (which signs in as the admin through the real UI).
const __dirname = path.dirname(fileURLToPath(import.meta.url))

export const ADMIN = { email: 'admin@portal.local', password: 'adminpass1', name: 'E2E Admin' }
export const DEV = { email: 'dev-e2e@portal.local', password: 'devpass12', name: 'E2E Dev' }

// Mirrors the pageSize hardcoded in every list page/component.
export const PAGE_SIZE = 20

// Saved admin session reused by the admin specs via test.use({ storageState }).
export const ADMIN_STATE = path.resolve(__dirname, '.auth', 'admin.json')
