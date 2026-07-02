import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { AuthProvider } from '../../auth/AuthProvider'
import { ApplicationsIndex } from './ApplicationsIndex'
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

  it('create form offers a team selector defaulting to the personal team and passes the chosen teamId', async () => {
    // Empty app list so the create form (empty-state variant) is rendered immediately,
    // without needing to click the "+ Nouvelle application" toggle.
    vi.spyOn(client, 'getApplications').mockResolvedValue({ items: [], total: 0, page: 1, pageSize: 20 } as never)
    vi.spyOn(client, 'getTeams').mockResolvedValue([
      { id: 9, name: 'Personal', personal: true, role: 'owner', memberCount: 1 },
      { id: 2, name: 'Acme', personal: false, role: 'owner', memberCount: 2 },
    ])
    const create = vi.spyOn(client, 'createApplication').mockResolvedValue({ id: 7, ownerId: 1, name: 'X', description: '', createdAt: '', teamId: 2, teamName: 'Acme' })
    renderPage()
    await waitFor(() => expect(client.getTeams).toHaveBeenCalled())

    const select = await screen.findByLabelText(/équipe/i) as HTMLSelectElement
    await waitFor(() => expect(select.value).toBe('9'))

    fireEvent.change(screen.getByPlaceholderText(/nom/i), { target: { value: 'X' } })
    fireEvent.change(select, { target: { value: '2' } })
    fireEvent.click(screen.getByRole('button', { name: /^créer$/i }))
    await waitFor(() => expect(create).toHaveBeenCalledWith('jwt', 'X', '', 2))
  })
})
