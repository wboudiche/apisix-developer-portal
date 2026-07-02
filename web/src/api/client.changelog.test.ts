import { describe, it, expect, vi, afterEach } from 'vitest'
import { getChangelog, addChangelogEntry, deleteChangelogEntry } from './client'

function mockFetch(body: unknown, status = 200) {
  return vi.fn(async () => new Response(status === 204 ? null : JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } }))
}
afterEach(() => vi.unstubAllGlobals())

describe('changelog client', () => {
  it('getChangelog GETs the public endpoint', async () => {
    const f = mockFetch([{ id: 1, version: 'v1', kind: 'added', notes: 'n', date: '2026-01-01' }])
    vi.stubGlobal('fetch', f)
    const entries = await getChangelog('slug-x')
    expect(entries[0].kind).toBe('added')
    expect(f).toHaveBeenCalledWith('/api/products/slug-x/changelog', expect.anything())
  })

  it('addChangelogEntry POSTs to the admin endpoint', async () => {
    const f = mockFetch({ id: 5, version: 'v2', kind: 'fixed', notes: 'p', date: '2026-02-01' }, 201)
    vi.stubGlobal('fetch', f)
    const e = await addChangelogEntry('jwt', 7, { version: 'v2', kind: 'fixed', notes: 'p', date: '2026-02-01' })
    expect(e.id).toBe(5)
    expect(f).toHaveBeenCalledWith('/api/admin/products/7/changelog', expect.objectContaining({ method: 'POST', body: JSON.stringify({ version: 'v2', kind: 'fixed', notes: 'p', date: '2026-02-01' }) }))
  })

  it('deleteChangelogEntry DELETEs the admin endpoint', async () => {
    const f = mockFetch(null, 204)
    vi.stubGlobal('fetch', f)
    await deleteChangelogEntry('jwt', 7, 5)
    expect(f).toHaveBeenCalledWith('/api/admin/products/7/changelog/5', expect.objectContaining({ method: 'DELETE' }))
  })
})
