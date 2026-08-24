import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, within, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { ProductsPage } from './ProductsPage'
import { AuthProvider } from '../../auth/AuthProvider'
import { LanguageProvider } from '../../i18n/LanguageProvider'
import * as api from '../../api/client'
import { ApiError } from '../../api/client'
import type { AdminProduct } from '../../api/types'

const products: AdminProduct[] = [
  { id: 1, name: 'CurrencyConverterAPI', slug: 'currency-converter', category: 'Finance', version: '1.0.0', contextPath: '/currencyconv', description: '', tags: [], icon: '', upstreamUrl: 'echo:8080', published: true },
  { id: 2, name: 'PizzaShackAPI', slug: 'pizzashack', category: 'Data', version: '0.9.0', contextPath: '/pizzashack', description: '', tags: [], icon: '', upstreamUrl: 'echo:8080', published: false },
]

beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('lang', 'fr')
  localStorage.setItem('token', 'jwt')
  localStorage.setItem('user', JSON.stringify({ id: 1, email: 'a@b.c', name: 'Admin', role: 'admin' }))
  vi.restoreAllMocks()
  vi.spyOn(api, 'adminGetProducts').mockResolvedValue({ items: products, total: products.length, page: 1, pageSize: 20 })
  vi.spyOn(api, 'adminGetPlans').mockResolvedValue({ items: [], total: 0, page: 1, pageSize: 20 })
  vi.spyOn(api, 'adminGetSubscriptions').mockResolvedValue({ items: [], total: 0, page: 1, pageSize: 20 })
})

