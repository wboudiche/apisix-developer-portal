import { describe, it, expect, vi, beforeEach } from 'vitest'
import { getProducts, login, register, getPlans, getApplications, createApplication, getApplicationDetail, subscribe } from './client'

beforeEach(() => { vi.restoreAllMocks() })

function mockFetch(status: number, body: unknown) {
  globalThis.fetch = vi.fn().mockResolvedValue({
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
  }) as unknown as typeof fetch
}

describe('getProducts', () => {
  it('GETs /api/products and returns the array', async () => {
    mockFetch(200, [{ id: 1, name: 'Orders', slug: 'orders', tags: [] }])
    const out = await getProducts({})
    expect(out).toHaveLength(1)
    expect((globalThis.fetch as any).mock.calls[0][0]).toBe('/api/products')
  })

  it('encodes query params', async () => {
    mockFetch(200, [])
    await getProducts({ search: 'pi zza', category: 'Finance', sort: 'alpha' })
    const url = (globalThis.fetch as any).mock.calls[0][0] as string
    expect(url).toContain('/api/products?')
    expect(url).toContain('search=pi+zza')
    expect(url).toContain('category=Finance')
    expect(url).toContain('sort=alpha')
  })
})

describe('login/register', () => {
  it('login POSTs credentials and returns AuthResponse', async () => {
    mockFetch(200, { user: { id: 1, email: 'a@b.c', name: '', role: 'developer' }, token: 'jwt' })
    const res = await login('a@b.c', 'pw12345678')
    expect(res.token).toBe('jwt')
    const [url, opts] = (globalThis.fetch as any).mock.calls[0]
    expect(url).toBe('/api/auth/login')
    expect(opts.method).toBe('POST')
  })

  it('register throws on non-2xx with the server error message', async () => {
    mockFetch(409, { error: 'email already registered' })
    await expect(register('a@b.c', 'pw12345678', 'A')).rejects.toThrow('email already registered')
  })
})

describe('authenticated endpoints', () => {
  it('getApplications sends the Bearer token', async () => {
    mockFetch(200, [{ id: 1, name: 'A', ownerId: 5, description: '', createdAt: '' }])
    const out = await getApplications('tok-1')
    expect(out).toHaveLength(1)
    const [url, opts] = (globalThis.fetch as any).mock.calls[0]
    expect(url).toBe('/api/applications')
    expect(opts.headers.Authorization).toBe('Bearer tok-1')
  })

  it('createApplication POSTs name with auth', async () => {
    mockFetch(201, { id: 9, name: 'New', ownerId: 5, description: '', createdAt: '' })
    const a = await createApplication('tok', 'New', '')
    expect(a.id).toBe(9)
    const [url, opts] = (globalThis.fetch as any).mock.calls[0]
    expect(url).toBe('/api/applications')
    expect(opts.method).toBe('POST')
    expect(opts.headers.Authorization).toBe('Bearer tok')
  })

  it('subscribe POSTs productId+planId to the app subscriptions URL', async () => {
    mockFetch(201, { applicationId: 9, apiKey: 'k', consumerUsername: 'app_9' })
    const cred = await subscribe('tok', 9, 3, 2)
    expect(cred.apiKey).toBe('k')
    const [url, opts] = (globalThis.fetch as any).mock.calls[0]
    expect(url).toBe('/api/applications/9/subscriptions')
    expect(JSON.parse(opts.body)).toEqual({ productId: 3, planId: 2 })
  })

  it('getApplicationDetail GETs the detail with auth', async () => {
    mockFetch(200, { apiKey: 'k', consumerUsername: 'app_9', subscriptions: [] })
    const d = await getApplicationDetail('tok', 9)
    expect(d.apiKey).toBe('k')
    expect((globalThis.fetch as any).mock.calls[0][0]).toBe('/api/applications/9')
  })

  it('getPlans returns the array', async () => {
    mockFetch(200, [{ id: 1, name: 'Free', rateLimit: 60, windowSeconds: 60 }])
    expect(await getPlans()).toHaveLength(1)
  })
})
