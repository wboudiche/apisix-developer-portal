import { it, expect, vi, afterEach } from 'vitest'
import { rotateKey } from './client'

afterEach(() => vi.restoreAllMocks())

it('POSTs to the rotate endpoint with auth and returns the new key', async () => {
  const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
    new Response(JSON.stringify({ apiKey: 'newkey' }), { status: 200, headers: { 'Content-Type': 'application/json' } }),
  )
  const out = await rotateKey('jwt', 7)
  expect(out.apiKey).toBe('newkey')
  const [url, init] = fetchMock.mock.calls[0]
  expect(url).toBe('/api/applications/7/credentials/rotate')
  expect((init as RequestInit).method).toBe('POST')
  expect((init as RequestInit).headers).toMatchObject({ Authorization: 'Bearer jwt' })
})