function renderPage() {
  return render(
    <MemoryRouter><LanguageProvider><AuthProvider><ProductsPage /></AuthProvider></LanguageProvider></MemoryRouter>
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

  // Regression tests for #10: there was previously no way to clear an
  // attached OpenAPI spec — an empty textarea on save silently kept the
  // existing one.
  it('remove specification disables the field and sends removeOpenapiSpec on save', async () => {
    const update = vi.spyOn(api, 'adminUpdateProduct').mockResolvedValue(products[0])
    renderPage()
    await screen.findByText('CurrencyConverterAPI')
    await userEvent.click(screen.getAllByRole('button', { name: 'Modifier' })[0])
    await userEvent.click(screen.getByRole('button', { name: 'Supprimer la spécification' }))
    const textarea = screen.getByLabelText(/Spécification OpenAPI/)
    expect(textarea).toHaveValue('')
    expect(textarea).toBeDisabled()
    await userEvent.click(screen.getByRole('button', { name: /Enregistrer/ }))
    await waitFor(() => expect(update).toHaveBeenCalled())
    expect(update.mock.calls[0][2].removeOpenapiSpec).toBe(true)
    expect(update.mock.calls[0][2].openapiSpec).toBe('')
  })

  it('keeping the specification undoes a pending removal', async () => {
    const update = vi.spyOn(api, 'adminUpdateProduct').mockResolvedValue(products[0])
    renderPage()
    await screen.findByText('CurrencyConverterAPI')
    await userEvent.click(screen.getAllByRole('button', { name: 'Modifier' })[0])
    await userEvent.click(screen.getByRole('button', { name: 'Supprimer la spécification' }))
    await userEvent.click(screen.getByRole('button', { name: 'Conserver la spécification' }))
    expect(screen.getByRole('button', { name: 'Supprimer la spécification' })).toBeInTheDocument()
    expect(screen.getByLabelText(/Spécification OpenAPI/)).not.toBeDisabled()
    await userEvent.click(screen.getByRole('button', { name: /Enregistrer/ }))
    await waitFor(() => expect(update).toHaveBeenCalled())
    expect(update.mock.calls[0][2].removeOpenapiSpec).toBe(false)
  })

  it('typing a new spec cancels a pending removal', async () => {
    renderPage()
    await screen.findByText('CurrencyConverterAPI')
    await userEvent.click(screen.getAllByRole('button', { name: 'Modifier' })[0])
    await userEvent.click(screen.getByRole('button', { name: 'Supprimer la spécification' }))
    // Directly setting the value bypasses the disabled textarea (matching how
    // a spec-file upload behaves), exercising setSpec's reset of the flag.
    fireEvent.change(screen.getByLabelText(/Spécification OpenAPI/), { target: { value: '{"openapi":"3.0.0"}' } })
    expect(screen.getByRole('button', { name: 'Supprimer la spécification' })).toBeInTheDocument()
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

  it('sends sandboxUpstreamUrl when creating a product', async () => {
    const create = vi.spyOn(api, 'adminCreateProduct').mockResolvedValue({} as AdminProduct)
    renderPage()
    await screen.findByText('CurrencyConverterAPI')
    await userEvent.click(screen.getByRole('button', { name: /Nouveau produit/ }))
    await userEvent.type(screen.getByLabelText('Nom'), 'OrdersAPI')
    await userEvent.type(screen.getByLabelText(/Sandbox/i), 'sandbox.example.com:443')
    await userEvent.click(screen.getByRole('button', { name: /Créer le produit/ }))
    await waitFor(() => expect(create).toHaveBeenCalled())
    expect(create.mock.calls[0][1]).toMatchObject({ sandboxUpstreamUrl: 'sandbox.example.com:443' })
  })

  it('sends authType when creating a product', async () => {
    const create = vi.spyOn(api, 'adminCreateProduct').mockResolvedValue(products[0])
    renderPage()
    await screen.findByText('CurrencyConverterAPI')
    await userEvent.click(screen.getByRole('button', { name: /Nouveau produit/ }))
    await userEvent.type(screen.getByLabelText('Nom'), 'OrdersAPI')
    await userEvent.selectOptions(screen.getByLabelText(/Méthode d.authentification/i), 'oauth2')
    await userEvent.click(screen.getByRole('button', { name: /Créer le produit/ }))
    await waitFor(() => expect(create).toHaveBeenCalled())
    expect(create.mock.calls[0][1]).toMatchObject({ authType: 'oauth2' })
  })

  it('sends lifecycleStatus + sunsetDate when creating', async () => {
    const create = vi.spyOn(api, 'adminCreateProduct').mockResolvedValue({} as AdminProduct)
    renderPage()
    await screen.findByText('CurrencyConverterAPI')
    await userEvent.click(screen.getByRole('button', { name: /Nouveau produit/ }))
    await userEvent.type(screen.getByLabelText('Nom'), 'OrdersAPI')
    await userEvent.selectOptions(screen.getByLabelText(/Statut/i), 'sunset')
    await userEvent.type(screen.getByLabelText(/Date de retrait/i), '2026-12-31')
    await userEvent.click(screen.getByRole('button', { name: /Créer le produit/ }))
    await waitFor(() => expect(create).toHaveBeenCalled())
    expect(create.mock.calls[0][1]).toMatchObject({ lifecycleStatus: 'sunset', sunsetDate: '2026-12-31' })
  })

  it('the changelog editor lists entries, deletes one, and adds a new one when editing', async () => {
    const list = vi.spyOn(api, 'adminGetChangelog').mockResolvedValue([
      { id: 11, version: '1.0.0', kind: 'added', notes: 'Initial release', date: '2026-01-01' },
    ])
    const del = vi.spyOn(api, 'deleteChangelogEntry').mockResolvedValue(undefined)
    const add = vi.spyOn(api, 'addChangelogEntry').mockResolvedValue({ id: 12, version: '1.1.0', kind: 'fixed', notes: 'Bugfix', date: '2026-02-01' })
    renderPage()
    await screen.findByText('CurrencyConverterAPI')
    await userEvent.click(screen.getAllByRole('button', { name: 'Modifier' })[0])
    expect(await screen.findByText('Journal des modifications')).toBeInTheDocument()
    expect(await screen.findByText('Initial release')).toBeInTheDocument()
    expect(list).toHaveBeenCalledWith('jwt', 1)

    await userEvent.click(screen.getByRole('button', { name: 'Supprimer une entrée du journal' }))
    await waitFor(() => expect(del).toHaveBeenCalledWith('jwt', 1, 11))

    await userEvent.type(screen.getByLabelText(/Nouvelle version/i), '1.1.0')
    await userEvent.selectOptions(screen.getByLabelText(/Type de changement/i), 'fixed')
    await userEvent.type(screen.getByLabelText(/Date de publication/i), '2026-02-01')
    await userEvent.type(screen.getByLabelText(/Notes du changelog/i), 'Bugfix')
    await userEvent.click(screen.getByRole('button', { name: /^Ajouter$/i }))
    await waitFor(() => expect(add).toHaveBeenCalledWith('jwt', 1, { version: '1.1.0', kind: 'fixed', notes: 'Bugfix', date: '2026-02-01' }))
  })

  it('shows changelog entries for an unpublished (draft) product via the admin listing endpoint', async () => {
    // Regression test: the admin editor must use the ADMIN changelog endpoint
    // (adminGetChangelog), not the public published-only one, or a draft's
    // entries would always appear empty.
    const list = vi.spyOn(api, 'adminGetChangelog').mockResolvedValue([
      { id: 21, version: '0.9.0', kind: 'added', notes: 'Draft entry', date: '2026-01-05' },
    ])
    renderPage()
    await screen.findByText('PizzaShackAPI')
    await userEvent.click(screen.getAllByRole('button', { name: 'Modifier' })[1])
    expect(await screen.findByText('Journal des modifications')).toBeInTheDocument()
    expect(await screen.findByText('Draft entry')).toBeInTheDocument()
    expect(list).toHaveBeenCalledWith('jwt', 2)
  })

  it('pressing Enter in a changelog field adds the entry instead of submitting the product form', async () => {
    vi.spyOn(api, 'adminGetChangelog').mockResolvedValue([])
    const add = vi.spyOn(api, 'addChangelogEntry').mockResolvedValue({ id: 12, version: '1.1.0', kind: 'added', notes: '', date: '2026-02-01' })
    const update = vi.spyOn(api, 'adminUpdateProduct').mockResolvedValue(products[0])
    renderPage()
    await screen.findByText('CurrencyConverterAPI')
    await userEvent.click(screen.getAllByRole('button', { name: 'Modifier' })[0])
    expect(await screen.findByText('Journal des modifications')).toBeInTheDocument()

    const versionInput = screen.getByLabelText(/Nouvelle version/i)
    await userEvent.type(versionInput, '1.1.0')
    await userEvent.type(screen.getByLabelText(/Date de publication/i), '2026-02-01')
    fireEvent.keyDown(versionInput, { key: 'Enter' })

    await waitFor(() => expect(add).toHaveBeenCalledWith('jwt', 1, { version: '1.1.0', kind: 'added', notes: '', date: '2026-02-01' }))
    expect(update).not.toHaveBeenCalled()
  })

  it('persists the imported spec in the create payload', async () => {
    const create = vi.spyOn(api, 'adminCreateProduct').mockResolvedValue({} as AdminProduct)
    vi.spyOn(api, 'adminImportProduct').mockResolvedValue({
      name: 'Imported API', slug: 'imported', category: 'Finance', version: '2.5.0',
      contextPath: '/v2', description: '', tags: [], icon: '', upstreamUrl: '', published: false,
      openapiSpec: '{"openapi":"3.0.0","info":{"title":"Imported API","version":"2.5.0"}}',
    })
    renderPage()
    await screen.findByText('CurrencyConverterAPI')
    await userEvent.click(screen.getByRole('button', { name: /Importer une API/i }))
    await userEvent.click(screen.getByRole('tab', { name: /URL/i }))
    await userEvent.type(screen.getByPlaceholderText(/https/i), 'https://x/openapi.json')
    await userEvent.click(screen.getByRole('button', { name: /^Importer$/i }))
    await screen.findByText('Créer un produit')
    await userEvent.click(screen.getByRole('button', { name: /Créer le produit/i }))
    await waitFor(() => expect(create).toHaveBeenCalled())
    const payload = create.mock.calls[0][1]
    expect(payload.openapiSpec).toContain('"openapi":"3.0.0"')
  })

  it('shows the built-in icon grid and selects a glyph', async () => {
    renderPage()
    await screen.findByText('CurrencyConverterAPI')
    await userEvent.click(screen.getByRole('button', { name: /Nouveau produit/ }))
    const grid = screen.getByTestId('icon-picker')
    expect(grid).toBeTruthy()
    // 9 built-in glyphs + 1 Default tile
    expect(grid.querySelectorAll('button.icon-tile').length).toBe(10)
    fireEvent.click(grid.querySelector('button.icon-tile[data-key="pizza"]')!)
    expect(grid.querySelector('button.icon-tile[data-key="pizza"]')!.getAttribute('aria-pressed')).toBe('true')
  })

  it('disables custom upload in create mode with a hint', async () => {
    renderPage()
    await screen.findByText('CurrencyConverterAPI')
    await userEvent.click(screen.getByRole('button', { name: /Nouveau produit/ }))
    expect((screen.getByTestId('icon-upload') as HTMLInputElement).disabled).toBe(true)
    expect(screen.getByTestId('icon-upload-hint')).toBeTruthy()
  })

  it('hides the sandbox upstream field when no sandbox gateway is configured', async () => {
    vi.spyOn(api, 'adminGetMeta').mockResolvedValue({ sandboxConfigured: false, oidcConfigured: false })
    renderPage()
    await screen.findByText('CurrencyConverterAPI')
    await userEvent.click(screen.getByRole('button', { name: /Nouveau produit/ }))
    expect(screen.getByLabelText('Nom')).toBeInTheDocument()
    expect(screen.queryByLabelText(/Sandbox/)).not.toBeInTheDocument()
  })

  it('shows the sandbox upstream field when the sandbox gateway is configured', async () => {
    renderPage()
    await screen.findByText('CurrencyConverterAPI')
    await userEvent.click(screen.getByRole('button', { name: /Nouveau produit/ }))
    expect(screen.getByLabelText(/Sandbox/)).toBeInTheDocument()
  })
})
