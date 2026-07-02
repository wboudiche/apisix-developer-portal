# Teams / Organizations — Plan B (Frontend) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Surface teams in the React UI — a Teams area to create teams and manage members, plus a team selector when creating an app and a team label on the app list — against the already-shipped teams backend.

**Architecture:** Add `Team`/`TeamMember` types + team API client functions; a new `TeamsPage` (list + create + per-team member management, owner-gated controls) reached from a nav link; and extend the applications index create-form with a team selector and each app card with its team label. React 19 + TS + Vite + vitest, in `web/`.

**Tech Stack:** React 19, TypeScript, react-router-dom, vitest + @testing-library/react. French copy, existing Atlas CSS tokens/classes.

## Global Constraints

- All work under `web/`. Run `pnpm exec vitest run --exclude 'e2e/**'` + `pnpm exec tsc --noEmit` for verification.
- Backend contract (already shipped): `GET /api/teams` → `Team[]`; `POST /api/teams {name}` → `Team` (201); `GET /api/teams/{id}/members` → `TeamMember[]`; `POST /api/teams/{id}/members {email}` → 204 (404 unknown email, 409 dup/personal); `DELETE /api/teams/{id}/members/{userId}` → 204 (409 last-owner/personal); `PATCH /api/teams/{id} {name}` → 204; `DELETE /api/teams/{id}` → 204 (409 team-has-apps). `POST /api/applications` accepts optional `teamId` (defaults to the caller's personal team). `GET /api/applications` items now carry `teamId` + `teamName`.
- Roles are `'owner' | 'member'`. Management controls (add/remove member, rename, delete) show only to a team **owner**; personal teams cannot be renamed/deleted or have members added/removed.
- New fields on existing types are **optional** (`?`) to avoid fixture churn (the established pattern for sandbox/oauth fields).
- Client helpers to reuse (do NOT reinvent): `authHeaders(token)`, `parse<T>(res, url)`, `sendAuthed(method, url, token, body?)` (body optional). Pages read auth via `const { token, user } = useAuth()` from `web/src/auth/AuthProvider`.
- `ApiError` (thrown by `parse`/`sendAuthed`) carries `.message` (backend error string) + `.status`.

---

## Task 1: Types + team API client functions

**Files:**
- Modify: `web/src/api/types.ts`, `web/src/api/client.ts`
- Test: `web/src/api/client.teams.test.ts` (create)

**Interfaces:**
- Produces (types): `Team { id:number; name:string; personal:boolean; role:'owner'|'member'; memberCount:number }`, `TeamMember { userId:number; email:string; name:string; role:'owner'|'member' }`; `Application` gains `teamId?:number` + `teamName?:string`.
- Produces (client): `getTeams(token)`, `createTeam(token,name)`, `getTeamMembers(token,teamId)`, `addTeamMember(token,teamId,email)`, `removeTeamMember(token,teamId,userId)`, `renameTeam(token,teamId,name)`, `deleteTeam(token,teamId)`; `createApplication(token,name,description,teamId?)`.

- [ ] **Step 1: Write the failing test**

Create `web/src/api/client.teams.test.ts` (mirror `client.oauth.test.ts` — a `vi.stubGlobal('fetch', ...)` mock returning `Response`s):
```ts
import { describe, it, expect, vi, afterEach } from 'vitest'
import { getTeams, createTeam, addTeamMember, removeTeamMember, createApplication } from './client'

function mockFetch(body: unknown, status = 200) {
  return vi.fn(async () => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } }))
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
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && pnpm exec vitest run src/api/client.teams.test.ts`
Expected: FAIL — `getTeams`/`createTeam`/… not exported.

- [ ] **Step 3: Add the types**

In `web/src/api/types.ts`:
```ts
export interface Team {
  id: number
  name: string
  personal: boolean
  role: 'owner' | 'member'
  memberCount: number
}

export interface TeamMember {
  userId: number
  email: string
  name: string
  role: 'owner' | 'member'
}
```
And add to the existing `Application` interface (keep them optional):
```ts
  teamId?: number
  teamName?: string
```

- [ ] **Step 4: Add the client functions**

In `web/src/api/client.ts`, add `Team, TeamMember` to the import from `./types`, then add:
```ts
export async function getTeams(token: string): Promise<Team[]> {
  const url = '/api/teams'
  return parse<Team[]>(await fetch(url, { headers: authHeaders(token) }), url)
}

export async function createTeam(token: string, name: string): Promise<Team> {
  const url = '/api/teams'
  return parse<Team>(await fetch(url, { method: 'POST', headers: authHeaders(token), body: JSON.stringify({ name }) }), url)
}

export async function getTeamMembers(token: string, teamId: number): Promise<TeamMember[]> {
  const url = `/api/teams/${teamId}/members`
  return parse<TeamMember[]>(await fetch(url, { headers: authHeaders(token) }), url)
}

export async function addTeamMember(token: string, teamId: number, email: string): Promise<void> {
  return sendAuthed('POST', `/api/teams/${teamId}/members`, token, { email })
}

export async function removeTeamMember(token: string, teamId: number, userId: number): Promise<void> {
  return sendAuthed('DELETE', `/api/teams/${teamId}/members/${userId}`, token)
}

export async function renameTeam(token: string, teamId: number, name: string): Promise<void> {
  return sendAuthed('PATCH', `/api/teams/${teamId}`, token, { name })
}

export async function deleteTeam(token: string, teamId: number): Promise<void> {
  return sendAuthed('DELETE', `/api/teams/${teamId}`, token)
}
```
And change `createApplication` to accept an optional `teamId` (only include it in the body when provided, so existing callers/fixtures are unaffected):
```ts
export async function createApplication(token: string, name: string, description: string, teamId?: number): Promise<Application> {
  const body: { name: string; description: string; teamId?: number } = { name, description }
  if (teamId != null) body.teamId = teamId
  return parse<Application>(await fetch('/api/applications', {
    method: 'POST', headers: authHeaders(token), body: JSON.stringify(body),
  }), '/api/applications')
}
```

- [ ] **Step 5: Run to verify it passes**

Run: `cd web && pnpm exec vitest run src/api/client.teams.test.ts && pnpm exec tsc --noEmit`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src/api/types.ts web/src/api/client.ts web/src/api/client.teams.test.ts
git commit -m "feat(web): team types + API client functions"
```

---

## Task 2: TeamsPage (list, create, member management) + route + nav link

**Files:**
- Create: `web/src/pages/teams/TeamsPage.tsx`, `web/src/pages/teams/TeamsPage.test.tsx`
- Modify: `web/src/App.tsx` (route), `web/src/components/TopBar.tsx` (nav link)

**Interfaces:**
- Consumes: `getTeams`, `createTeam`, `getTeamMembers`, `addTeamMember`, `removeTeamMember`, `renameTeam`, `deleteTeam` (Task 1); `useAuth()` → `{ token, user }`.
- Produces: route `/teams` → `<TeamsPage />`; a "Équipes" nav link (shown when logged in).

- [ ] **Step 1: Write the failing test**

Create `web/src/pages/teams/TeamsPage.test.tsx` (mirror `ApprovalsPage.test.tsx`: mock the client module, render inside `MemoryRouter`+`AuthProvider` with a token in `localStorage`):
```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { AuthProvider } from '../../auth/AuthProvider'
import TeamsPage from './TeamsPage'
import * as client from '../../api/client'

beforeEach(() => {
  localStorage.setItem('token', 'jwt')
  localStorage.setItem('user', JSON.stringify({ id: 1, email: 'me@e.com', name: 'Me', role: 'developer' }))
})

const renderPage = () => render(<MemoryRouter><AuthProvider><TeamsPage /></AuthProvider></MemoryRouter>)

describe('TeamsPage', () => {
  it('lists my teams with role and member count', async () => {
    vi.spyOn(client, 'getTeams').mockResolvedValue([
      { id: 1, name: 'Personal', personal: true, role: 'owner', memberCount: 1 },
      { id: 2, name: 'Acme', personal: false, role: 'owner', memberCount: 3 },
    ])
    renderPage()
    expect(await screen.findByText('Acme')).toBeInTheDocument()
    expect(screen.getByText('Personal')).toBeInTheDocument()
  })

  it('creates a team', async () => {
    vi.spyOn(client, 'getTeams').mockResolvedValue([])
    const create = vi.spyOn(client, 'createTeam').mockResolvedValue({ id: 5, name: 'New', personal: false, role: 'owner', memberCount: 1 })
    renderPage()
    await waitFor(() => expect(client.getTeams).toHaveBeenCalled())
    fireEvent.change(screen.getByPlaceholderText(/nom de l'équipe/i), { target: { value: 'New' } })
    fireEvent.click(screen.getByRole('button', { name: /créer/i }))
    await waitFor(() => expect(create).toHaveBeenCalledWith('jwt', 'New'))
  })

  it('an owner can add a member by email on a non-personal team', async () => {
    vi.spyOn(client, 'getTeams').mockResolvedValue([{ id: 2, name: 'Acme', personal: false, role: 'owner', memberCount: 1 }])
    vi.spyOn(client, 'getTeamMembers').mockResolvedValue([{ userId: 1, email: 'me@e.com', name: 'Me', role: 'owner' }])
    const add = vi.spyOn(client, 'addTeamMember').mockResolvedValue()
    renderPage()
    fireEvent.click(await screen.findByText('Acme'))
    const emailInput = await screen.findByPlaceholderText(/email/i)
    fireEvent.change(emailInput, { target: { value: 'bob@e.com' } })
    fireEvent.click(screen.getByRole('button', { name: /ajouter/i }))
    await waitFor(() => expect(add).toHaveBeenCalledWith('jwt', 2, 'bob@e.com'))
  })

  it('hides member-management controls for a member (non-owner)', async () => {
    vi.spyOn(client, 'getTeams').mockResolvedValue([{ id: 3, name: 'Beta', personal: false, role: 'member', memberCount: 2 }])
    vi.spyOn(client, 'getTeamMembers').mockResolvedValue([
      { userId: 9, email: 'boss@e.com', name: 'Boss', role: 'owner' },
      { userId: 1, email: 'me@e.com', name: 'Me', role: 'member' },
    ])
    renderPage()
    fireEvent.click(await screen.findByText('Beta'))
    await screen.findByText('boss@e.com')
    expect(screen.queryByPlaceholderText(/email/i)).not.toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && pnpm exec vitest run src/pages/teams/TeamsPage.test.tsx`
Expected: FAIL — module `./TeamsPage` does not exist.

- [ ] **Step 3: Implement TeamsPage**

Create `web/src/pages/teams/TeamsPage.tsx`:
```tsx
import { useEffect, useState, type FormEvent } from 'react'
import { Navigate } from 'react-router-dom'
import { useAuth } from '../../auth/AuthProvider'
import {
  getTeams, createTeam, getTeamMembers, addTeamMember, removeTeamMember, deleteTeam,
} from '../../api/client'
import type { Team, TeamMember } from '../../api/types'

export default function TeamsPage() {
  const { token, user } = useAuth()
  const [teams, setTeams] = useState<Team[] | null>(null)
  const [selected, setSelected] = useState<Team | null>(null)
  const [name, setName] = useState('')
  const [err, setErr] = useState('')

  const reload = () => {
    if (!token) return
    getTeams(token).then(setTeams).catch(() => setErr('Impossible de charger les équipes.'))
  }
  useEffect(reload, [token])

  if (!token) return <Navigate to="/login" replace />

  const onCreate = async (e: FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return
    try {
      await createTeam(token, name.trim())
      setName('')
      reload()
    } catch (x) { setErr((x as Error).message) }
  }

  return (
    <div className="teams-page">
      <h1>Équipes</h1>
      {err && <p className="err">{err}</p>}
      <form onSubmit={onCreate} className="team-create">
        <input placeholder="Nom de l'équipe" value={name} onChange={e => setName(e.target.value)} />
        <button className="btn primary" type="submit">Créer</button>
      </form>
      <ul className="team-list">
        {teams?.map(t => (
          <li key={t.id}>
            <button className="team-row" onClick={() => setSelected(t)}>
              <b>{t.name}</b>
              {t.personal && <span className="pill">Personnelle</span>}
              <span className="team-role">{t.role === 'owner' ? 'Propriétaire' : 'Membre'}</span>
              <span className="team-count">{t.memberCount} membre{t.memberCount > 1 ? 's' : ''}</span>
            </button>
          </li>
        ))}
      </ul>
      {selected && (
        <TeamDetail
          key={selected.id}
          team={selected}
          token={token}
          meId={user?.id ?? 0}
          onChanged={reload}
        />
      )}
    </div>
  )
}

function TeamDetail({ team, token, meId, onChanged }: { team: Team; token: string; meId: number; onChanged: () => void }) {
  const [members, setMembers] = useState<TeamMember[] | null>(null)
  const [email, setEmail] = useState('')
  const [err, setErr] = useState('')
  const canManage = team.role === 'owner' && !team.personal

  const reload = () => { getTeamMembers(token, team.id).then(setMembers).catch(() => setErr('Impossible de charger les membres.')) }
  useEffect(reload, [token, team.id])

  const onAdd = async (e: FormEvent) => {
    e.preventDefault()
    if (!email.trim()) return
    setErr('')
    try {
      await addTeamMember(token, team.id, email.trim())
      setEmail('')
      reload(); onChanged()
    } catch (x) { setErr((x as Error).message) }
  }
  const onRemove = async (userId: number) => {
    setErr('')
    try { await removeTeamMember(token, team.id, userId); reload(); onChanged() }
    catch (x) { setErr((x as Error).message) }
  }
  const onDelete = async () => {
    setErr('')
    try { await deleteTeam(token, team.id); onChanged() }
    catch (x) { setErr((x as Error).message) }
  }

  return (
    <div className="team-detail">
      <h2>{team.name}</h2>
      {err && <p className="err">{err}</p>}
      <ul className="member-list">
        {members?.map(m => (
          <li key={m.userId}>
            <span>{m.name} · <span className="mono">{m.email}</span></span>
            <span className="team-role">{m.role === 'owner' ? 'Propriétaire' : 'Membre'}</span>
            {canManage && m.userId !== meId && m.role !== 'owner' && (
              <button className="btn ghost" onClick={() => onRemove(m.userId)}>Retirer</button>
            )}
          </li>
        ))}
      </ul>
      {canManage && (
        <>
          <form onSubmit={onAdd} className="member-add">
            <input placeholder="Email d'un utilisateur" value={email} onChange={e => setEmail(e.target.value)} />
            <button className="btn" type="submit">Ajouter</button>
          </form>
          <button className="btn danger" onClick={onDelete}>Supprimer l'équipe</button>
        </>
      )}
    </div>
  )
}
```
(YAGNI: rename is deferred to keep this focused — add/remove/delete + create cover the spec's management surface. `renameTeam` client fn stays available for a later pass.)

- [ ] **Step 4: Add the route + nav link**

In `web/src/App.tsx`, add the import `import TeamsPage from './pages/teams/TeamsPage'` and a route (near `/applications`):
```tsx
      <Route path="/teams" element={<TeamsPage />} />
```
In `web/src/components/TopBar.tsx`, add an "Équipes" link right after the Applications link (same `user &&` gating + `tab(...)` active pattern):
```tsx
        {user && <Link className={tab(pathname.startsWith('/teams'))} to="/teams"><IconDoc />Équipes</Link>}
```
(Reuse an existing icon such as `IconDoc`; do not invent a new icon component.)

- [ ] **Step 5: Run to verify it passes**

Run: `cd web && pnpm exec vitest run src/pages/teams/TeamsPage.test.tsx src/components/TopBar.test.tsx && pnpm exec tsc --noEmit`
Expected: PASS. (If `TopBar.test.tsx` asserts an exact nav-link count, update it to include the new link.)

- [ ] **Step 6: Commit**

```bash
git add web/src/pages/teams/ web/src/App.tsx web/src/components/TopBar.tsx
git commit -m "feat(web): Teams page (list, create, member management) + nav"
```

---

## Task 3: App-create team selector + team label on the app list

**Files:**
- Modify: `web/src/pages/application/ApplicationsIndex.tsx`
- Test: `web/src/pages/application/ApplicationsIndex.test.tsx` (create if absent, else extend)

**Interfaces:**
- Consumes: `getTeams`, `createApplication(token,name,description,teamId?)` (Task 1); the `Application.teamName` field.
- Produces: the create form includes a team `<select>` (default = the caller's personal team); each app row shows its `teamName`.

- [ ] **Step 1: Write the failing test**

Create/extend `web/src/pages/application/ApplicationsIndex.test.tsx`:
```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { AuthProvider } from '../../auth/AuthProvider'
import ApplicationsIndex from './ApplicationsIndex'
import * as client from '../../api/client'

beforeEach(() => {
  localStorage.setItem('token', 'jwt')
  localStorage.setItem('user', JSON.stringify({ id: 1, email: 'me@e.com', name: 'Me', role: 'developer' }))
})
const renderPage = () => render(<MemoryRouter><AuthProvider><ApplicationsIndex /></AuthProvider></MemoryRouter>)

describe('ApplicationsIndex teams', () => {
  it('shows each app’s team label', async () => {
    vi.spyOn(client, 'getApplications').mockResolvedValue({ items: [
      { id: 1, ownerId: 1, name: 'Shared', description: '', createdAt: '2026-01-01T00:00:00Z', teamId: 2, teamName: 'Acme' },
    ], total: 1, page: 1, pageSize: 20 } as never)
    vi.spyOn(client, 'getTeams').mockResolvedValue([{ id: 9, name: 'Personal', personal: true, role: 'owner', memberCount: 1 }])
    renderPage()
    expect(await screen.findByText('Shared')).toBeInTheDocument()
    expect(screen.getByText('Acme')).toBeInTheDocument()
  })

  it('create form offers a team selector and passes the chosen teamId', async () => {
    vi.spyOn(client, 'getApplications').mockResolvedValue({ items: [], total: 0, page: 1, pageSize: 20 } as never)
    vi.spyOn(client, 'getTeams').mockResolvedValue([
      { id: 9, name: 'Personal', personal: true, role: 'owner', memberCount: 1 },
      { id: 2, name: 'Acme', personal: false, role: 'owner', memberCount: 2 },
    ])
    const create = vi.spyOn(client, 'createApplication').mockResolvedValue({ id: 7, ownerId: 1, name: 'X', description: '', createdAt: '', teamId: 2, teamName: 'Acme' })
    renderPage()
    await waitFor(() => expect(client.getTeams).toHaveBeenCalled())
    fireEvent.click(screen.getByRole('button', { name: /nouvelle application/i }))
    fireEvent.change(screen.getByPlaceholderText(/nom/i), { target: { value: 'X' } })
    fireEvent.change(screen.getByLabelText(/équipe/i), { target: { value: '2' } })
    fireEvent.click(screen.getByRole('button', { name: /^créer$/i }))
    await waitFor(() => expect(create).toHaveBeenCalledWith('jwt', 'X', '', 2))
  })
})
```
**NOTE for the implementer:** read the current `ApplicationsIndex.tsx` first and match its real markup (the create-form button text is "+ Nouvelle application"; the name input; the submit button). Adjust the test's queries to the actual labels if they differ, but keep the two behaviors asserted: (1) an app row renders its `teamName`; (2) the create form has a team `<select>` (labelled "Équipe") defaulting to the personal team and `createApplication` receives the selected `teamId`.

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && pnpm exec vitest run src/pages/application/ApplicationsIndex.test.tsx`
Expected: FAIL — no team select / no team label.

- [ ] **Step 3: Implement the selector + label**

In `web/src/pages/application/ApplicationsIndex.tsx`:
- Import `getTeams` and the `Team` type; add `createApplication`'s new arg.
- Add state `const [teams, setTeams] = useState<Team[]>([])` and `const [teamId, setTeamId] = useState<number | ''>('')`; in the load effect, also `getTeams(token).then(ts => { setTeams(ts); const personal = ts.find(t => t.personal); if (personal) setTeamId(personal.id) })`.
- In the create form, add a labelled select before/after the name input:
```tsx
        <label htmlFor="app-team">Équipe</label>
        <select id="app-team" value={teamId} onChange={e => setTeamId(Number(e.target.value))}>
          {teams.map(t => <option key={t.id} value={t.id}>{t.name}{t.personal ? ' (personnelle)' : ''}</option>)}
        </select>
```
- In `onCreate`, pass the team: `await createApplication(token, name.trim(), '', typeof teamId === 'number' ? teamId : undefined)`.
- In each app row, render the team label (near the created-date line), guarded so pre-team fixtures don't break:
```tsx
                      {a.teamName && <span className="pill team">{a.teamName}</span>}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd web && pnpm exec vitest run src/pages/application/ApplicationsIndex.test.tsx && pnpm exec tsc --noEmit`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/application/ApplicationsIndex.tsx web/src/pages/application/ApplicationsIndex.test.tsx
git commit -m "feat(web): team selector on app create + team label on app list"
```

---

## Task 4: Live verification (browser)

**Files:** none (verification only).

- [ ] **Step 1: Full frontend suite + typecheck + build**

Run: `cd web && pnpm exec vitest run --exclude 'e2e/**' && pnpm exec tsc --noEmit && pnpm build`
Expected: all green.

- [ ] **Step 2: Browser walkthrough**

Bring the stack up (`docker compose up -d postgres apisix`), run the portal (`:8090`) and vite (`:5173`, `PORTAL_PROXY=http://localhost:8090`). In the browser (register two users A and B via the UI or seed tokens):
1. As A, open **Équipes** (nav link) → the personal team shows (role Propriétaire).
2. Create a team "Acme" → it appears in the list.
3. Open Acme → add B by email (Ajouter) → B appears as Membre; try a bogus email → the inline error shows the backend message; the Supprimer/Ajouter/Retirer controls are visible (A is owner).
4. Go to **Applications** → **+ Nouvelle application** → the **Équipe** selector lists Personal + Acme (defaults to personal); create "Shared" under Acme → the card shows the **Acme** team label.
5. Log in as B → **Équipes** shows Acme (role Membre, no add/remove/delete controls); **Applications** lists "Shared" with the Acme label; B can open it.
6. As A, remove B from Acme → B no longer sees "Shared".
**Take a screenshot of the Teams page and the app-create form; look at them.**

- [ ] **Step 3: No commit** (verification only; note results in the progress ledger).

---

## Self-Review notes

- **Spec coverage (Plan B section):** Teams area — list/create/member-management (T2) ✅; app-create team selector defaulting to personal (T3) ✅; app-list team labels (T3) ✅; types + client fns incl. `createApplication` teamId (T1) ✅; nav link (T2) ✅. Rename is intentionally deferred in T2 (client fn present, control not wired) — a small follow-up, noted so it isn't mistaken for a gap. The spec lists rename as owner-only management; if the reviewer deems it required, wiring a rename control is a T2 addition.
- **Placeholder scan:** none — every step has concrete code or exact commands.
- **Type consistency:** `Team`/`TeamMember` field names (`role`, `personal`, `memberCount`, `userId`) are identical across types (T1), client (T1), and both pages/tests (T2/T3). `createApplication(token, name, description, teamId?)` matches its call in T3 and the client test in T1. `useAuth()` yields `{ token, user }` with `user.id` (used for the self-exclusion in remove). The app-list guard uses `a.teamName` (optional) so pre-team fixtures render unchanged.
- **Implementer note:** verify the real `useAuth()` shape (`user.id` presence) and `ApplicationsIndex.tsx`'s actual create-form markup before adapting the test queries; the code blocks use the conventional labels but the components are the source of truth. If `TopBar.test.tsx` asserts an exact set/count of nav links, update it for the new "Équipes" link.
```
