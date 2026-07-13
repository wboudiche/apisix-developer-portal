import type {
  Product, AuthResponse, RegisterResponse, ProductQuery, Plan, Application, Credential, AppDetail,
  AdminMeta, AdminProduct, AdminSubscription, Usage, UsageRange, Paginated, TryApp, Quota,
  RatingsView, Team, TeamMember, ChangelogEntry, Invoice, SettingsGroup, ProbeResult,
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

// SettingsSaveError carries field validation errors or probe failures on 422.
export class SettingsSaveError extends ApiError {
  fields?: Record<string, string>
  probe?: ProbeResult[]
  constructor(msg: string, status: number, fields?: Record<string, string>, probe?: ProbeResult[]) {
    super(msg, status)
    this.fields = fields
    this.probe = probe
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
  const res = await fetch(url, { headers: langHeaders() })
  return parse<Paginated<Product>>(res, url)
}

export async function getProduct(slug: string): Promise<Product> {
  const url = `/api/products/${encodeURIComponent(slug)}`
  return parse<Product>(await fetch(url, { headers: langHeaders() }), url)
}

export async function getRatings(slug: string, token?: string): Promise<RatingsView> {
  const url = `/api/ratings/${encodeURIComponent(slug)}`
  return parse<RatingsView>(await fetch(url, { headers: langHeaders(token) }), url)
}
export async function submitRating(token: string, slug: string, body: { stars: number; comment: string }): Promise<RatingsView> {
  const url = `/api/ratings/${encodeURIComponent(slug)}`
  return parse<RatingsView>(await fetch(url, { method: 'PUT', headers: langHeaders(token), body: JSON.stringify(body) }), url)
}

export async function getProductSpec(slug: string): Promise<string | null> {
  const res = await fetch(`/api/products/${encodeURIComponent(slug)}/spec`, { headers: langHeaders() })
  if (res.status === 404) return null
  if (!res.ok) throw new ApiError(`spec fetch failed (${res.status})`, res.status)
  return res.text()
}

export async function getChangelog(slug: string): Promise<ChangelogEntry[]> {
  const url = `/api/products/${slug}/changelog`
  return parse<ChangelogEntry[]>(await fetch(url, { headers: langHeaders() }), url)
}

function postJSON(url: string, body: unknown): Promise<Response> {
  return fetch(url, {
    method: 'POST',
    headers: { ...langHeaders(), 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
}

export async function login(email: string, password: string): Promise<AuthResponse> {
  return parse<AuthResponse>(await postJSON('/api/auth/login', { email, password }), '/api/auth/login')
}

export async function register(email: string, password: string, name: string): Promise<RegisterResponse> {
  return parse<RegisterResponse>(await postJSON('/api/auth/register', { email, password, name }), '/api/auth/register')
}

export async function verifyEmail(token: string): Promise<void> {
  await parse<unknown>(await postJSON('/api/auth/verify', { token }), '/api/auth/verify')
}

export async function resendVerification(email: string): Promise<void> {
  await parse<unknown>(await postJSON('/api/auth/resend-verification', { email }), '/api/auth/resend-verification')
}

function langHeaders(token?: string): HeadersInit {
  const h: Record<string, string> = { 'Accept-Language': localStorage.getItem('lang') || 'fr' }
  if (token) { h['Content-Type'] = 'application/json'; h['Authorization'] = `Bearer ${token}` }
  return h
}

export async function getPlans(page?: PageOpts): Promise<Paginated<Plan>> {
  const params = new URLSearchParams()
  appendPage(params, page)
  const qs = params.toString()
  const url = qs ? `/api/plans?${qs}` : '/api/plans'
  return parse<Paginated<Plan>>(await fetch(url, { headers: langHeaders() }), url)
}

export async function getApplications(token: string, page?: PageOpts): Promise<Paginated<Application>> {
  const params = new URLSearchParams()
  appendPage(params, page)
  const qs = params.toString()
  const url = qs ? `/api/applications?${qs}` : '/api/applications'
  return parse<Paginated<Application>>(await fetch(url, { headers: langHeaders(token) }), url)
}

export async function createApplication(token: string, name: string, description: string, teamId?: number): Promise<Application> {
  const body: { name: string; description: string; teamId?: number } = { name, description }
  if (teamId != null) body.teamId = teamId
  return parse<Application>(await fetch('/api/applications', {
    method: 'POST', headers: langHeaders(token), body: JSON.stringify(body),
  }), '/api/applications')
}

export async function getApplicationDetail(token: string, appId: number): Promise<AppDetail> {
  const url = `/api/applications/${appId}`
  return parse<AppDetail>(await fetch(url, { headers: langHeaders(token) }), url)
}

export async function getUsage(token: string, appId: number, range: UsageRange): Promise<Usage> {
  const url = `/api/applications/${appId}/usage?range=${range}`
  return parse<Usage>(await fetch(url, { headers: langHeaders(token) }), url)
}

export async function getQuota(token: string, appId: number): Promise<Quota> {
  const url = `/api/applications/${appId}/quota`
  return parse<Quota>(await fetch(url, { headers: langHeaders(token) }), url)
}

export async function subscribe(token: string, appId: number, productId: number, planId: number): Promise<Credential> {
  const url = `/api/applications/${appId}/subscriptions`
  return parse<Credential>(await fetch(url, {
    method: 'POST', headers: langHeaders(token), body: JSON.stringify({ productId, planId }),
  }), url)
}

export async function rotateKey(token: string, appId: number): Promise<{ apiKey: string }> {
  const url = `/api/applications/${appId}/credentials/rotate`
  return parse<{ apiKey: string }>(await fetch(url, { method: 'POST', headers: langHeaders(token) }), url)
}

export async function enableSandbox(token: string, appId: number): Promise<{ sandboxApiKey: string }> {
  const url = `/api/applications/${appId}/sandbox/enable`
  return parse<{ sandboxApiKey: string }>(await fetch(url, { method: 'POST', headers: langHeaders(token) }), url)
}

export async function setOidcClient(token: string, appId: number, clientId: string): Promise<void> {
  return sendAuthed('PUT', `/api/applications/${appId}/oidc-client`, token, { clientId })
}

export async function setMyLanguage(token: string, language: 'fr' | 'en'): Promise<void> {
  await sendAuthed('PUT', '/api/me/language', token, { language })
}

export async function rotateSandboxKey(token: string, appId: number): Promise<{ sandboxApiKey: string }> {
  const url = `/api/applications/${appId}/sandbox/rotate`
  return parse<{ sandboxApiKey: string }>(await fetch(url, { method: 'POST', headers: langHeaders(token) }), url)
}

export async function unsubscribe(token: string, appId: number, productId: number): Promise<void> {
  const url = `/api/applications/${appId}/subscriptions/${productId}`
  const res = await fetch(url, { method: 'DELETE', headers: langHeaders(token) })
  if (!res.ok) {
    handle401(res.status, url)
    throw new ApiError(`unsubscribe failed (${res.status})`, res.status)
  }
}

async function sendAuthed(method: string, url: string, token: string, body?: unknown): Promise<void> {
  const res = await fetch(url, {
    method,
    headers: langHeaders(token),
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
  return parse<Team[]>(await fetch(url, { headers: langHeaders(token) }), url)
}

export async function createTeam(token: string, name: string): Promise<Team> {
  const url = '/api/teams'
  return parse<Team>(await fetch(url, { method: 'POST', headers: langHeaders(token), body: JSON.stringify({ name }) }), url)
}

export async function getTeamMembers(token: string, teamId: number): Promise<TeamMember[]> {
  const url = `/api/teams/${teamId}/members`
  return parse<TeamMember[]>(await fetch(url, { headers: langHeaders(token) }), url)
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
  return parse<{ apps: TryApp[]; sandboxAvailable?: boolean }>(await fetch(url, { headers: langHeaders(token) }), url)
}

// --- Admin: products ---
export async function adminGetMeta(token: string): Promise<AdminMeta> {
  return parse<AdminMeta>(await fetch('/api/admin/meta', { headers: langHeaders(token) }), '/api/admin/meta')
}
export async function adminGetProducts(token: string, page?: PageOpts): Promise<Paginated<AdminProduct>> {
  const params = new URLSearchParams()
  appendPage(params, page)
  const qs = params.toString()
  const url = qs ? `/api/admin/products?${qs}` : '/api/admin/products'
  return parse<Paginated<AdminProduct>>(await fetch(url, { headers: langHeaders(token) }), url)
}
export async function adminCreateProduct(token: string, p: AdminProduct): Promise<AdminProduct> {
  return parse<AdminProduct>(await fetch('/api/admin/products', { method: 'POST', headers: langHeaders(token), body: JSON.stringify(p) }), '/api/admin/products')
}
export async function adminUpdateProduct(token: string, id: number, p: AdminProduct): Promise<AdminProduct> {
  const url = `/api/admin/products/${id}`
  return parse<AdminProduct>(await fetch(url, { method: 'PUT', headers: langHeaders(token), body: JSON.stringify(p) }), url)
}
export async function adminDeleteProduct(token: string, id: number): Promise<void> {
  return sendAuthed('DELETE', `/api/admin/products/${id}`, token)
}
export async function adminImportProduct(token: string, src: { spec: string } | { url: string }): Promise<AdminProduct> {
  const url = '/api/admin/products/import'
  return parse<AdminProduct>(await fetch(url, { method: 'POST', headers: langHeaders(token), body: JSON.stringify(src) }), url)
}

export async function addChangelogEntry(token: string, productId: number, entry: { version: string; kind: string; notes: string; date: string }): Promise<ChangelogEntry> {
  const url = `/api/admin/products/${productId}/changelog`
  return parse<ChangelogEntry>(await fetch(url, { method: 'POST', headers: langHeaders(token), body: JSON.stringify(entry) }), url)
}

export async function deleteChangelogEntry(token: string, productId: number, entryId: number): Promise<void> {
  return sendAuthed('DELETE', `/api/admin/products/${productId}/changelog/${entryId}`, token)
}

// Admin listing: unlike the public getChangelog (published-only), this shows
// entries for draft/unpublished products too.
export async function adminGetChangelog(token: string, productId: number): Promise<ChangelogEntry[]> {
  const url = `/api/admin/products/${productId}/changelog`
  return parse<ChangelogEntry[]>(await fetch(url, { headers: langHeaders(token) }), url)
}

// --- Admin: plans ---
export async function adminGetPlans(token: string, page?: PageOpts): Promise<Paginated<Plan>> {
  const params = new URLSearchParams()
  appendPage(params, page)
  const qs = params.toString()
  const url = qs ? `/api/admin/plans?${qs}` : '/api/admin/plans'
  return parse<Paginated<Plan>>(await fetch(url, { headers: langHeaders(token) }), url)
}
export async function adminCreatePlan(token: string, p: Plan): Promise<Plan> {
  return parse<Plan>(await fetch('/api/admin/plans', { method: 'POST', headers: langHeaders(token), body: JSON.stringify(p) }), '/api/admin/plans')
}
export async function adminUpdatePlan(token: string, id: number, p: Plan): Promise<Plan> {
  const url = `/api/admin/plans/${id}`
  return parse<Plan>(await fetch(url, { method: 'PUT', headers: langHeaders(token), body: JSON.stringify(p) }), url)
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
  return parse<Paginated<AdminSubscription>>(await fetch(url, { headers: langHeaders(token) }), url)
}
export async function adminApproveSubscription(token: string, id: number): Promise<void> {
  return sendAuthed('POST', `/api/admin/subscriptions/${id}/approve`, token)
}
export async function adminRejectSubscription(token: string, id: number): Promise<void> {
  return sendAuthed('POST', `/api/admin/subscriptions/${id}/reject`, token)
}

// --- Billing ---
export async function getBillingInvoices(token: string): Promise<Invoice[]> {
  return parse<Invoice[]>(await fetch('/api/billing/invoices', { headers: langHeaders(token) }), '/api/billing/invoices')
}
export async function adminGetInvoices(token: string, status?: string): Promise<Invoice[]> {
  const url = status ? `/api/admin/invoices?status=${encodeURIComponent(status)}` : '/api/admin/invoices'
  return parse<Invoice[]>(await fetch(url, { headers: langHeaders(token) }), url)
}
export async function adminPayInvoice(token: string, id: number): Promise<void> {
  return sendAuthed('POST', `/api/admin/invoices/${id}/pay`, token)
}
export async function adminVoidInvoice(token: string, id: number): Promise<void> {
  return sendAuthed('POST', `/api/admin/invoices/${id}/void`, token)
}

// --- Admin: product icon upload ---
// Multipart upload: do NOT set Content-Type — the browser sets the boundary.
export async function adminUploadProductIcon(token: string, id: number, file: File): Promise<{ updatedAt: string }> {
  const form = new FormData()
  form.append('file', file)
  const url = `/api/admin/products/${id}/icon`
  const headers: Record<string, string> = { Authorization: `Bearer ${token}` }
  const lang = localStorage.getItem('lang')
  if (lang) headers['Accept-Language'] = lang
  const res = await fetch(url, { method: 'POST', headers, body: form })
  return parse<{ updatedAt: string }>(res, url)
}

// adminFetchProductIcon fetches a product's stored icon (any publish state)
// with the admin bearer token, for the Composer's draft-icon preview — a
// plain <img src> can't send an Authorization header, so the caller renders
// the returned Blob via an object URL instead.
export async function adminFetchProductIcon(token: string, id: number): Promise<Blob> {
  const res = await fetch(`/api/admin/products/${id}/icon`, { headers: { Authorization: `Bearer ${token}` } })
  if (!res.ok) throw new ApiError(`HTTP ${res.status}`, res.status)
  return res.blob()
}

export async function adminGetSettings(token: string): Promise<SettingsGroup[]> {
  return parse<SettingsGroup[]>(await fetch('/api/admin/settings', { headers: langHeaders(token) }), '/api/admin/settings')
}

export async function adminPutSettings(token: string, values: Record<string, string>, force = false): Promise<void> {
  const res = await fetch('/api/admin/settings', {
    method: 'PUT', headers: langHeaders(token), body: JSON.stringify({ values, force }),
  })
  if (res.status === 422) {
    const body = await res.json().catch(() => ({}))
    throw new SettingsSaveError('settings save failed', 422, body.fields, body.probe)
  }
  await parse<unknown>(res, '/api/admin/settings')
}

export async function adminResetSetting(token: string, key: string): Promise<void> {
  await parse<unknown>(await fetch(`/api/admin/settings/${encodeURIComponent(key)}`, {
    method: 'DELETE', headers: langHeaders(token),
  }), '/api/admin/settings')
}

export async function adminTestSettings(token: string, values: Record<string, string>): Promise<ProbeResult[]> {
  return parse<ProbeResult[]>(await fetch('/api/admin/settings/test', {
    method: 'POST', headers: langHeaders(token), body: JSON.stringify({ values }),
  }), '/api/admin/settings/test')
}
