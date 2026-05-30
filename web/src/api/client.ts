import type { Product, AuthResponse, ProductQuery, Plan, Application, Credential, AppDetail, AdminProduct, AdminSubscription } from './types'

async function parse<T>(res: Response): Promise<T> {
  const body = await res.json().catch(() => ({}))
  if (!res.ok) {
    const msg = (body as { error?: string }).error || `request failed (${res.status})`
    throw new Error(msg)
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
  const res = await fetch(qs ? `/api/products?${qs}` : '/api/products')
  return parse<Product[]>(res)
}

function postJSON(url: string, body: unknown): Promise<Response> {
  return fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
}

export async function login(email: string, password: string): Promise<AuthResponse> {
  return parse<AuthResponse>(await postJSON('/api/auth/login', { email, password }))
}

export async function register(email: string, password: string, name: string): Promise<AuthResponse> {
  return parse<AuthResponse>(await postJSON('/api/auth/register', { email, password, name }))
}

function authHeaders(token: string): HeadersInit {
  return { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` }
}

export async function getPlans(): Promise<Plan[]> {
  return parse<Plan[]>(await fetch('/api/plans'))
}

export async function getApplications(token: string): Promise<Application[]> {
  return parse<Application[]>(await fetch('/api/applications', { headers: authHeaders(token) }))
}

export async function createApplication(token: string, name: string, description: string): Promise<Application> {
  return parse<Application>(await fetch('/api/applications', {
    method: 'POST', headers: authHeaders(token), body: JSON.stringify({ name, description }),
  }))
}

export async function getApplicationDetail(token: string, appId: number): Promise<AppDetail> {
  return parse<AppDetail>(await fetch(`/api/applications/${appId}`, { headers: authHeaders(token) }))
}

export async function subscribe(token: string, appId: number, productId: number, planId: number): Promise<Credential> {
  return parse<Credential>(await fetch(`/api/applications/${appId}/subscriptions`, {
    method: 'POST', headers: authHeaders(token), body: JSON.stringify({ productId, planId }),
  }))
}

export async function unsubscribe(token: string, appId: number, productId: number): Promise<void> {
  const res = await fetch(`/api/applications/${appId}/subscriptions/${productId}`, {
    method: 'DELETE', headers: authHeaders(token),
  })
  if (!res.ok) throw new Error(`unsubscribe failed (${res.status})`)
}

async function sendAuthed(method: string, url: string, token: string, body?: unknown): Promise<void> {
  const res = await fetch(url, {
    method,
    headers: authHeaders(token),
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!res.ok) {
    const b = await res.json().catch(() => ({}))
    throw new Error((b as { error?: string }).error || `request failed (${res.status})`)
  }
}

// --- Admin: products ---
export async function adminGetProducts(token: string): Promise<AdminProduct[]> {
  return parse<AdminProduct[]>(await fetch('/api/admin/products', { headers: authHeaders(token) }))
}
export async function adminCreateProduct(token: string, p: AdminProduct): Promise<AdminProduct> {
  return parse<AdminProduct>(await fetch('/api/admin/products', { method: 'POST', headers: authHeaders(token), body: JSON.stringify(p) }))
}
export async function adminUpdateProduct(token: string, id: number, p: AdminProduct): Promise<AdminProduct> {
  return parse<AdminProduct>(await fetch(`/api/admin/products/${id}`, { method: 'PUT', headers: authHeaders(token), body: JSON.stringify(p) }))
}
export async function adminDeleteProduct(token: string, id: number): Promise<void> {
  return sendAuthed('DELETE', `/api/admin/products/${id}`, token)
}

// --- Admin: plans ---
export async function adminGetPlans(token: string): Promise<Plan[]> {
  return parse<Plan[]>(await fetch('/api/admin/plans', { headers: authHeaders(token) }))
}
export async function adminCreatePlan(token: string, p: Plan): Promise<Plan> {
  return parse<Plan>(await fetch('/api/admin/plans', { method: 'POST', headers: authHeaders(token), body: JSON.stringify(p) }))
}
export async function adminUpdatePlan(token: string, id: number, p: Plan): Promise<Plan> {
  return parse<Plan>(await fetch(`/api/admin/plans/${id}`, { method: 'PUT', headers: authHeaders(token), body: JSON.stringify(p) }))
}
export async function adminDeletePlan(token: string, id: number): Promise<void> {
  return sendAuthed('DELETE', `/api/admin/plans/${id}`, token)
}

// --- Admin: subscriptions (approval) ---
export async function adminGetSubscriptions(token: string, status?: string): Promise<AdminSubscription[]> {
  const qs = status ? `?status=${encodeURIComponent(status)}` : ''
  return parse<AdminSubscription[]>(await fetch(`/api/admin/subscriptions${qs}`, { headers: authHeaders(token) }))
}
export async function adminApproveSubscription(token: string, id: number): Promise<void> {
  return sendAuthed('POST', `/api/admin/subscriptions/${id}/approve`, token)
}
export async function adminRejectSubscription(token: string, id: number): Promise<void> {
  return sendAuthed('POST', `/api/admin/subscriptions/${id}/reject`, token)
}
