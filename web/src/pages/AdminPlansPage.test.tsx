import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { AdminPlansPage } from './AdminPlansPage'
import { ThemeProvider } from '../theme/ThemeProvider'
import { AuthProvider } from '../auth/AuthProvider'
import * as api from '../api/client'
import type { Plan } from '../api/types'

const plans: Plan[] = [{ id: 1, name: 'Silver', rateLimit: 300, windowSeconds: 60 }]

beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('token', 'tok')
  localStorage.setItem('user', JSON.stringify({ id: 1, email: 'admin@portal.local', name: '', role: 'admin' }))
  vi.restoreAllMocks()
})

function renderPage() {
  return render(<MemoryRouter><ThemeProvider><AuthProvider><AdminPlansPage /></AuthProvider></ThemeProvider></MemoryRouter>)
}

describe('AdminPlansPage', () => {
  it('lists plans', async () => {
    vi.spyOn(api, 'adminGetPlans').mockResolvedValue(plans)
    renderPage()
    await waitFor(() => expect(screen.getByText('Silver')).toBeInTheDocument())
  })

  it('creates a plan from the form', async () => {
    vi.spyOn(api, 'adminGetPlans').mockResolvedValue([])
    const create = vi.spyOn(api, 'adminCreatePlan').mockResolvedValue({ id: 5, name: 'Gold', rateLimit: 1000, windowSeconds: 60 })
    renderPage()
    await waitFor(() => expect(screen.getByLabelText('Nom du plan')).toBeInTheDocument())
    await userEvent.type(screen.getByLabelText('Nom du plan'), 'Gold')
    await userEvent.clear(screen.getByLabelText('Limite (requêtes)')); await userEvent.type(screen.getByLabelText('Limite (requêtes)'), '1000')
    await userEvent.clear(screen.getByLabelText('Fenêtre (secondes)')); await userEvent.type(screen.getByLabelText('Fenêtre (secondes)'), '60')
    await userEvent.click(screen.getByRole('button', { name: 'Créer le plan' }))
    await waitFor(() => expect(create).toHaveBeenCalled())
    expect(create.mock.calls[0][1]).toMatchObject({ name: 'Gold', rateLimit: 1000, windowSeconds: 60 })
  })

  it('shows the 409 message when a delete is blocked', async () => {
    vi.spyOn(api, 'adminGetPlans').mockResolvedValue(plans)
    vi.spyOn(api, 'adminDeletePlan').mockRejectedValue(new Error('plan is referenced by subscriptions'))
    renderPage()
    await waitFor(() => expect(screen.getByText('Silver')).toBeInTheDocument())
    await userEvent.click(screen.getByRole('button', { name: 'Supprimer Silver' }))
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent(/referenced by subscriptions/i))
  })
})
