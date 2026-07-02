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
