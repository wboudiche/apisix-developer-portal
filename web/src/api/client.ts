import type { Product, AuthResponse, ProductQuery, Plan, Application, Credential, AppDetail } from './types'

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
