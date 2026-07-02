import { describe, it, expect, vi, afterEach } from 'vitest'
import { getTeams, createTeam, addTeamMember, removeTeamMember, createApplication } from './client'

function mockFetch(body: unknown, status = 200) {
  // 204 is a null-body status per the Fetch spec — passing a stringified body
  // for it throws in Node's Response constructor, so send null instead.
  return vi.fn(async () => new Response(status === 204 ? null : JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } }))
}
afterEach(() => vi.unstubAllGlobals())

describe('teams client', () => {
  it('getTeams GETs /api/teams with auth', async () => {
    const f = mockFetch([{ id: 1, name: 'Personal', personal: true, role: 'owner', memberCount: 1 }])
    vi.stubGlobal('fetch', f)
    const teams = await getTeams('jwt')
    expect(teams[0].name).toBe('Personal')
    expect(f).toHaveBeenCalledWith('/api/teams', expect.objectContaining({ headers: expect.objectContaining({ Authorization: 'Bearer jwt' }) }))
  })

  it('createTeam POSTs the name', async () => {
    const f = mockFetch({ id: 2, name: 'Acme', personal: false, role: 'owner', memberCount: 1 }, 201)
    vi.stubGlobal('fetch', f)
    const t = await createTeam('jwt', 'Acme')
    expect(t.id).toBe(2)
    expect(f).toHaveBeenCalledWith('/api/teams', expect.objectContaining({ method: 'POST', body: JSON.stringify({ name: 'Acme' }) }))
  })

  it('addTeamMember POSTs email to the members endpoint (204, no body parse)', async () => {
    const f = mockFetch({}, 204)
    vi.stubGlobal('fetch', f)
    await addTeamMember('jwt', 2, 'x@e.com')
    expect(f).toHaveBeenCalledWith('/api/teams/2/members', expect.objectContaining({ method: 'POST', body: JSON.stringify({ email: 'x@e.com' }) }))
  })

  it('removeTeamMember DELETEs the member', async () => {
    const f = mockFetch({}, 204)
    vi.stubGlobal('fetch', f)
    await removeTeamMember('jwt', 2, 7)
    expect(f).toHaveBeenCalledWith('/api/teams/2/members/7', expect.objectContaining({ method: 'DELETE' }))
  })

  it('createApplication includes teamId when given', async () => {
    const f = mockFetch({ id: 9, ownerId: 1, name: 'A', description: '', createdAt: '', teamId: 2, teamName: 'Acme' }, 201)
    vi.stubGlobal('fetch', f)
    await createApplication('jwt', 'A', '', 2)
    expect(f).toHaveBeenCalledWith('/api/applications', expect.objectContaining({ body: JSON.stringify({ name: 'A', description: '', teamId: 2 }) }))
  })
})
