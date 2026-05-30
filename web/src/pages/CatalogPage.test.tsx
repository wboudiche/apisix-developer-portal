import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { CatalogPage } from './CatalogPage'
import { ThemeProvider } from '../theme/ThemeProvider'
import { AuthProvider } from '../auth/AuthProvider'
import * as api from '../api/client'
import type { Product } from '../api/types'

const sample: Product[] = [
  { id: 1, name: 'PizzaShackAPI', slug: 'pizzashackapi', category: 'Engineering', version: '1.0.0', contextPath: '/pizzashack', description: 'demo', tags: ['pizza'], icon: 'pi', rating: 4.5 },
  { id: 2, name: 'CurrencyConverterAPI', slug: 'currencyconverterapi', category: 'Finance', version: '1.0.0', contextPath: '/currencyconv', description: 'fx', tags: ['devises'], icon: 'cu', rating: 5 },
]

function renderPage() {
  return render(
    <MemoryRouter><ThemeProvider><AuthProvider><CatalogPage /></AuthProvider></ThemeProvider></MemoryRouter>
  )
}

beforeEach(() => { localStorage.clear(); vi.restoreAllMocks() })

describe('CatalogPage', () => {
  it('loads and renders all products from the API', async () => {
    const spy = vi.spyOn(api, 'getProducts').mockResolvedValue(sample)
    renderPage()
    await waitFor(() => expect(screen.getAllByTestId('api-card')).toHaveLength(2))
    expect(spy).toHaveBeenCalledWith({})
    expect(screen.getByText('PizzaShackAPI')).toBeInTheDocument()
  })

  it('re-queries the API when the user types a search', async () => {
    const spy = vi.spyOn(api, 'getProducts').mockResolvedValue(sample)
    renderPage()
    await waitFor(() => expect(screen.getAllByTestId('api-card')).toHaveLength(2))
    await userEvent.type(screen.getByLabelText('Rechercher'), 'pizza')
    await waitFor(() => expect(spy).toHaveBeenCalledWith(expect.objectContaining({ search: 'pizza' })))
  })

  it('shows an error message when loading fails', async () => {
    vi.spyOn(api, 'getProducts').mockRejectedValue(new Error('network down'))
    renderPage()
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent(/Impossible de charger/i))
    expect(screen.queryAllByTestId('api-card')).toHaveLength(0)
  })

  it('re-queries with the chosen sort', async () => {
    const spy = vi.spyOn(api, 'getProducts').mockResolvedValue(sample)
    renderPage()
    await waitFor(() => expect(screen.getAllByTestId('api-card').length).toBeGreaterThan(0))
    await userEvent.selectOptions(screen.getByLabelText('Trier'), 'alpha')
    await waitFor(() => expect(spy).toHaveBeenCalledWith(expect.objectContaining({ sort: 'alpha' })))
  })

  it('toggles to list view', async () => {
    vi.spyOn(api, 'getProducts').mockResolvedValue(sample)
    const { container } = renderPage()
    await waitFor(() => expect(screen.getAllByTestId('api-card').length).toBeGreaterThan(0))
    await userEvent.click(screen.getByLabelText('Vue liste'))
    expect(container.querySelector('.grid.list')).not.toBeNull()
  })

  it('redirects to /login when an anonymous user clicks Subscribe', async () => {
    vi.spyOn(api, 'getProducts').mockResolvedValue(sample)
    render(
      <MemoryRouter initialEntries={['/']}>
        <ThemeProvider><AuthProvider>
          <Routes>
            <Route path="/" element={<CatalogPage />} />
            <Route path="/login" element={<div>LOGIN PAGE</div>} />
          </Routes>
        </AuthProvider></ThemeProvider>
      </MemoryRouter>
    )
    await waitFor(() => expect(screen.getAllByTestId('api-card').length).toBeGreaterThan(0))
    await userEvent.click(screen.getAllByRole('button', { name: /s'abonner/i })[0])
    await waitFor(() => expect(screen.getByText('LOGIN PAGE')).toBeInTheDocument())
  })

  it('filters by tag when a tag is clicked', async () => {
    const spy = vi.spyOn(api, 'getProducts').mockResolvedValue(sample)
    renderPage()
    await waitFor(() => expect(screen.getAllByTestId('api-card').length).toBeGreaterThan(0))
    await userEvent.click(screen.getByRole('button', { name: 'pizza' }))
    await waitFor(() => expect(spy).toHaveBeenCalledWith(expect.objectContaining({ tag: 'pizza' })))
  })
})
