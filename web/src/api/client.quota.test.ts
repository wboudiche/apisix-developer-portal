import { it, expect, vi, afterEach } from 'vitest'
import { getQuota } from './client'

afterEach(() => vi.restoreAllMocks())

it('GETs the quota endpoint with auth', async () => {
  const body = { hasQuota: true, used: 612, limit: 1000, windowSeconds: 60, available: true }
  const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
    new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } }),
  )
  const out = await getQuota('jwt', 7)
  expect(out.used).toBe(612)
  const [url, init] = fetchMock.mock.calls[0]
  expect(url).toBe('/api/applications/7/quota')
  expect((init as RequestInit).headers).toMatchObject({ Authorization: 'Bearer jwt' })
})
