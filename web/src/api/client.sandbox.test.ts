import { it, expect, vi, afterEach } from 'vitest'
import { enableSandbox, rotateSandboxKey } from './client'

afterEach(() => vi.restoreAllMocks())

it('enableSandbox POSTs the enable endpoint with auth', async () => {
  const f = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
    new Response(JSON.stringify({ sandboxApiKey: 'sb-1' }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
  const out = await enableSandbox('jwt', 7)
  expect(out.sandboxApiKey).toBe('sb-1')
  const [url, init] = f.mock.calls[0]
  expect(url).toBe('/api/applications/7/sandbox/enable')
  expect((init as RequestInit).method).toBe('POST')
  expect((init as RequestInit).headers).toMatchObject({ Authorization: 'Bearer jwt' })
})

it('rotateSandboxKey POSTs the rotate endpoint with auth', async () => {
  const f = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
    new Response(JSON.stringify({ sandboxApiKey: 'sb-2' }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
  const out = await rotateSandboxKey('jwt', 7)
  expect(out.sandboxApiKey).toBe('sb-2')
  expect(f.mock.calls[0][0]).toBe('/api/applications/7/sandbox/rotate')
  expect((f.mock.calls[0][1] as RequestInit).method).toBe('POST')
})
