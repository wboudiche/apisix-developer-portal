import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { getProducts, login, register, getPlans, getApplications, createApplication, getApplicationDetail, getUsage, subscribe, unsubscribe, adminGetProducts, adminGetPlans, adminCreateProduct, adminDeleteProduct, adminGetSubscriptions, adminApproveSubscription, ApiError, set401Handler, reset401Handler } from './client'

beforeEach(() => { vi.restoreAllMocks() })

function mockFetch(status: number, body: unknown) {
  globalThis.fetch = vi.fn().mockResolvedValue({
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
  }) as unknown as typeof fetch
}

describe('getProducts', () => {
  it('GETs /api/products and returns the items array', async () => {
    mockFetch(200, { items: [{ id: 1, name: 'Orders', slug: 'orders', tags: [] }], total: 1, page: 1, pageSize: 20 })
    const res = await getProducts({})
    expect((globalThis.fetch as any).mock.calls[0][0]).toBe('/api/products')
    expect(res.items).toHaveLength(1)
    expect(res.total).toBe(1)
  })

  it('encodes query params', async () => {
    mockFetch(200, { items: [], total: 0, page: 1, pageSize: 20 })
    await getProducts({ search: 'pi zza', category: 'Finance', sort: 'alpha' })
    const url = (globalThis.fetch as any).mock.calls[0][0] as string
    expect(url).toContain('/api/products?')
    expect(url).toContain('search=pi+zza')
    expect(url).toContain('category=Finance')
    expect(url).toContain('sort=alpha')
  })

  it('getProducts forwards page + pageSize', async () => {
    mockFetch(200, { items: [], total: 0, page: 2, pageSize: 10 })
    await getProducts({}, { page: 2, pageSize: 10 })
    const url = (globalThis.fetch as any).mock.calls[0][0] as string
    expect(url).toContain('page=2')
    expect(url).toContain('pageSize=10')
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
  it('getApplications sends the Bearer token and returns items', async () => {
    mockFetch(200, { items: [{ id: 1, name: 'A', ownerId: 5, description: '', createdAt: '' }], total: 1, page: 1, pageSize: 20 })
    const out = await getApplications('tok-1')
    expect(out.items).toHaveLength(1)
    const [url, opts] = (globalThis.fetch as any).mock.calls[0]
    expect(url).toBe('/api/applications')
    expect(opts.headers.Authorization).toBe('Bearer tok-1')
  })

  it('getApplications forwards page + pageSize', async () => {
    mockFetch(200, { items: [], total: 0, page: 2, pageSize: 5 })
    await getApplications('tok-1', { page: 2, pageSize: 5 })
    const url = (globalThis.fetch as any).mock.calls[0][0] as string
    expect(url).toContain('page=2')
    expect(url).toContain('pageSize=5')
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

  it('getPlans returns the items array', async () => {
    mockFetch(200, { items: [{ id: 1, name: 'Free', rateLimit: 60, windowSeconds: 60 }], total: 1, page: 1, pageSize: 20 })
    const out = await getPlans()
    expect(out.items).toHaveLength(1)
    expect(out.total).toBe(1)
  })

  it('getPlans forwards page + pageSize', async () => {
    mockFetch(200, { items: [], total: 0, page: 3, pageSize: 15 })
    await getPlans({ page: 3, pageSize: 15 })
    const url = (globalThis.fetch as any).mock.calls[0][0] as string
    expect(url).toContain('page=3')
    expect(url).toContain('pageSize=15')
  })

  it('getUsage GETs the usage URL with range + auth', async () => {
    mockFetch(200, { summary: { requestsToday: 7, monthToDate: 0, p95Ms: 12, errorRate: 0 }, series: [] })
    const u = await getUsage('tok', 9, '7d')
    expect(u.summary.requestsToday).toBe(7)
    const [url, opts] = (globalThis.fetch as any).mock.calls[0]
    expect(url).toBe('/api/applications/9/usage?range=7d')
    expect(opts.headers.Authorization).toBe('Bearer tok')
  })

  it('getUsage throws ApiError with the status when metrics are unavailable', async () => {
    mockFetch(503, { error: 'metrics unavailable' })
    await expect(getUsage('tok', 9, '24h')).rejects.toMatchObject({ status: 503 })
  })
})

describe('admin endpoints', () => {
  it('adminGetProducts sends Bearer and hits /api/admin/products, returns items', async () => {
    mockFetch(200, { items: [{ id: 1, name: 'P', slug: 'p', category: 'C', version: '1.0.0', contextPath: '/p', description: '', tags: [], icon: '', upstreamUrl: '', published: true }], total: 1, page: 1, pageSize: 20 })
    const out = await adminGetProducts('tok')
    expect(out.items).toHaveLength(1)
    const [url, opts] = (globalThis.fetch as any).mock.calls[0]
    expect(url).toBe('/api/admin/products')
    expect(opts.headers.Authorization).toBe('Bearer tok')
  })

  it('adminGetProducts forwards page + pageSize', async () => {
    mockFetch(200, { items: [], total: 0, page: 2, pageSize: 10 })
    await adminGetProducts('tok', { page: 2, pageSize: 10 })
    const url = (globalThis.fetch as any).mock.calls[0][0] as string
    expect(url).toContain('page=2')
    expect(url).toContain('pageSize=10')
  })

  it('adminGetPlans sends Bearer and hits /api/admin/plans, returns items', async () => {
    mockFetch(200, { items: [{ id: 1, name: 'Free', rateLimit: 60, windowSeconds: 60 }], total: 1, page: 1, pageSize: 20 })
    const out = await adminGetPlans('tok')
    expect(out.items).toHaveLength(1)
    const [url, opts] = (globalThis.fetch as any).mock.calls[0]
    expect(url).toBe('/api/admin/plans')
    expect(opts.headers.Authorization).toBe('Bearer tok')
  })

  it('adminGetPlans forwards page + pageSize', async () => {
    mockFetch(200, { items: [], total: 0, page: 2, pageSize: 10 })
    await adminGetPlans('tok', { page: 2, pageSize: 10 })
    const url = (globalThis.fetch as any).mock.calls[0][0] as string
    expect(url).toContain('page=2')
    expect(url).toContain('pageSize=10')
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
    mockFetch(200, { items: [], total: 0, page: 1, pageSize: 20 })
    await adminGetSubscriptions('tok', 'pending')
    const url = (globalThis.fetch as any).mock.calls[0][0] as string
    expect(url).toBe('/api/admin/subscriptions?status=pending')
  })

  it('adminGetSubscriptions returns items and total', async () => {
    mockFetch(200, { items: [{ id: 1, applicationName: 'App', ownerEmail: 'a@b.c', productName: 'P', version: '1', planName: 'Free', status: 'pending', createdAt: '' }], total: 1, page: 1, pageSize: 20 })
    const out = await adminGetSubscriptions('tok')
    expect(out.items).toHaveLength(1)
    expect(out.total).toBe(1)
  })

  it('adminGetSubscriptions forwards page + pageSize', async () => {
    mockFetch(200, { items: [], total: 0, page: 2, pageSize: 10 })
    await adminGetSubscriptions('tok', undefined, { page: 2, pageSize: 10 })
    const url = (globalThis.fetch as any).mock.calls[0][0] as string
    expect(url).toContain('page=2')
    expect(url).toContain('pageSize=10')
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
  // Each test installs its own spy via set401Handler; restore the real
  // redirect afterwards so later tests never inherit a stale stub.
  afterEach(() => { reset401Handler() })

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

  it('clears stored auth and calls the 401 handler on unsubscribe (inline path)', async () => {
    localStorage.setItem('token', 'jwt')
    localStorage.setItem('user', '{"id":1}')
    const handler = vi.fn()
    set401Handler(handler)
    mockFetch(401, { error: 'invalid token' })
    await expect(unsubscribe('jwt', 9, 3)).rejects.toBeInstanceOf(ApiError)
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
