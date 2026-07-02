import type {
  Product, AuthResponse, ProductQuery, Plan, Application, Credential, AppDetail,
  AdminProduct, AdminSubscription, Usage, UsageRange, Paginated, TryApp, Quota,
  RatingsView, Team, TeamMember,
} from './types'

// ApiError carries the HTTP status so callers can branch on it (e.g. 409 when
// deleting a product that still has active subscriptions).
export class ApiError extends Error {
  status: number
  constructor(message: string, status: number) {
    super(message)
    this.status = status
  }
}

// Injectable redirect function — defaults to navigating to /login. Replaced in
// tests via set401Handler() to avoid jsdom location stub issues.
function defaultRedirectToLogin(): void {
  if (typeof window !== 'undefined' && window.location.pathname !== '/login') {
    window.location.href = '/login'
  }
}

let _redirectToLogin: () => void = defaultRedirectToLogin

/** Override the redirect action (used in tests to spy on 401 handling). */
export function set401Handler(fn: () => void): void {
  _redirectToLogin = fn
}

/** Restore the default redirect (tests must call this after stubbing). */
export function reset401Handler(): void {
  _redirectToLogin = defaultRedirectToLogin
}

// Auth endpoints that must NOT trigger the 401 handler: a wrong-password login
// returns 401, and clearing the session + redirecting would destroy the error
// message UX and log out any existing user in a different tab.
const AUTH_ENDPOINT_PREFIX = '/api/auth/'

function handle401(status: number, url?: string): void {
  if (status !== 401) return
  if (url && url.startsWith(AUTH_ENDPOINT_PREFIX)) return
  // Always clear stored credentials first, then redirect.
  try { localStorage.removeItem('token'); localStorage.removeItem('user') } catch { /* ignore */ }
  _redirectToLogin()
}

async function parse<T>(res: Response, url?: string): Promise<T> {
  const body = await res.json().catch(() => ({}))
  if (!res.ok) {
    handle401(res.status, url)
    const msg = (body as { error?: string }).error || `request failed (${res.status})`
    throw new ApiError(msg, res.status)
  }
  return body as T
}

export interface PageOpts { page?: number; pageSize?: number }

// appendPage adds page/pageSize to an existing URLSearchParams when provided.
function appendPage(params: URLSearchParams, page?: PageOpts): void {
  if (page?.page != null) params.set('page', String(page.page))
  if (page?.pageSize != null) params.set('pageSize', String(page.pageSize))
}

export async function getProducts(q: ProductQuery, page?: PageOpts): Promise<Paginated<Product>> {
  const params = new URLSearchParams()
  if (q.search) params.set('search', q.search)
  if (q.category) params.set('category', q.category)
  if (q.tag) params.set('tag', q.tag)
  if (q.sort) params.set('sort', q.sort)
  appendPage(params, page)
  const qs = params.toString()
  const url = qs ? `/api/products?${qs}` : '/api/products'
  const res = await fetch(url)
  return parse<Paginated<Product>>(res, url)
}

export async function getProduct(slug: string): Promise<Product> {
  const url = `/api/products/${encodeURIComponent(slug)}`
  return parse<Product>(await fetch(url), url)
}

export async function getRatings(slug: string, token?: string): Promise<RatingsView> {
  const url = `/api/ratings/${encodeURIComponent(slug)}`
  const headers = token ? authHeaders(token) : undefined
  return parse<RatingsView>(await fetch(url, headers ? { headers } : undefined), url)
}
export async function submitRating(token: string, slug: string, body: { stars: number; comment: string }): Promise<RatingsView> {
  const url = `/api/ratings/${encodeURIComponent(slug)}`
  return parse<RatingsView>(await fetch(url, { method: 'PUT', headers: authHeaders(token), body: JSON.stringify(body) }), url)
}

export async function getProductSpec(slug: string): Promise<string | null> {
  const res = await fetch(`/api/products/${encodeURIComponent(slug)}/spec`)
  if (res.status === 404) return null
  if (!res.ok) throw new ApiError(`spec fetch failed (${res.status})`, res.status)
  return res.text()
}

function postJSON(url: string, body: unknown): Promise<Response> {
  return fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
}

export async function login(email: string, password: string): Promise<AuthResponse> {
  return parse<AuthResponse>(await postJSON('/api/auth/login', { email, password }), '/api/auth/login')
}

export async function register(email: string, password: string, name: string): Promise<AuthResponse> {
  return parse<AuthResponse>(await postJSON('/api/auth/register', { email, password, name }), '/api/auth/register')
}

