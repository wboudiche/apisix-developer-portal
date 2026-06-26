import { it, expect, vi, afterEach } from 'vitest'
import { getProduct, getProductSpec } from './client'

afterEach(() => vi.restoreAllMocks())

it('getProduct fetches a single product by slug', async () => {
  const product = { id: 1, name: 'Orders', slug: 'orders', category: 'Data', version: '1.0.0', contextPath: '/orders', description: '', tags: [], icon: '', rating: 4 }
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(
    new Response(JSON.stringify(product), { status: 200, headers: { 'Content-Type': 'application/json' } }),
  )
  const out = await getProduct('orders')
  expect(out.name).toBe('Orders')
})

it('getProductSpec returns the raw spec text', async () => {
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(
    new Response('{"openapi":"3.0.0"}', { status: 200, headers: { 'Content-Type': 'application/json' } }),
  )
  expect(await getProductSpec('orders')).toBe('{"openapi":"3.0.0"}')
})

it('getProductSpec returns null on 404 (no docs)', async () => {
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('{"error":"spec not found"}', { status: 404 }))
  expect(await getProductSpec('orders')).toBeNull()
})
