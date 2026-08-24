import { it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { ProductDetailPage } from './ProductDetailPage'
import { AuthProvider } from '../auth/AuthProvider'
import { ThemeProvider } from '../theme/ThemeProvider'
import { LanguageProvider } from '../i18n/LanguageProvider'
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
  // jsdom's navigator.language defaults to 'en-US', which would auto-detect to
  // English; force French so existing assertions (against French strings) hold.
  localStorage.setItem('lang', 'fr')
  vi.spyOn(api, 'getProduct').mockResolvedValue(product)
  vi.spyOn(api, 'getChangelog').mockResolvedValue([])
})
afterEach(() => vi.restoreAllMocks())

function renderAt(slug: string) {
  return render(
    <MemoryRouter initialEntries={[`/catalog/${slug}`]}>
      <LanguageProvider><ThemeProvider><AuthProvider>
        <Routes><Route path="/catalog/:slug" element={<ProductDetailPage />} /></Routes>
      </AuthProvider></ThemeProvider></LanguageProvider>
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

// Regression tests for #11: the "Try it" area was a dead end when no spec
// was attached, even for a subscribed user with a valid app/key.
it('shows the manual try panel (not the placeholder) for a subscribed user with no spec', async () => {
  localStorage.setItem('token', 'jwt')
  localStorage.setItem('user', JSON.stringify({ id: 1, email: 'a@b.c', name: 'D', role: 'developer' }))
  vi.spyOn(api, 'getProductSpec').mockResolvedValue(null)
  vi.spyOn(api, 'getTryContext').mockResolvedValue({ apps: [{ id: 3, name: 'App A' }] })
  renderAt('orders')
  expect(await screen.findByRole('heading', { name: 'Essayer manuellement' })).toBeInTheDocument()
  expect(screen.queryByText(/Documentation bientôt disponible/i)).not.toBeInTheDocument()
  expect(screen.getByText('/api/try/orders/3')).toBeInTheDocument()
})

it('sends a manual request through the tryit proxy and shows the response', async () => {
  const user = userEvent.setup()
  localStorage.setItem('token', 'jwt')
  localStorage.setItem('user', JSON.stringify({ id: 1, email: 'a@b.c', name: 'D', role: 'developer' }))
  vi.spyOn(api, 'getProductSpec').mockResolvedValue(null)
  vi.spyOn(api, 'getTryContext').mockResolvedValue({ apps: [{ id: 3, name: 'App A' }] })
  // Reviews also fetches ratings on mount via the real fetch — only intercept
  // the tryit proxy call itself, and let anything else fail like an
  // unmocked fetch normally does in these tests (Reviews swallows that error).
  const f = vi.fn(async (input: RequestInfo | URL) => {
    const url = typeof input === 'string' ? input : input.toString()
    if (url.startsWith('/api/try/')) {
      return new Response('{"ok":true}', { status: 200, statusText: 'OK', headers: { 'Content-Type': 'application/json' } })
    }
    return new Response('{}', { status: 500 })
  })
  vi.stubGlobal('fetch', f)
  renderAt('orders')
  await screen.findByRole('heading', { name: 'Essayer manuellement' })
  await user.type(screen.getByLabelText('Chemin de la requête'), '/rates')
  await user.click(screen.getByRole('button', { name: 'Envoyer la requête' }))
  await waitFor(() => expect(f).toHaveBeenCalled())
  const call = f.mock.calls.find(([u]) => (typeof u === 'string' ? u : u.toString()).startsWith('/api/try/'))
  const [url, init] = call!
  expect(url).toBe('/api/try/orders/3/rates')
  expect((init?.headers as Record<string, string>).Authorization).toBe('Bearer jwt')
  expect(await screen.findByText('200 OK')).toBeInTheDocument()
  expect(screen.getByText('{"ok":true}')).toBeInTheDocument()
  vi.unstubAllGlobals()
})

// Regression test for a code-review finding on #11's own fix: a user-typed
// "Authorization" header in the custom-headers box must never override the
// injected portal token, since fetch() would otherwise send two conflicting
// Authorization values (comma-joined by the Headers algorithm) or the
// wrong one outright.
it('does not let a typed Authorization header override the injected token', async () => {
  localStorage.setItem('token', 'jwt')
  localStorage.setItem('user', JSON.stringify({ id: 1, email: 'a@b.c', name: 'D', role: 'developer' }))
  vi.spyOn(api, 'getProductSpec').mockResolvedValue(null)
  vi.spyOn(api, 'getTryContext').mockResolvedValue({ apps: [{ id: 3, name: 'App A' }] })
  const f = vi.fn(async (input: RequestInfo | URL) => {
    const url = typeof input === 'string' ? input : input.toString()
    if (url.startsWith('/api/try/')) {
      return new Response('{}', { status: 200, statusText: 'OK' })
    }
    return new Response('{}', { status: 500 })
  })
  vi.stubGlobal('fetch', f)
  renderAt('orders')
  await screen.findByRole('heading', { name: 'Essayer manuellement' })
  await userEvent.click(screen.getByText('En-têtes personnalisés (optionnel)'))
  await userEvent.type(screen.getByLabelText('En-têtes personnalisés (optionnel)'), 'Authorization: evil-value')
  await userEvent.click(screen.getByRole('button', { name: 'Envoyer la requête' }))
  await waitFor(() => expect(f).toHaveBeenCalled())
  const call = f.mock.calls.find(([u]) => (typeof u === 'string' ? u : u.toString()).startsWith('/api/try/'))
  const [, init] = call!
  const headers = init?.headers as Record<string, string>
  expect(headers.Authorization).toBe('Bearer jwt')
  expect(Object.keys(headers).filter(k => k.toLowerCase() === 'authorization')).toHaveLength(1)
  vi.unstubAllGlobals()
})

// Regression test for a code-review finding on #11's own fix: ManualTryPanel
// wasn't remounted when the "try with" app picker switched apps, so the
// previous app's response stayed on screen after switching — misleading the
// developer into thinking it was the newly-picked app's response.
it('clears the previous response when switching to a different app', async () => {
  localStorage.setItem('token', 'jwt')
  localStorage.setItem('user', JSON.stringify({ id: 1, email: 'a@b.c', name: 'D', role: 'developer' }))
  vi.spyOn(api, 'getProductSpec').mockResolvedValue(null)
  vi.spyOn(api, 'getTryContext').mockResolvedValue({ apps: [{ id: 3, name: 'App A' }, { id: 4, name: 'App B' }] })
  const f = vi.fn(async (input: RequestInfo | URL) => {
    const url = typeof input === 'string' ? input : input.toString()
    if (url.startsWith('/api/try/')) {
      return new Response('{"ok":true}', { status: 200, statusText: 'OK' })
    }
    return new Response('{}', { status: 500 })
  })
  vi.stubGlobal('fetch', f)
  renderAt('orders')
  await screen.findByRole('heading', { name: 'Essayer manuellement' })
  await userEvent.click(screen.getByRole('button', { name: 'Envoyer la requête' }))
  expect(await screen.findByText('200 OK')).toBeInTheDocument()
  await userEvent.selectOptions(screen.getByLabelText('Essayer avec :'), '4')
  expect(screen.queryByText('200 OK')).not.toBeInTheDocument()
  vi.unstubAllGlobals()
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
