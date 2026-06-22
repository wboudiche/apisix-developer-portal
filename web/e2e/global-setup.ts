import { execFileSync } from 'node:child_process'
import { mkdirSync, writeFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { request, type FullConfig } from '@playwright/test'
import { ADMIN, ADMIN_STATE, DEV } from './seed-data'

// Seeds the isolated e2e database with enough data to make every paginated list
// span more than one page (pageSize is 20 everywhere), then saves an admin
// storageState the admin specs reuse. Runs once, after the webServers are up.
//
// Counts are deliberately > 20 so prev/next and the "N total" display are
// exercised. The specs do not hardcode totals — they read the expected count
// from the API and assert the UI matches — so this stays robust to seed drift.

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const REPO_ROOT = path.resolve(__dirname, '..', '..')
const AUTH_FILE = ADMIN_STATE

const API = `http://localhost:${process.env.E2E_API_PORT || '8090'}`
const WEB_ORIGIN = `http://localhost:${process.env.E2E_WEB_PORT || '5173'}`

const N_PRODUCTS = 25
const N_PLANS = 22
const N_SUBS = 22

interface Auth { token: string; user: { role: string } }

export default async function globalSetup(_config: FullConfig): Promise<void> {
  const api = await request.newContext({ baseURL: API })

  // 1. Admin account: register (ignore "already exists"), promote to admin in the
  //    DB (EnsureAdminRole only runs at server start, before any user exists), then
  //    log in so the token carries the admin role the SPA reads from localStorage.
  await register(api, ADMIN)
  promoteToAdmin(ADMIN.email)
  const admin = await login(api, ADMIN)
  if (admin.user.role !== 'admin') {
    throw new Error(`admin promotion failed: role is "${admin.user.role}"`)
  }
  const authHeaders = { Authorization: `Bearer ${admin.token}` }

  // 2. Products (published so they appear in the public catalog too) and plans.
  for (let i = 0; i < N_PRODUCTS; i++) {
    const n = String(i).padStart(2, '0')
    await postOk(api, '/api/admin/products', authHeaders, {
      name: `E2E Product ${n}`,
      slug: `e2e-product-${n}`,
      category: 'E2E',
      version: '1.0.0',
      contextPath: `/e2e-product-${n}`,
      description: `Seeded product ${n} for pagination e2e`,
      tags: ['e2e'],
      icon: '',
      upstreamUrl: '', // optional; left blank so no upstream/SSRF validation applies
      published: true,
    }, [201, 409])
  }
  for (let i = 0; i < N_PLANS; i++) {
    const n = String(i).padStart(2, '0')
    await postOk(api, '/api/admin/plans', authHeaders, {
      name: `E2E Plan ${n}`,
      rateLimit: 100 + i,
      windowSeconds: 60,
    }, [201, 409])
  }

  // 3. Pending subscriptions for the approvals queue: a dev user subscribes one
  //    application to N_SUBS distinct published products. Subscribe records a
  //    PENDING row with no gateway provisioning, so APISIX need not be running.
  await register(api, DEV)
  const dev = await login(api, DEV)
  const devHeaders = { Authorization: `Bearer ${dev.token}` }

  const products = await getList<{ id: number }>(api, '/api/admin/products', authHeaders)
  const plans = await getList<{ id: number }>(api, '/api/admin/plans', authHeaders)
  if (products.length < N_SUBS || plans.length === 0) {
    throw new Error(`seed shortfall: ${products.length} products, ${plans.length} plans`)
  }
  const app = await postJson(api, '/api/applications', devHeaders, {
    name: 'E2E App', description: 'pagination e2e',
  })
  const planId = plans[0].id
  for (let i = 0; i < N_SUBS; i++) {
    await postOk(api, `/api/applications/${app.id}/subscriptions`, devHeaders, {
      productId: products[i].id, planId,
    }, [200, 201, 409])
  }

  // 4. Persist the admin session as localStorage so admin specs start logged in.
  mkdirSync(path.dirname(AUTH_FILE), { recursive: true })
  writeFileSync(AUTH_FILE, JSON.stringify({
    cookies: [],
    origins: [{
      origin: WEB_ORIGIN,
      localStorage: [
        { name: 'token', value: admin.token },
        { name: 'user', value: JSON.stringify(admin.user) },
      ],
    }],
  }, null, 2))

  await api.dispose()
}

// --- helpers ---

type Req = Awaited<ReturnType<typeof request.newContext>>

async function register(api: Req, u: typeof ADMIN): Promise<void> {
  // Ignore failures: a reused DB may already have the account, which is fine —
  // the subsequent login is the source of truth.
  await api.post('/api/auth/register', { data: u }).catch(() => undefined)
}

async function login(api: Req, u: { email: string; password: string }): Promise<Auth> {
  const res = await api.post('/api/auth/login', { data: { email: u.email, password: u.password } })
  if (!res.ok()) throw new Error(`login ${u.email} failed: ${res.status()} ${await res.text()}`)
  return res.json() as Promise<Auth>
}

function promoteToAdmin(email: string): void {
  execFileSync('docker', [
    'compose', '-p', 'portal-e2e', '-f', 'docker-compose.e2e.yml',
    'exec', '-T', 'postgres',
    'psql', '-U', 'portal', '-d', 'portal',
    '-c', `UPDATE users SET role='admin' WHERE email='${email}'`,
  ], { cwd: REPO_ROOT, stdio: 'pipe' })
}

async function postJson(
  api: Req, url: string, headers: Record<string, string>, data: unknown,
): Promise<{ id: number }> {
  const res = await api.post(url, { headers, data })
  if (!res.ok()) throw new Error(`POST ${url} failed: ${res.status()} ${await res.text()}`)
  return res.json() as Promise<{ id: number }>
}

async function postOk(
  api: Req, url: string, headers: Record<string, string>, data: unknown, accept: number[],
): Promise<void> {
  const res = await api.post(url, { headers, data })
  if (!accept.includes(res.status())) {
    throw new Error(`POST ${url} unexpected ${res.status()}: ${await res.text()}`)
  }
}

async function getList<T>(api: Req, url: string, headers: Record<string, string>): Promise<T[]> {
  const res = await api.get(`${url}?pageSize=100`, { headers })
  if (!res.ok()) throw new Error(`GET ${url} failed: ${res.status()}`)
  const body = await res.json() as { items: T[] }
  return body.items
}
