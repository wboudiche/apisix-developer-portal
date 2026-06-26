import { it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { ProductDetailPage } from './ProductDetailPage'
import { AuthProvider } from '../auth/AuthProvider'
import { ThemeProvider } from '../theme/ThemeProvider'
import * as api from '../api/client'
import type { Product } from '../api/types'

// Mock Scalar so the page test doesn't pull the real renderer.
vi.mock('@scalar/api-reference-react', () => ({
  ApiReferenceReact: ({ configuration }: { configuration: { content: string } }) => (
    <div data-testid="scalar" data-content={configuration.content} />
  ),
}))

const product: Product = {
  id: 1, name: 'Orders API', slug: 'orders', category: 'Data', version: '2.1.0',
  contextPath: '/orders', description: 'Gère les commandes.', tags: ['data'], icon: '', rating: 4,
}

beforeEach(() => {
  localStorage.clear()
  vi.spyOn(api, 'getProduct').mockResolvedValue(product)
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
