import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { ProductsPage } from './ProductsPage'
import { AuthProvider } from '../../auth/AuthProvider'
import * as api from '../../api/client'
import { ApiError } from '../../api/client'
import type { AdminProduct } from '../../api/types'

const products: AdminProduct[] = [
  { id: 1, name: 'CurrencyConverterAPI', slug: 'currency-converter', category: 'Finance', version: '1.0.0', contextPath: '/currencyconv', description: '', tags: [], icon: '', upstreamUrl: 'echo:8080', published: true },
  { id: 2, name: 'PizzaShackAPI', slug: 'pizzashack', category: 'Data', version: '0.9.0', contextPath: '/pizzashack', description: '', tags: [], icon: '', upstreamUrl: 'echo:8080', published: false },
]

beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('token', 'jwt')
  localStorage.setItem('user', JSON.stringify({ id: 1, email: 'a@b.c', name: 'Admin', role: 'admin' }))
  vi.restoreAllMocks()
  vi.spyOn(api, 'adminGetProducts').mockResolvedValue({ items: products, total: products.length, page: 1, pageSize: 20 })
  vi.spyOn(api, 'adminGetPlans').mockResolvedValue({ items: [], total: 0, page: 1, pageSize: 20 })
  vi.spyOn(api, 'adminGetSubscriptions').mockResolvedValue({ items: [], total: 0, page: 1, pageSize: 20 })
})

function renderPage() {
  return render(
    <MemoryRouter><AuthProvider><ProductsPage /></AuthProvider></MemoryRouter>
  )
}

describe('ProductsPage', () => {
  it('renders rows with context chip, upstream and status pill', async () => {
    renderPage()
    expect(await screen.findByText('CurrencyConverterAPI')).toBeInTheDocument()
    expect(screen.getByText('/currencyconv')).toBeInTheDocument()
    expect(screen.getAllByText('echo:8080', { selector: '.up' })[0]).toBeInTheDocument()
    expect(screen.getByText('publié')).toBeInTheDocument()
    expect(screen.getByText('brouillon')).toBeInTheDocument()
    expect(screen.getByText('v1.0.0')).toBeInTheDocument()
  })

  it('filters rows client-side', async () => {
    renderPage()
    await screen.findByText('CurrencyConverterAPI')
    await userEvent.type(screen.getByPlaceholderText('Filtrer les produits…'), 'pizza')
    expect(screen.queryByText('CurrencyConverterAPI')).not.toBeInTheDocument()
    expect(screen.getByText('PizzaShackAPI')).toBeInTheDocument()
  })

  it('creates a product with an auto-generated slug', async () => {
    const create = vi.spyOn(api, 'adminCreateProduct').mockResolvedValue(products[0])
    renderPage()
    await screen.findByText('CurrencyConverterAPI')
    await userEvent.click(screen.getByRole('button', { name: /Nouveau produit/ }))
    await userEvent.type(screen.getByLabelText('Nom'), 'OrdersAPI')
    expect(screen.getByLabelText('Slug')).toHaveValue('orders')
    await userEvent.click(screen.getByRole('button', { name: /Créer le produit/ }))
    await waitFor(() => expect(create).toHaveBeenCalled())
    const payload = create.mock.calls[0][1]
    expect(payload.name).toBe('OrdersAPI')
    expect(payload.slug).toBe('orders')
    expect(payload.contextPath).toBe('/orders')
    expect(payload.published).toBe(true)
  })

  it('edit opens the composer prefilled and saves with PUT', async () => {
    const update = vi.spyOn(api, 'adminUpdateProduct').mockResolvedValue(products[0])
    renderPage()
    await screen.findByText('CurrencyConverterAPI')
    await userEvent.click(screen.getAllByRole('button', { name: 'Modifier' })[0])
    expect(screen.getByLabelText('Nom')).toHaveValue('CurrencyConverterAPI')
    await userEvent.clear(screen.getByLabelText(/Upstream/))
    await userEvent.type(screen.getByLabelText(/Upstream/), 'fx:9000')
    await userEvent.click(screen.getByRole('button', { name: /Enregistrer/ }))
    await waitFor(() => expect(update).toHaveBeenCalled())
    expect(update.mock.calls[0][1]).toBe(1)
    expect(update.mock.calls[0][2].upstreamUrl).toBe('fx:9000')
  })

  it('the eye toggle flips published', async () => {
    const update = vi.spyOn(api, 'adminUpdateProduct').mockResolvedValue(products[0])
    renderPage()
    await screen.findByText('CurrencyConverterAPI')
    await userEvent.click(screen.getAllByRole('button', { name: 'Dépublier' })[0])
    await waitFor(() => expect(update).toHaveBeenCalled())
    expect(update.mock.calls[0][2].published).toBe(false)
  })

  it('delete goes through the confirm modal', async () => {
    const del = vi.spyOn(api, 'adminDeleteProduct').mockResolvedValue(undefined)
    renderPage()
    await screen.findByText('CurrencyConverterAPI')
    await userEvent.click(screen.getAllByRole('button', { name: 'Supprimer' })[0])
    const dialog = await screen.findByRole('dialog')
    expect(dialog).toHaveTextContent('/currencyconv')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Supprimer' }))
    await waitFor(() => expect(del).toHaveBeenCalledWith('jwt', 1))
  })

  it('a 409 delete shows the active-subscriptions toast and keeps the row', async () => {
    vi.spyOn(api, 'adminDeleteProduct').mockRejectedValue(new ApiError('conflict', 409))
    renderPage()
    await screen.findByText('CurrencyConverterAPI')
    await userEvent.click(screen.getAllByRole('button', { name: 'Supprimer' })[0])
    const dialog = await screen.findByRole('dialog')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Supprimer' }))
    expect(await screen.findByText('Suppression impossible : abonnements actifs.')).toBeInTheDocument()
    expect(screen.getByText('CurrencyConverterAPI')).toBeInTheDocument()
  })

  it('import opens the Composer pre-filled from the returned draft', async () => {
    vi.spyOn(api, 'adminImportProduct').mockResolvedValue({
      name: 'Imported API', slug: 'imported', category: 'Finance', version: '2.5.0',
      contextPath: '/v2', description: 'desc', tags: ['Finance'], icon: '', upstreamUrl: 'api.example.com:443', published: false,
    })
    renderPage()
    await screen.findByText('CurrencyConverterAPI')
    await userEvent.click(screen.getByRole('button', { name: /Importer une API/i }))
    await userEvent.click(screen.getByRole('tab', { name: /URL/i }))
    await userEvent.type(screen.getByPlaceholderText(/https/i), 'https://api.example.com/openapi.json')
    await userEvent.click(screen.getByRole('button', { name: /^Importer$/i }))

    // Composer is now open, pre-filled as a create
    expect(await screen.findByText('Créer un produit')).toBeInTheDocument()
    expect(screen.getByLabelText('Nom')).toHaveValue('Imported API')
    expect(screen.getByLabelText('Version')).toHaveValue('2.5.0')
    expect(screen.getByLabelText('Context path')).toHaveValue('/v2')
  })
})
