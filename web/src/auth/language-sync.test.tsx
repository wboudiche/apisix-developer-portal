import { it, expect, vi, afterEach, beforeEach } from 'vitest'
import { setMyLanguage } from '../api/client'

beforeEach(() => { localStorage.clear() })
afterEach(() => vi.restoreAllMocks())

it('setMyLanguage PUTs /api/me/language with the token + body', async () => {
  const f = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(null, { status: 204 }))
  await setMyLanguage('tok123', 'en')
  const [url, opts] = f.mock.calls[0]
  expect(url).toBe('/api/me/language')
  expect((opts as RequestInit).method).toBe('PUT')
  expect(((opts as RequestInit).headers as Record<string, string>).Authorization).toBe('Bearer tok123')
  expect(JSON.parse((opts as RequestInit).body as string)).toEqual({ language: 'en' })
})
