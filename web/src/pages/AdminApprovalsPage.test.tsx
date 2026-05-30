import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { AdminApprovalsPage } from './AdminApprovalsPage'
import { ThemeProvider } from '../theme/ThemeProvider'
import { AuthProvider } from '../auth/AuthProvider'
import * as api from '../api/client'
import type { AdminSubscription } from '../api/types'

const pending: AdminSubscription[] = [
  { id: 7, applicationName: 'My App', ownerEmail: 'dev@x.com', productName: 'PizzaShackAPI', version: '1.0.0', planName: 'Silver', status: 'pending', createdAt: '2026-05-30T10:00:00Z' },
]

beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('token', 'tok')
  localStorage.setItem('user', JSON.stringify({ id: 1, email: 'admin@portal.local', name: '', role: 'admin' }))
  vi.restoreAllMocks()
})

function renderPage() {
  return render(<MemoryRouter><ThemeProvider><AuthProvider><AdminApprovalsPage /></AuthProvider></ThemeProvider></MemoryRouter>)
}

describe('AdminApprovalsPage', () => {
  it('lists pending subscriptions with app, product, plan and requester', async () => {
    vi.spyOn(api, 'adminGetSubscriptions').mockResolvedValue(pending)
    renderPage()
    await waitFor(() => expect(screen.getByText('PizzaShackAPI')).toBeInTheDocument())
    expect(screen.getByText('My App')).toBeInTheDocument()
    expect(screen.getByText('dev@x.com')).toBeInTheDocument()
    expect(api.adminGetSubscriptions).toHaveBeenCalledWith('tok', 'pending')
  })

  it('approves a subscription and refreshes', async () => {
    const get = vi.spyOn(api, 'adminGetSubscriptions').mockResolvedValue(pending)
    const approve = vi.spyOn(api, 'adminApproveSubscription').mockResolvedValue(undefined)
    renderPage()
    await waitFor(() => expect(screen.getByText('PizzaShackAPI')).toBeInTheDocument())
    get.mockResolvedValue([])
    await userEvent.click(screen.getByRole('button', { name: 'Approuver' }))
    await waitFor(() => expect(approve).toHaveBeenCalledWith('tok', 7))
    await waitFor(() => expect(screen.getByText(/Aucun abonnement en attente/i)).toBeInTheDocument())
  })

  it('rejects a subscription', async () => {
    vi.spyOn(api, 'adminGetSubscriptions').mockResolvedValue(pending)
    const reject = vi.spyOn(api, 'adminRejectSubscription').mockResolvedValue(undefined)
    renderPage()
    await waitFor(() => expect(screen.getByText('PizzaShackAPI')).toBeInTheDocument())
    await userEvent.click(screen.getByRole('button', { name: 'Rejeter' }))
    await waitFor(() => expect(reject).toHaveBeenCalledWith('tok', 7))
  })
})
