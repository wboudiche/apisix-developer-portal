import { describe, it, expect, vi, afterEach } from 'vitest'
import { adminUploadProductIcon } from './client'

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
