import { describe, it, expect, vi, beforeEach } from 'vitest'
import { getProducts, login, register, getPlans, getApplications, createApplication, getApplicationDetail, subscribe, adminGetProducts, adminCreateProduct, adminDeleteProduct, adminGetSubscriptions, adminApproveSubscription, ApiError, set401Handler } from './client'

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

describe('admin endpoints', () => {
  it('adminGetProducts sends Bearer and hits /api/admin/products', async () => {
    mockFetch(200, [{ id: 1, name: 'P', slug: 'p', category: 'C', version: '1.0.0', contextPath: '/p', description: '', tags: [], icon: '', upstreamUrl: '', published: true }])
    const out = await adminGetProducts('tok')
    expect(out).toHaveLength(1)
    const [url, opts] = (globalThis.fetch as any).mock.calls[0]
    expect(url).toBe('/api/admin/products')
    expect(opts.headers.Authorization).toBe('Bearer tok')
  })
  it('adminCreateProduct POSTs the product with auth', async () => {
    mockFetch(201, { id: 2, name: 'New', slug: 'new', category: 'C', version: '', contextPath: '/new', description: '', tags: [], icon: '', upstreamUrl: 'echo:8080', published: true })
    const p = await adminCreateProduct('tok', { name: 'New', slug: 'new', category: 'C', version: '', contextPath: '/new', description: '', tags: [], icon: '', upstreamUrl: 'echo:8080', published: true })
    expect(p.id).toBe(2)
    const [url, opts] = (globalThis.fetch as any).mock.calls[0]
    expect(url).toBe('/api/admin/products')
    expect(opts.method).toBe('POST')
    expect(opts.headers.Authorization).toBe('Bearer tok')
  })
  it('adminDeleteProduct throws the server error on 409', async () => {
    mockFetch(409, { error: 'product has active subscriptions' })
    await expect(adminDeleteProduct('tok', 5)).rejects.toThrow('product has active subscriptions')
  })
  it('adminGetSubscriptions passes the status filter', async () => {
    mockFetch(200, [])
    await adminGetSubscriptions('tok', 'pending')
    const url = (globalThis.fetch as any).mock.calls[0][0] as string
    expect(url).toBe('/api/admin/subscriptions?status=pending')
  })
  it('adminApproveSubscription POSTs to the approve URL and resolves on 204', async () => {
    mockFetch(204, {})
    await adminApproveSubscription('tok', 7)
    const [url, opts] = (globalThis.fetch as any).mock.calls[0]
    expect(url).toBe('/api/admin/subscriptions/7/approve')
    expect(opts.method).toBe('POST')
    expect(opts.headers.Authorization).toBe('Bearer tok')
  })
})

describe('ApiError', () => {
  it('carries the HTTP status on non-ok responses', async () => {
    mockFetch(409, { error: 'product has active subscriptions' })
    try {
      await adminDeleteProduct('t', 1)
      expect.unreachable('should have thrown')
    } catch (e) {
      expect(e).toBeInstanceOf(ApiError)
      expect((e as ApiError).status).toBe(409)
      expect((e as ApiError).message).toBe('product has active subscriptions')
    }
  })
})

describe('401 handling', () => {
  beforeEach(() => {
    // Install a spy handler so we can assert on redirect without jsdom location issues
    set401Handler(vi.fn())
  })

  it('clears stored auth and calls the 401 handler on authed endpoint (parse path)', async () => {
    localStorage.setItem('token', 'jwt')
    localStorage.setItem('user', '{"id":1}')
    const handler = vi.fn()
    set401Handler(handler)
    mockFetch(401, { error: 'invalid token' })
    await expect(getApplications('jwt')).rejects.toBeInstanceOf(ApiError)
    expect(localStorage.getItem('token')).toBeNull()
    expect(localStorage.getItem('user')).toBeNull()
    expect(handler).toHaveBeenCalledOnce()
  })

  it('clears stored auth and calls the 401 handler on sendAuthed path (adminDeleteProduct)', async () => {
    localStorage.setItem('token', 'jwt')
    localStorage.setItem('user', '{"id":1}')
    const handler = vi.fn()
    set401Handler(handler)
    mockFetch(401, { error: 'invalid token' })
    await expect(adminDeleteProduct('jwt', 1)).rejects.toBeInstanceOf(ApiError)
    expect(localStorage.getItem('token')).toBeNull()
    expect(localStorage.getItem('user')).toBeNull()
    expect(handler).toHaveBeenCalledOnce()
  })

  it('does NOT clear auth or call handler on a 401 from login (auth endpoint exemption)', async () => {
    localStorage.setItem('token', 'jwt')
    localStorage.setItem('user', '{"id":1}')
    const handler = vi.fn()
    set401Handler(handler)
    mockFetch(401, { error: 'invalid credentials' })
    await expect(login('bad@user.com', 'wrongpass')).rejects.toBeInstanceOf(ApiError)
    // token and user must NOT be cleared — a wrong-password login should not log the user out
    expect(localStorage.getItem('token')).toBe('jwt')
    expect(localStorage.getItem('user')).toBe('{"id":1}')
    expect(handler).not.toHaveBeenCalled()
  })

  it('does NOT clear auth or call handler on a 401 from register (auth endpoint exemption)', async () => {
    localStorage.setItem('token', 'jwt')
    localStorage.setItem('user', '{"id":1}')
    const handler = vi.fn()
    set401Handler(handler)
    mockFetch(401, { error: 'invalid credentials' })
    await expect(register('bad@user.com', 'wrongpass', 'User')).rejects.toBeInstanceOf(ApiError)
    expect(localStorage.getItem('token')).toBe('jwt')
    expect(localStorage.getItem('user')).toBe('{"id":1}')
    expect(handler).not.toHaveBeenCalled()
  })
})
