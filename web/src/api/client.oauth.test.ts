import { it, expect, vi, afterEach } from 'vitest'
import { setOidcClient } from './client'

afterEach(() => vi.restoreAllMocks())

it('setOidcClient PUTs the oidc-client endpoint with auth + body', async () => {
  const f = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(null, { status: 200 }))
  await setOidcClient('jwt', 7, 'client-abc')
  const [url, init] = f.mock.calls[0]
  expect(url).toBe('/api/applications/7/oidc-client')
  expect((init as RequestInit).method).toBe('PUT')
  expect((init as RequestInit).headers).toMatchObject({ Authorization: 'Bearer jwt' })
  expect(JSON.parse((init as RequestInit).body as string)).toEqual({ clientId: 'client-abc' })
})

it('setOidcClient throws on a 400 (bad client id)', async () => {
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(
    new Response(JSON.stringify({ error: 'invalid client id' }), { status: 400, headers: { 'Content-Type': 'application/json' } }))
  await expect(setOidcClient('jwt', 7, 'bad id')).rejects.toThrow(/invalid client id/i)
})
