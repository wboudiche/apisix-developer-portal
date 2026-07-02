import { it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { ProductDetailPage } from './ProductDetailPage'
import { AuthProvider } from '../auth/AuthProvider'
import { ThemeProvider } from '../theme/ThemeProvider'
import * as api from '../api/client'
import type { Product } from '../api/types'

// Mock Scalar so the page test doesn't pull the real renderer.
vi.mock('@scalar/api-reference-react', () => ({
  ApiReferenceReact: ({ configuration }: { configuration: { content: string; servers?: { url: string }[] } }) => (
    <div data-testid="scalar" data-content={configuration.content} data-server={configuration.servers?.[0]?.url ?? ''} />
  ),
}))

const product: Product = {
  id: 1, name: 'Orders API', slug: 'orders', category: 'Data', version: '2.1.0',
  contextPath: '/orders', description: 'Gère les commandes.', tags: ['data'], icon: '', rating: 4, ratingCount: 0,
}
const baseProduct = product

beforeEach(() => {
  localStorage.clear()
  vi.spyOn(api, 'getProduct').mockResolvedValue(product)
  vi.spyOn(api, 'getChangelog').mockResolvedValue([])
})
afterEach(() => vi.restoreAllMocks())

function renderAt(slug: string) {
  return render(
    <MemoryRouter initialEntries={[`/catalog/${slug}`]}>
      <ThemeProvider><AuthProvider>
        <Routes><Route path="/catalog/:slug" element={<ProductDetailPage />} /></Routes>
      </AuthProvider></ThemeProvider>
    </MemoryRouter>
  )
}

it('renders the product header and the Scalar docs when a spec exists', async () => {
  vi.spyOn(api, 'getProductSpec').mockResolvedValue('{"openapi":"3.0.0"}')
  renderAt('orders')
  expect(await screen.findByRole('heading', { name: /Orders API/ })).toBeInTheDocument()
  expect(screen.getByText('Gère les commandes.')).toBeInTheDocument()
  await waitFor(() => expect(screen.getByTestId('scalar')).toHaveAttribute('data-content', '{"openapi":"3.0.0"}'))
})

it('shows a placeholder when the product has no spec', async () => {
  vi.spyOn(api, 'getProductSpec').mockResolvedValue(null)
  renderAt('orders')
  expect(await screen.findByText(/Documentation bientôt disponible/i)).toBeInTheDocument()
  expect(screen.queryByTestId('scalar')).not.toBeInTheDocument()
})

it('routes try-it through the proxy for a subscribed user (single app)', async () => {
  localStorage.setItem('token', 'jwt')
  localStorage.setItem('user', JSON.stringify({ id: 1, email: 'a@b.c', name: 'D', role: 'developer' }))
  vi.spyOn(api, 'getProductSpec').mockResolvedValue('{"openapi":"3.0.0"}')
  vi.spyOn(api, 'getTryContext').mockResolvedValue({ apps: [{ id: 3, name: 'App A' }] })
  renderAt('orders')
  await waitFor(() => expect(screen.getByTestId('scalar')).toHaveAttribute('data-server', '/api/try/orders/3'))
})

it('shows a subscribe banner when the user has no approved app', async () => {
  localStorage.setItem('token', 'jwt')
  localStorage.setItem('user', JSON.stringify({ id: 1, email: 'a@b.c', name: 'D', role: 'developer' }))
  vi.spyOn(api, 'getProductSpec').mockResolvedValue('{"openapi":"3.0.0"}')
  vi.spyOn(api, 'getTryContext').mockResolvedValue({ apps: [] })
  renderAt('orders')
  expect(await screen.findByText(/Abonnez-vous pour essayer/i)).toBeInTheDocument()
})

it('toggles the Try-it server between production and sandbox', async () => {
  const user = userEvent.setup()
  localStorage.setItem('token', 'jwt')
  localStorage.setItem('user', JSON.stringify({ id: 1, email: 'a@b.c', name: 'D', role: 'developer' }))
  vi.spyOn(api, 'getProductSpec').mockResolvedValue('{"openapi":"3.0.0"}')
  vi.spyOn(api, 'getTryContext').mockResolvedValue({ apps: [{ id: 3, name: 'App A' }], sandboxAvailable: true })
  renderAt('orders')
  await waitFor(() => expect(screen.getByTestId('scalar')).toHaveAttribute('data-server', '/api/try/orders/3'))
  const sandboxToggle = screen.getByRole('button', { name: /Sandbox/i })
  await user.click(sandboxToggle)
  await waitFor(() => expect(screen.getByTestId('scalar')).toHaveAttribute('data-server', '/api/try/orders/3/sandbox'))
})

it('does not show sandbox toggle when sandboxAvailable is false', async () => {
  localStorage.setItem('token', 'jwt')
  localStorage.setItem('user', JSON.stringify({ id: 1, email: 'a@b.c', name: 'D', role: 'developer' }))
  vi.spyOn(api, 'getProductSpec').mockResolvedValue('{"openapi":"3.0.0"}')
  vi.spyOn(api, 'getTryContext').mockResolvedValue({ apps: [{ id: 3, name: 'App A' }] })
  renderAt('orders')
  await waitFor(() => expect(screen.getByTestId('scalar')).toHaveAttribute('data-server', '/api/try/orders/3'))
  expect(screen.queryByRole('button', { name: /Sandbox/i })).not.toBeInTheDocument()
})

it('shows the deprecation notice, changelog, and a disabled subscribe for a deprecated product', async () => {
  vi.spyOn(api, 'getProduct').mockResolvedValue({ ...baseProduct, slug: 'dep', lifecycleStatus: 'deprecated' })
  vi.spyOn(api, 'getChangelog').mockResolvedValue([{ id: 1, version: 'v1.2', kind: 'deprecated', notes: 'moved to v2', date: '2026-07-01' }])
  vi.spyOn(api, 'getProductSpec').mockResolvedValue(null)
  renderAt('dep')
  expect(await screen.findByText(/n'accepte plus de nouveaux abonnements/i)).toBeInTheDocument()
  expect(screen.getByText('v1.2')).toBeInTheDocument()
  expect(screen.getByText(/moved to v2/)).toBeInTheDocument()
  const subBtn = screen.getByRole('button', { name: /S'abonner/i })
  expect(subBtn).toBeDisabled()
})