function authHeaders(token: string): HeadersInit {
  return { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` }
}

export async function getPlans(page?: PageOpts): Promise<Paginated<Plan>> {
  const params = new URLSearchParams()
  appendPage(params, page)
  const qs = params.toString()
  const url = qs ? `/api/plans?${qs}` : '/api/plans'
  return parse<Paginated<Plan>>(await fetch(url), url)
}

export async function getApplications(token: string, page?: PageOpts): Promise<Paginated<Application>> {
  const params = new URLSearchParams()
  appendPage(params, page)
  const qs = params.toString()
  const url = qs ? `/api/applications?${qs}` : '/api/applications'
  return parse<Paginated<Application>>(await fetch(url, { headers: authHeaders(token) }), url)
}

export async function createApplication(token: string, name: string, description: string, teamId?: number): Promise<Application> {
  const body: { name: string; description: string; teamId?: number } = { name, description }
  if (teamId != null) body.teamId = teamId
  return parse<Application>(await fetch('/api/applications', {
    method: 'POST', headers: authHeaders(token), body: JSON.stringify(body),
  }), '/api/applications')
}

export async function getApplicationDetail(token: string, appId: number): Promise<AppDetail> {
  const url = `/api/applications/${appId}`
  return parse<AppDetail>(await fetch(url, { headers: authHeaders(token) }), url)
}

export async function getUsage(token: string, appId: number, range: UsageRange): Promise<Usage> {
  const url = `/api/applications/${appId}/usage?range=${range}`
  return parse<Usage>(await fetch(url, { headers: authHeaders(token) }), url)
}

export async function getQuota(token: string, appId: number): Promise<Quota> {
  const url = `/api/applications/${appId}/quota`
  return parse<Quota>(await fetch(url, { headers: authHeaders(token) }), url)
}

export async function subscribe(token: string, appId: number, productId: number, planId: number): Promise<Credential> {
  const url = `/api/applications/${appId}/subscriptions`
  return parse<Credential>(await fetch(url, {
    method: 'POST', headers: authHeaders(token), body: JSON.stringify({ productId, planId }),
  }), url)
}

export async function rotateKey(token: string, appId: number): Promise<{ apiKey: string }> {
  const url = `/api/applications/${appId}/credentials/rotate`
  return parse<{ apiKey: string }>(await fetch(url, { method: 'POST', headers: authHeaders(token) }), url)
}

export async function enableSandbox(token: string, appId: number): Promise<{ sandboxApiKey: string }> {
  const url = `/api/applications/${appId}/sandbox/enable`
  return parse<{ sandboxApiKey: string }>(await fetch(url, { method: 'POST', headers: authHeaders(token) }), url)
}

export async function setOidcClient(token: string, appId: number, clientId: string): Promise<void> {
  return sendAuthed('PUT', `/api/applications/${appId}/oidc-client`, token, { clientId })
}

export async function rotateSandboxKey(token: string, appId: number): Promise<{ sandboxApiKey: string }> {
  const url = `/api/applications/${appId}/sandbox/rotate`
  return parse<{ sandboxApiKey: string }>(await fetch(url, { method: 'POST', headers: authHeaders(token) }), url)
}

export async function unsubscribe(token: string, appId: number, productId: number): Promise<void> {
  const url = `/api/applications/${appId}/subscriptions/${productId}`
  const res = await fetch(url, { method: 'DELETE', headers: authHeaders(token) })
  if (!res.ok) {
    handle401(res.status, url)
    throw new ApiError(`unsubscribe failed (${res.status})`, res.status)
  }
}

async function sendAuthed(method: string, url: string, token: string, body?: unknown): Promise<void> {
  const res = await fetch(url, {
    method,
    headers: authHeaders(token),
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!res.ok) {
    const b = await res.json().catch(() => ({}))
    handle401(res.status, url)
    throw new ApiError((b as { error?: string }).error || `request failed (${res.status})`, res.status)
  }
}

// --- Teams ---
export async function getTeams(token: string): Promise<Team[]> {
  const url = '/api/teams'
  return parse<Team[]>(await fetch(url, { headers: authHeaders(token) }), url)
}

export async function createTeam(token: string, name: string): Promise<Team> {
  const url = '/api/teams'
  return parse<Team>(await fetch(url, { method: 'POST', headers: authHeaders(token), body: JSON.stringify({ name }) }), url)
}

export async function getTeamMembers(token: string, teamId: number): Promise<TeamMember[]> {
  const url = `/api/teams/${teamId}/members`
  return parse<TeamMember[]>(await fetch(url, { headers: authHeaders(token) }), url)
}

export async function addTeamMember(token: string, teamId: number, email: string): Promise<void> {
  return sendAuthed('POST', `/api/teams/${teamId}/members`, token, { email })
}

export async function removeTeamMember(token: string, teamId: number, userId: number): Promise<void> {
  return sendAuthed('DELETE', `/api/teams/${teamId}/members/${userId}`, token)
}

export async function renameTeam(token: string, teamId: number, name: string): Promise<void> {
  return sendAuthed('PATCH', `/api/teams/${teamId}`, token, { name })
}

export async function deleteTeam(token: string, teamId: number): Promise<void> {
  return sendAuthed('DELETE', `/api/teams/${teamId}`, token)
}

export async function getTryContext(token: string, slug: string): Promise<{ apps: TryApp[]; sandboxAvailable?: boolean }> {
  const url = `/api/try/${encodeURIComponent(slug)}/context`
  return parse<{ apps: TryApp[]; sandboxAvailable?: boolean }>(await fetch(url, { headers: authHeaders(token) }), url)
}

// --- Admin: products ---
export async function adminGetProducts(token: string, page?: PageOpts): Promise<Paginated<AdminProduct>> {
  const params = new URLSearchParams()
  appendPage(params, page)
  const qs = params.toString()
  const url = qs ? `/api/admin/products?${qs}` : '/api/admin/products'
  return parse<Paginated<AdminProduct>>(await fetch(url, { headers: authHeaders(token) }), url)
}
export async function adminCreateProduct(token: string, p: AdminProduct): Promise<AdminProduct> {
  return parse<AdminProduct>(await fetch('/api/admin/products', { method: 'POST', headers: authHeaders(token), body: JSON.stringify(p) }), '/api/admin/products')
}
export async function adminUpdateProduct(token: string, id: number, p: AdminProduct): Promise<AdminProduct> {
  const url = `/api/admin/products/${id}`
  return parse<AdminProduct>(await fetch(url, { method: 'PUT', headers: authHeaders(token), body: JSON.stringify(p) }), url)
}
export async function adminDeleteProduct(token: string, id: number): Promise<void> {
  return sendAuthed('DELETE', `/api/admin/products/${id}`, token)
}
export async function adminImportProduct(token: string, src: { spec: string } | { url: string }): Promise<AdminProduct> {
  const url = '/api/admin/products/import'
  return parse<AdminProduct>(await fetch(url, { method: 'POST', headers: authHeaders(token), body: JSON.stringify(src) }), url)
}

// --- Admin: plans ---
export async function adminGetPlans(token: string, page?: PageOpts): Promise<Paginated<Plan>> {
  const params = new URLSearchParams()
  appendPage(params, page)
  const qs = params.toString()
  const url = qs ? `/api/admin/plans?${qs}` : '/api/admin/plans'
  return parse<Paginated<Plan>>(await fetch(url, { headers: authHeaders(token) }), url)
}
export async function adminCreatePlan(token: string, p: Plan): Promise<Plan> {
  return parse<Plan>(await fetch('/api/admin/plans', { method: 'POST', headers: authHeaders(token), body: JSON.stringify(p) }), '/api/admin/plans')
}
export async function adminUpdatePlan(token: string, id: number, p: Plan): Promise<Plan> {
  const url = `/api/admin/plans/${id}`
  return parse<Plan>(await fetch(url, { method: 'PUT', headers: authHeaders(token), body: JSON.stringify(p) }), url)
}
export async function adminDeletePlan(token: string, id: number): Promise<void> {
  return sendAuthed('DELETE', `/api/admin/plans/${id}`, token)
}

// --- Admin: subscriptions (approval) ---
export async function adminGetSubscriptions(token: string, status?: string, page?: PageOpts): Promise<Paginated<AdminSubscription>> {
  const params = new URLSearchParams()
  if (status) params.set('status', status)
  appendPage(params, page)
  const qs = params.toString()
  const url = qs ? `/api/admin/subscriptions?${qs}` : '/api/admin/subscriptions'
  return parse<Paginated<AdminSubscription>>(await fetch(url, { headers: authHeaders(token) }), url)
}
export async function adminApproveSubscription(token: string, id: number): Promise<void> {
  return sendAuthed('POST', `/api/admin/subscriptions/${id}/approve`, token)
}
export async function adminRejectSubscription(token: string, id: number): Promise<void> {
  return sendAuthed('POST', `/api/admin/subscriptions/${id}/reject`, token)
}
