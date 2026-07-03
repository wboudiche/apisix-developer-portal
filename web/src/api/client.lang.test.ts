import { it, expect, vi, afterEach } from 'vitest'
import { getProducts } from './client'

afterEach(() => vi.unstubAllGlobals())

it('sends Accept-Language from localStorage', async () => {
  localStorage.setItem('lang', 'en')
  const f = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) =>
    new Response(JSON.stringify({ items: [], total: 0, page: 1, pageSize: 20 }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
  vi.stubGlobal('fetch', f)
  await getProducts({})
  const opts = f.mock.calls[0][1]
  expect((opts?.headers as Record<string, string>)['Accept-Language']).toBe('en')
})
