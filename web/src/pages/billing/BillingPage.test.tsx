import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import BillingPage from './BillingPage'
import { AuthProvider } from '../../auth/AuthProvider'
import { LanguageProvider } from '../../i18n/LanguageProvider'
import * as api from '../../api/client'
import type { Invoice } from '../../api/types'

const invoice: Invoice = {
  id: 21, teamId: 5, subscriptionId: 100, planName: 'Gold', priceCents: 4900,
  currency: 'EUR', status: 'pending', createdAt: '2026-06-06T10:00:00Z', paidAt: null,
}

beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('lang', 'fr')
  localStorage.setItem('token', 'jwt')
  localStorage.setItem('user', JSON.stringify({ id: 1, email: 'a@b.c', name: 'Dev', role: 'developer' }))
  vi.restoreAllMocks()
})

const renderPage = () => render(
  <MemoryRouter><LanguageProvider><AuthProvider><BillingPage /></AuthProvider></LanguageProvider></MemoryRouter>
)

describe('BillingPage', () => {
  it('shows the plan name, formatted amount and pending status pill', async () => {
    vi.spyOn(api, 'getBillingInvoices').mockResolvedValue([invoice])
    renderPage()
    expect(await screen.findByText('Gold')).toBeInTheDocument()
    expect(screen.getByText('49,00 €')).toBeInTheDocument()
    expect(screen.getByText('En attente')).toBeInTheDocument()
  })

  it('shows the empty state when there are no invoices', async () => {
    vi.spyOn(api, 'getBillingInvoices').mockResolvedValue([])
    renderPage()
    expect(await screen.findByText('Aucune facture.')).toBeInTheDocument()
  })
})
