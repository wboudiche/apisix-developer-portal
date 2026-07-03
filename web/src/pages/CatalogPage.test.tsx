import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { CatalogPage } from './CatalogPage'
import { ThemeProvider } from '../theme/ThemeProvider'
import { AuthProvider } from '../auth/AuthProvider'
import { LanguageProvider } from '../i18n/LanguageProvider'
import * as api from '../api/client'
import type { Product } from '../api/types'

const sample: Product[] = [
  { id: 1, name: 'PizzaShackAPI', slug: 'pizzashackapi', category: 'Engineering', version: '1.0.0', contextPath: '/pizzashack', description: 'demo', tags: ['pizza'], icon: 'pi', rating: 4.5, ratingCount: 0 },
  { id: 2, name: 'CurrencyConverterAPI', slug: 'currencyconverterapi', category: 'Finance', version: '1.0.0', contextPath: '/currencyconv', description: 'fx', tags: ['devises'], icon: 'cu', rating: 5, ratingCount: 0 },
]

function renderPage() {
  return render(
    <MemoryRouter><LanguageProvider><ThemeProvider><AuthProvider><CatalogPage /></AuthProvider></ThemeProvider></LanguageProvider></MemoryRouter>
  )
}

beforeEach(() => {
  localStorage.clear()
  // jsdom's navigator.language defaults to 'en-US', which would auto-detect to
  // English; force French so existing assertions (against French strings) hold.
  localStorage.setItem('lang', 'fr')
  vi.restoreAllMocks()
})

const envelope = { items: sample, total: sample.length, page: 1, pageSize: 20 }

describe('CatalogPage', () => {
  it('loads and renders all products from the API', async () => {
    const spy = vi.spyOn(api, 'getProducts').mockResolvedValue(envelope)
    renderPage()
    await waitFor(() => expect(screen.getAllByTestId('api-card')).toHaveLength(2))
    expect(spy).toHaveBeenCalledWith({}, expect.objectContaining({ pageSize: 100 }))
    expect(screen.getByText('PizzaShackAPI')).toBeInTheDocument()
  })

  it('re-queries the API when the user types a search', async () => {
    const spy = vi.spyOn(api, 'getProducts').mockResolvedValue(envelope)
    renderPage()
    await waitFor(() => expect(screen.getAllByTestId('api-card')).toHaveLength(2))
    await userEvent.type(screen.getByLabelText('Rechercher'), 'pizza')
    await waitFor(() => expect(spy).toHaveBeenCalledWith(expect.objectContaining({ search: 'pizza' }), expect.any(Object)))
  })

  it('shows an error message when loading fails', async () => {
    vi.spyOn(api, 'getProducts').mockRejectedValue(new Error('network down'))
    renderPage()
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent(/Impossible de charger/i))
    expect(screen.queryAllByTestId('api-card')).toHaveLength(0)
  })

  it('re-queries with the chosen sort', async () => {
    const spy = vi.spyOn(api, 'getProducts').mockResolvedValue(envelope)
    renderPage()
    await waitFor(() => expect(screen.getAllByTestId('api-card').length).toBeGreaterThan(0))
    await userEvent.selectOptions(screen.getByLabelText('Trier'), 'alpha')
    await waitFor(() => expect(spy).toHaveBeenCalledWith(expect.objectContaining({ sort: 'alpha' }), expect.any(Object)))
  })

  it('toggles to list view', async () => {
    vi.spyOn(api, 'getProducts').mockResolvedValue(envelope)
    const { container } = renderPage()
    await waitFor(() => expect(screen.getAllByTestId('api-card').length).toBeGreaterThan(0))
    await userEvent.click(screen.getByLabelText('Vue liste'))
    expect(container.querySelector('.grid.list')).not.toBeNull()
  })

  it('redirects to /login when an anonymous user clicks Subscribe', async () => {
    vi.spyOn(api, 'getProducts').mockResolvedValue(envelope)
    render(
      <MemoryRouter initialEntries={['/']}>
        <LanguageProvider><ThemeProvider><AuthProvider>
          <Routes>
            <Route path="/" element={<CatalogPage />} />
            <Route path="/login" element={<div>LOGIN PAGE</div>} />
          </Routes>
        </AuthProvider></ThemeProvider></LanguageProvider>
      </MemoryRouter>
    )
    await waitFor(() => expect(screen.getAllByTestId('api-card').length).toBeGreaterThan(0))
    await userEvent.click(screen.getAllByRole('button', { name: /s'abonner/i })[0])
    await waitFor(() => expect(screen.getByText('LOGIN PAGE')).toBeInTheDocument())
  })

  it('filters by tag when a tag is clicked', async () => {
    const spy = vi.spyOn(api, 'getProducts').mockResolvedValue(envelope)
    renderPage()
    await waitFor(() => expect(screen.getAllByTestId('api-card').length).toBeGreaterThan(0))
    await userEvent.click(screen.getByRole('button', { name: 'pizza' }))
    await waitFor(() => expect(spy).toHaveBeenCalledWith(expect.objectContaining({ tag: 'pizza' }), expect.any(Object)))
  })
})
