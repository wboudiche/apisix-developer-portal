import type { Product, AuthResponse, ProductQuery, Plan, Application, Credential, AppDetail, AdminProduct, AdminSubscription } from './types'

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

export async function getProducts(q: ProductQuery): Promise<Product[]> {
  const params = new URLSearchParams()
  if (q.search) params.set('search', q.search)
  if (q.category) params.set('category', q.category)
  if (q.tag) params.set('tag', q.tag)
  if (q.sort) params.set('sort', q.sort)
  const qs = params.toString()
  const url = qs ? `/api/products?${qs}` : '/api/products'
  const res = await fetch(url)
  return parse<Product[]>(res, url)
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

export async function getPlans(): Promise<Plan[]> {
  return parse<Plan[]>(await fetch('/api/plans'), '/api/plans')
}

export async function getApplications(token: string): Promise<Application[]> {
  return parse<Application[]>(await fetch('/api/applications', { headers: authHeaders(token) }), '/api/applications')
}

export async function createApplication(token: string, name: string, description: string): Promise<Application> {
  return parse<Application>(await fetch('/api/applications', {
    method: 'POST', headers: authHeaders(token), body: JSON.stringify({ name, description }),
  }), '/api/applications')
}

export async function getApplicationDetail(token: string, appId: number): Promise<AppDetail> {
  const url = `/api/applications/${appId}`
  return parse<AppDetail>(await fetch(url, { headers: authHeaders(token) }), url)
}

export async function subscribe(token: string, appId: number, productId: number, planId: number): Promise<Credential> {
  const url = `/api/applications/${appId}/subscriptions`
  return parse<Credential>(await fetch(url, {
    method: 'POST', headers: authHeaders(token), body: JSON.stringify({ productId, planId }),
  }), url)
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

// --- Admin: products ---
export async function adminGetProducts(token: string): Promise<AdminProduct[]> {
  return parse<AdminProduct[]>(await fetch('/api/admin/products', { headers: authHeaders(token) }), '/api/admin/products')
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

// --- Admin: plans ---
export async function adminGetPlans(token: string): Promise<Plan[]> {
  return parse<Plan[]>(await fetch('/api/admin/plans', { headers: authHeaders(token) }), '/api/admin/plans')
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
export async function adminGetSubscriptions(token: string, status?: string): Promise<AdminSubscription[]> {
  const qs = status ? `?status=${encodeURIComponent(status)}` : ''
  const url = `/api/admin/subscriptions${qs}`
  return parse<AdminSubscription[]>(await fetch(url, { headers: authHeaders(token) }), url)
}
export async function adminApproveSubscription(token: string, id: number): Promise<void> {
  return sendAuthed('POST', `/api/admin/subscriptions/${id}/approve`, token)
}
export async function adminRejectSubscription(token: string, id: number): Promise<void> {
  return sendAuthed('POST', `/api/admin/subscriptions/${id}/reject`, token)
}
