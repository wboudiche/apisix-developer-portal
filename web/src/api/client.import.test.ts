import { it, expect, vi, afterEach } from 'vitest'
import { adminImportProduct, ApiError } from './client'

afterEach(() => vi.restoreAllMocks())

const draft = {
  name: 'Imported API', slug: 'imported', category: 'Finance', version: '1.0.0',
  contextPath: '/v1', description: '', tags: ['Finance'], icon: '', upstreamUrl: 'api.example.com:443', published: false,
}

it('POSTs a pasted spec and returns the draft', async () => {
  const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
    new Response(JSON.stringify(draft), { status: 200, headers: { 'Content-Type': 'application/json' } }),
  )
  const out = await adminImportProduct('jwt', { spec: '{"openapi":"3.0.0"}' })
  expect(out.name).toBe('Imported API')
  const [url, init] = fetchMock.mock.calls[0]
  expect(url).toBe('/api/admin/products/import')
  expect((init as RequestInit).method).toBe('POST')
  expect(JSON.parse((init as RequestInit).body as string)).toEqual({ spec: '{"openapi":"3.0.0"}' })
})

it('surfaces a 422 as an ApiError', async () => {
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(
    new Response(JSON.stringify({ error: 'spec could not be parsed' }), { status: 422, headers: { 'Content-Type': 'application/json' } }),
  )
  await expect(adminImportProduct('jwt', { url: 'https://x/y' })).rejects.toBeInstanceOf(ApiError)
})
