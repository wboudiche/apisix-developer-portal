import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { InvoicesPage } from './InvoicesPage'
import { AuthProvider } from '../../auth/AuthProvider'
import { LanguageProvider } from '../../i18n/LanguageProvider'
import * as api from '../../api/client'
import { ApiError } from '../../api/client'
import type { Invoice } from '../../api/types'

const invoices: Invoice[] = [
  { id: 21, teamId: 5, subscriptionId: 100, planName: 'Gold', priceCents: 4900, currency: 'EUR', status: 'pending', createdAt: '2026-06-06T10:00:00Z', paidAt: null },
  { id: 22, teamId: 7, subscriptionId: 101, planName: 'Silver', priceCents: 1900, currency: 'EUR', status: 'paid', createdAt: '2026-06-01T10:00:00Z', paidAt: '2026-06-02T10:00:00Z' },
]

beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('lang', 'fr')
  localStorage.setItem('token', 'jwt')
  localStorage.setItem('user', JSON.stringify({ id: 1, email: 'a@b.c', name: 'Admin', role: 'admin' }))
  vi.restoreAllMocks()
  vi.spyOn(api, 'adminGetInvoices').mockResolvedValue(invoices)
})

const renderPage = () => render(
  <MemoryRouter><LanguageProvider><AuthProvider><InvoicesPage /></AuthProvider></LanguageProvider></MemoryRouter>
)

describe('InvoicesPage', () => {
  it('lists invoices with plan name, formatted amount and status label', async () => {
    renderPage()
    expect(await screen.findByText('Gold')).toBeInTheDocument()
    expect(screen.getByText('49,00 €')).toBeInTheDocument()
    expect(screen.getByText('Silver')).toBeInTheDocument()
    const table = screen.getByRole('table')
    expect(within(table).getByText('En attente')).toBeInTheDocument()
    expect(within(table).getByText('Payée')).toBeInTheDocument()
  })

  it('clicking a status filter re-calls adminGetInvoices with that status', async () => {
    renderPage()
    await screen.findByText('Gold')
    await userEvent.click(screen.getByRole('button', { name: 'Payée' }))
    await waitFor(() => expect(api.adminGetInvoices).toHaveBeenLastCalledWith('jwt', 'paid'))
  })

  it('clicking Payer on a pending row pays and reloads', async () => {
    const pay = vi.spyOn(api, 'adminPayInvoice').mockResolvedValue(undefined)
    renderPage()
    await screen.findByText('Gold')
    await userEvent.click(screen.getAllByRole('button', { name: 'Payer' })[0])
    await waitFor(() => expect(pay).toHaveBeenCalledWith('jwt', 21))
    await waitFor(() => expect(api.adminGetInvoices).toHaveBeenCalledTimes(2))
  })

  it('surfaces a rejected pay call via role=alert', async () => {
    vi.spyOn(api, 'adminPayInvoice').mockRejectedValue(new ApiError('invoice already paid', 409))
    renderPage()
    await screen.findByText('Gold')
    await userEvent.click(screen.getAllByRole('button', { name: 'Payer' })[0])
    expect(await screen.findByRole('alert')).toHaveTextContent('invoice already paid')
  })

  it('does not show Payer/Annuler on a paid row', async () => {
    vi.spyOn(api, 'adminGetInvoices').mockResolvedValue([invoices[1]])
    renderPage()
    await screen.findByText('Silver')
    expect(screen.queryByRole('button', { name: 'Payer' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Annuler' })).not.toBeInTheDocument()
  })
})
