import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { ApprovalsPage } from './ApprovalsPage'
import { AuthProvider } from '../../auth/AuthProvider'
import * as api from '../../api/client'
import type { AdminSubscription } from '../../api/types'

const subs: AdminSubscription[] = [
  { id: 11, applicationName: 'MobileCheckout', ownerEmail: 'lina@acme.io', productName: 'CurrencyConverterAPI', version: '1.0.0', planName: 'Silver', status: 'pending', createdAt: '2026-06-06T10:00:00Z' },
  { id: 12, applicationName: 'PartnerSync', ownerEmail: 'dev@partner.dev', productName: 'PeopleAPI', version: '1.3.2', planName: 'Free', status: 'pending', createdAt: '2026-06-04T09:00:00Z' },
]

beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('token', 'jwt')
  localStorage.setItem('user', JSON.stringify({ id: 1, email: 'a@b.c', name: 'Admin', role: 'admin' }))
  vi.restoreAllMocks()
  vi.spyOn(api, 'adminGetSubscriptions').mockResolvedValue({ items: subs, total: subs.length, page: 1, pageSize: 20 })
  vi.spyOn(api, 'adminGetProducts').mockResolvedValue({ items: [], total: 0, page: 1, pageSize: 20 })
  vi.spyOn(api, 'adminGetPlans').mockResolvedValue({ items: [], total: 0, page: 1, pageSize: 20 })
})

const renderPage = () => render(
  <MemoryRouter><AuthProvider><ApprovalsPage /></AuthProvider></MemoryRouter>
)

describe('ApprovalsPage', () => {
  it('renders pending rows: avatar initials, app → product, plan tag, requester and date', async () => {
    renderPage()
    expect(await screen.findByText('MobileCheckout')).toBeInTheDocument()
    expect(screen.getByText('MC')).toBeInTheDocument()
    expect(screen.getByText('CurrencyConverterAPI')).toBeInTheDocument()
    expect(screen.getByText('Silver')).toBeInTheDocument()
    expect(screen.getByText('lina@acme.io')).toBeInTheDocument()
    expect(screen.getByText('2026-06-06')).toBeInTheDocument()
  })

  it('approve calls the API and removes the row', async () => {
    const approve = vi.spyOn(api, 'adminApproveSubscription').mockResolvedValue(undefined)
    vi.spyOn(api, 'adminGetSubscriptions')
      .mockResolvedValueOnce({ items: subs, total: subs.length, page: 1, pageSize: 20 })            // initial load
      .mockResolvedValue({ items: [subs[1]], total: 1, page: 1, pageSize: 20 })           // after approval
    renderPage()
    await screen.findByText('MobileCheckout')
    await userEvent.click(screen.getAllByRole('button', { name: 'Approuver' })[0])
    await waitFor(() => expect(approve).toHaveBeenCalledWith('jwt', 11))
    await waitFor(() => expect(screen.queryByText('MobileCheckout')).not.toBeInTheDocument())
    expect(screen.getByText(/approuvé — consumer APISIX créé/)).toBeInTheDocument()
  })

  it('reject calls the API and removes the row', async () => {
    const reject = vi.spyOn(api, 'adminRejectSubscription').mockResolvedValue(undefined)
    vi.spyOn(api, 'adminGetSubscriptions')
      .mockResolvedValueOnce({ items: subs, total: subs.length, page: 1, pageSize: 20 })
      .mockResolvedValue({ items: [subs[0]], total: 1, page: 1, pageSize: 20 })
    renderPage()
    await screen.findByText('PartnerSync')
    await userEvent.click(screen.getAllByRole('button', { name: 'Refuser' })[1])
    await waitFor(() => expect(reject).toHaveBeenCalledWith('jwt', 12))
    await waitFor(() => expect(screen.queryByText('PartnerSync')).not.toBeInTheDocument())
  })

  it('shows the blueprint empty state when the queue is empty', async () => {
    vi.spyOn(api, 'adminGetSubscriptions').mockResolvedValue({ items: [], total: 0, page: 1, pageSize: 20 })
    renderPage()
    expect(await screen.findByText("File d'attente vide")).toBeInTheDocument()
    expect(screen.getByText('Aucun abonnement en attente de validation.')).toBeInTheDocument()
  })
})
