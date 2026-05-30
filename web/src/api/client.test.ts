import { describe, it, expect, vi, beforeEach } from 'vitest'
import { getProducts, login, register } from './client'

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
