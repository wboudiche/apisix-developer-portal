import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { AdminProductsPage } from './AdminProductsPage'
import { ThemeProvider } from '../theme/ThemeProvider'
import { AuthProvider } from '../auth/AuthProvider'
import * as api from '../api/client'
import type { AdminProduct } from '../api/types'

const sample: AdminProduct[] = [
  { id: 1, name: 'PizzaShackAPI', slug: 'pizzashackapi', category: 'Engineering', version: '1.0.0', contextPath: '/pizzashack', description: 'demo', tags: ['pizza'], icon: '', upstreamUrl: 'echo:8080', published: true },
]

beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('token', 'tok')
  localStorage.setItem('user', JSON.stringify({ id: 1, email: 'admin@portal.local', name: '', role: 'admin' }))
  vi.restoreAllMocks()
})

function renderPage() {
  return render(<MemoryRouter><ThemeProvider><AuthProvider><AdminProductsPage /></AuthProvider></ThemeProvider></MemoryRouter>)
}

describe('AdminProductsPage', () => {
  it('lists all products including unpublished state', async () => {
    vi.spyOn(api, 'adminGetProducts').mockResolvedValue(sample)
    renderPage()
    await waitFor(() => expect(screen.getByText('PizzaShackAPI')).toBeInTheDocument())
    expect(screen.getByText('echo:8080')).toBeInTheDocument()
  })

  it('creates a product from the form', async () => {
    vi.spyOn(api, 'adminGetProducts').mockResolvedValue([])
    const create = vi.spyOn(api, 'adminCreateProduct').mockResolvedValue({ ...sample[0], id: 9 })
    renderPage()
    await waitFor(() => expect(screen.getByLabelText('Nom')).toBeInTheDocument())
    await userEvent.type(screen.getByLabelText('Nom'), 'NewAPI')
    await userEvent.type(screen.getByLabelText('Slug'), 'newapi')
    await userEvent.type(screen.getByLabelText('Catégorie'), 'Engineering')
    await userEvent.type(screen.getByLabelText('Context path'), '/new')
    await userEvent.click(screen.getByRole('button', { name: 'Créer le produit' }))
    await waitFor(() => expect(create).toHaveBeenCalled())
    expect(create.mock.calls[0][1]).toMatchObject({ name: 'NewAPI', slug: 'newapi', contextPath: '/new' })
  })

  it('shows the 409 message when a delete is blocked', async () => {
    vi.spyOn(api, 'adminGetProducts').mockResolvedValue(sample)
    vi.spyOn(api, 'adminDeleteProduct').mockRejectedValue(new Error('product has active subscriptions'))
    renderPage()
    await waitFor(() => expect(screen.getByText('PizzaShackAPI')).toBeInTheDocument())
    await userEvent.click(screen.getByRole('button', { name: 'Supprimer PizzaShackAPI' }))
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent(/active subscriptions/i))
  })
})
