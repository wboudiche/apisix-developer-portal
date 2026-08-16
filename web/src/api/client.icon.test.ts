import { describe, it, expect, vi, afterEach } from 'vitest'
import { adminUploadProductIcon, adminFetchProductIcon } from './client'

afterEach(() => vi.restoreAllMocks())

describe('adminUploadProductIcon', () => {
  it('POSTs multipart form-data with the file and bearer token', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ updatedAt: '2026-07-06T00:00:00Z' }), { status: 200 }),
    )
    const file = new File([new Uint8Array([1, 2, 3])], 'icon.png', { type: 'image/png' })
    const res = await adminUploadProductIcon('tok', 7, file)
    expect(res.updatedAt).toBe('2026-07-06T00:00:00Z')
    const [url, opts] = fetchMock.mock.calls[0]
    expect(url).toBe('/api/admin/products/7/icon')
    expect((opts as RequestInit).method).toBe('POST')
    expect(((opts as RequestInit).headers as Record<string, string>).Authorization).toBe('Bearer tok')
    expect((opts as RequestInit).body).toBeInstanceOf(FormData)
  })
})

describe('adminFetchProductIcon', () => {
  it('GETs the admin icon endpoint with the bearer token and resolves to a Blob', async () => {
    // A raw byte body, not a jsdom Blob: undici's Response constructor calls
    // .stream() on a Blob body, which jsdom's Blob doesn't implement under
    // every Node version — bytes sidestep that entirely.
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(new Uint8Array([1, 2, 3]), { status: 200, headers: { 'Content-Type': 'image/png' } }),
    )
    const res = await adminFetchProductIcon('tok', 7)
    // Not toBeInstanceOf(Blob): res.blob() resolves undici's native Blob,
    // while the jsdom test environment's ambient `Blob` is a distinct
    // constructor from a different realm — instanceof across the two can
    // never reliably pass. Assert the shape callers actually rely on.
    expect(res.size).toBe(3)
    expect(res.type).toBe('image/png')
    const [url, opts] = fetchMock.mock.calls[0]
    expect(url).toBe('/api/admin/products/7/icon')
    expect(((opts as RequestInit).headers as Record<string, string>).Authorization).toBe('Bearer tok')
  })
})
