import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { PlansPage } from './PlansPage'
import { AuthProvider } from '../../auth/AuthProvider'
import { LanguageProvider } from '../../i18n/LanguageProvider'
import * as api from '../../api/client'
import { ApiError } from '../../api/client'
import type { Plan } from '../../api/types'

const plans: Plan[] = [
  { id: 1, name: 'Free', rateLimit: 60, windowSeconds: 60, priceCents: 0, currency: 'EUR' },
  { id: 3, name: 'Gold', rateLimit: 1000, windowSeconds: 60, priceCents: 2900, currency: 'EUR' },
]

beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('lang', 'fr')
  localStorage.setItem('token', 'jwt')
  localStorage.setItem('user', JSON.stringify({ id: 1, email: 'a@b.c', name: 'Admin', role: 'admin' }))
  vi.restoreAllMocks()
  vi.spyOn(api, 'adminGetPlans').mockResolvedValue({ items: plans, total: plans.length, page: 1, pageSize: 20 })
  vi.spyOn(api, 'adminGetProducts').mockResolvedValue({ items: [], total: 0, page: 1, pageSize: 20 })
  vi.spyOn(api, 'adminGetSubscriptions').mockResolvedValue({ items: [], total: 0, page: 1, pageSize: 20 })
})

const renderPage = () => render(
  <MemoryRouter><LanguageProvider><AuthProvider><PlansPage /></AuthProvider></LanguageProvider></MemoryRouter>
)

describe('PlansPage', () => {
  it('renders plan rows with limit chip and sustained rate', async () => {
    renderPage()
    expect(await screen.findByText('Free')).toBeInTheDocument()
    expect(screen.getByText('60 req / 60s')).toBeInTheDocument()
    expect(screen.getByText(/≈ 1 req\/s soutenu/)).toBeInTheDocument()
    expect(screen.getByText(/≈ 17 req\/s soutenu/)).toBeInTheDocument()
  })

  it('creates a plan and shows the live preview', async () => {
    const create = vi.spyOn(api, 'adminCreatePlan').mockResolvedValue(plans[0])
    renderPage()
    await screen.findByText('Free')
    await userEvent.click(screen.getByRole('button', { name: /Nouveau plan/ }))
    await userEvent.type(screen.getByLabelText('Nom du plan'), 'Platinum')
    expect(screen.getByText('≈ 1.7 req/s soutenu')).toBeInTheDocument() // 100/60 default
    await userEvent.click(screen.getByRole('button', { name: /Créer le plan/ }))
    await waitFor(() => expect(create).toHaveBeenCalled())
    expect(create.mock.calls[0][1]).toMatchObject({ name: 'Platinum', rateLimit: 100, windowSeconds: 60 })
  })

  it('edit opens prefilled and saves with PUT', async () => {
    const update = vi.spyOn(api, 'adminUpdatePlan').mockResolvedValue(plans[0])
    renderPage()
    await screen.findByText('Free')
    await userEvent.click(screen.getAllByRole('button', { name: 'Modifier' })[0])
    expect(screen.getByLabelText('Nom du plan')).toHaveValue('Free')
    await userEvent.clear(screen.getByLabelText(/Limite/))
    await userEvent.type(screen.getByLabelText(/Limite/), '120')
    await userEvent.click(screen.getByRole('button', { name: /Enregistrer/ }))
    await waitFor(() => expect(update).toHaveBeenCalled())
    expect(update.mock.calls[0][2]).toMatchObject({ name: 'Free', rateLimit: 120 })
  })

  it('a 409 delete shows the plan-in-use toast', async () => {
    vi.spyOn(api, 'adminDeletePlan').mockRejectedValue(new ApiError('in use', 409))
    renderPage()
    await screen.findByText('Free')
    await userEvent.click(screen.getAllByRole('button', { name: 'Supprimer' })[0])
    const dialog = await screen.findByRole('dialog')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Supprimer' }))
    expect(await screen.findByText('Suppression impossible : des abonnements utilisent ce plan.')).toBeInTheDocument()
    expect(screen.getByText('Free')).toBeInTheDocument()
  })
})
