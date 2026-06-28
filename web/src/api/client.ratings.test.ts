import { it, expect, vi, afterEach } from 'vitest'
import { getRatings, submitRating } from './client'

afterEach(() => vi.restoreAllMocks())

it('getRatings GETs the endpoint (no auth header when no token)', async () => {
  const f = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
    new Response(JSON.stringify({ average: 4, count: 1, items: [], mine: null, canRate: false }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
  const out = await getRatings('orders')
  expect(out.average).toBe(4)
  expect(f.mock.calls[0][0]).toBe('/api/ratings/orders')
})

it('getRatings sends the bearer token when given', async () => {
  const f = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
    new Response(JSON.stringify({ average: 0, count: 0, items: [], mine: null, canRate: true }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
  await getRatings('orders', 'jwt')
  expect((f.mock.calls[0][1] as RequestInit).headers).toMatchObject({ Authorization: 'Bearer jwt' })
})

it('submitRating PUTs stars+comment with auth', async () => {
  const f = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
    new Response(JSON.stringify({ average: 5, count: 1, items: [], mine: { stars: 5, comment: 'top', author: 'Me', createdAt: '' }, canRate: true }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
  const out = await submitRating('jwt', 'orders', { stars: 5, comment: 'top' })
  expect(out.average).toBe(5)
  const [url, init] = f.mock.calls[0]
  expect(url).toBe('/api/ratings/orders')
  expect((init as RequestInit).method).toBe('PUT')
  expect(JSON.parse((init as RequestInit).body as string)).toEqual({ stars: 5, comment: 'top' })
})
