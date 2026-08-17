import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { AppDetailPage } from './AppDetailPage'
import { ApplicationsIndex } from './ApplicationsIndex'
import { AuthProvider } from '../../auth/AuthProvider'
import { LanguageProvider } from '../../i18n/LanguageProvider'
import * as api from '../../api/client'
import type { Application, AppDetail, Plan } from '../../api/types'

const apps: Application[] = [
  { id: 1, ownerId: 1, name: 'Boutique Mobile', description: 'desc', createdAt: '2026-03-12T00:00:00Z', subscriptionCount: 2 },
  { id: 2, ownerId: 1, name: 'Analytics interne', description: '', createdAt: '2026-04-02T00:00:00Z', subscriptionCount: 0 },
]
const detail: AppDetail = {
  apiKey: 'ax_live_k1', consumerUsername: 'app_1',
  subscriptions: [
    { productId: 9, productName: 'Orders API', version: '2.1.0', contextPath: '/orders', planId: 3, planName: 'Gold', status: 'active' },
    { productId: 5, productName: 'Inventory API', version: '1.4.0', contextPath: '/inventory', planId: 1, planName: 'Free', status: 'pending' },
  ],
  events: [],
}
const plans: Plan[] = [
  { id: 1, name: 'Free', rateLimit: 60, windowSeconds: 60, priceCents: 0, currency: 'EUR' },
  { id: 3, name: 'Gold', rateLimit: 1000, windowSeconds: 60, priceCents: 2900, currency: 'EUR' },
]

beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('token', 'jwt')
  localStorage.setItem('user', JSON.stringify({ id: 1, email: 'a@b.c', name: 'Dev', role: 'developer' }))
  localStorage.setItem('lang', 'fr')
  vi.restoreAllMocks()
  vi.spyOn(api, 'getApplications').mockResolvedValue({ items: apps, total: apps.length, page: 1, pageSize: 20 })
  vi.spyOn(api, 'getApplicationDetail').mockResolvedValue(detail)
  vi.spyOn(api, 'getPlans').mockResolvedValue({ items: plans, total: plans.length, page: 1, pageSize: 20 })
  Object.assign(navigator, { clipboard: { writeText: vi.fn().mockResolvedValue(undefined) } })
})

function renderAt(path: string) {
  return render(
    <LanguageProvider>
      <MemoryRouter initialEntries={[path]}>
        <AuthProvider>
          <Routes>
            <Route path="/applications" element={<ApplicationsIndex />} />
            <Route path="/applications/:id" element={<AppDetailPage />} />
            <Route path="/" element={<div>CATALOG</div>} />
            <Route path="/login" element={<div>LOGIN</div>} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    </LanguageProvider>
  )
}

describe('ApplicationsIndex', () => {
  it('lists all applications with links to their detail', async () => {
    renderAt('/applications')
    const first = await screen.findByRole('link', { name: /Boutique Mobile/ })
    expect(first).toHaveAttribute('href', '/applications/1')
    const second = screen.getByRole('link', { name: /Analytics interne/ })
    expect(second).toHaveAttribute('href', '/applications/2')
    // It is a list, not a redirect: it does NOT jump into the first app's detail.
    expect(screen.queryByText("Changer d'application")).not.toBeInTheDocument()
  })

  it('shows per-app subscription count', async () => {
    renderAt('/applications')
    const first = await screen.findByRole('link', { name: /Boutique Mobile/ })
    expect(within(first).getByText('2 abonnements')).toBeInTheDocument()
    const second = screen.getByRole('link', { name: /Analytics interne/ })
    expect(within(second).getByText('0 abonnement')).toBeInTheDocument()
  })

  it('opens an application detail when its card is clicked', async () => {
    renderAt('/applications')
    const first = await screen.findByRole('link', { name: /Boutique Mobile/ })
    await userEvent.click(first)
    await waitFor(() => expect(screen.getByText("Changer d'application")).toBeInTheDocument())
  })

  it('shows the create form when no apps exist', async () => {
    vi.spyOn(api, 'getApplications').mockResolvedValue({ items: [], total: 0, page: 1, pageSize: 20 })
    renderAt('/applications')
    expect(await screen.findByText(/Créez votre première application/)).toBeInTheDocument()
  })
})

describe('AppDetailPage', () => {
  it('renders header with real name, id ref, status pill and subs badge', async () => {
    renderAt('/applications/1')
    expect(await screen.findByRole('heading', { level: 1, name: /Boutique Mobile/ })).toBeInTheDocument()
    expect(screen.getByText('app_1', { selector: '.mono' })).toBeInTheDocument()
    expect(screen.getByText('2 abonnements')).toBeInTheDocument()
    expect(await screen.findByText('Créée le', { exact: false })).toBeInTheDocument()
  })
  it('switches tabs and persists to localStorage', async () => {
    renderAt('/applications/1')
    await screen.findByRole('heading', { level: 1, name: /Boutique Mobile/ })
    await userEvent.click(screen.getByRole('button', { name: /^Identifiants$/ }))
    expect(screen.getByTestId('key-prod')).toBeInTheDocument()
    expect(localStorage.getItem('app:tab')).toBe('creds')
  })
  it('résilier flow: modal → confirm → api → refetch', async () => {
    const unsub = vi.spyOn(api, 'unsubscribe').mockResolvedValue(undefined)
    renderAt('/applications/1')
    await screen.findByRole('heading', { level: 1, name: /Boutique Mobile/ })
    await userEvent.click(screen.getByRole('button', { name: /Abonnements/ }))
    await userEvent.click(screen.getAllByText('Résilier')[0])
    expect(screen.getByText(/Résilier l'abonnement à Orders API/)).toBeInTheDocument()
    const dialog = screen.getByRole('dialog')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Résilier' }))
    await waitFor(() => expect(unsub).toHaveBeenCalledWith('jwt', 1, 9))
    expect(api.getApplicationDetail).toHaveBeenCalledTimes(2)
  })
  it('delete flow: modal → confirm → api → navigate to the applications list', async () => {
    const del = vi.spyOn(api, 'deleteApplication').mockResolvedValue(undefined)
    renderAt('/applications/1')
    await screen.findByRole('heading', { level: 1, name: /Boutique Mobile/ })
    // The Settings tab button shares its label with an unrelated shortcut
    // button elsewhere on the page — scope to the tab bar to disambiguate.
    await userEvent.click(within(document.querySelector('.tabs')!).getByRole('button', { name: /Paramètres/ }))
    await userEvent.click(screen.getByRole('button', { name: /Supprimer l'application/ }))
    expect(screen.getByText(/Supprimer « Boutique Mobile »/)).toBeInTheDocument()
    const dialog = screen.getByRole('dialog')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Supprimer définitivement' }))
    await waitFor(() => expect(del).toHaveBeenCalledWith('jwt', 1))
    await waitFor(() => expect(screen.getByRole('heading', { level: 1, name: 'Applications' })).toBeInTheDocument())
  })
  it('delete failure shows an error toast and stays on the page', async () => {
    const del = vi.spyOn(api, 'deleteApplication').mockRejectedValue(new Error('boom'))
    renderAt('/applications/1')
    await screen.findByRole('heading', { level: 1, name: /Boutique Mobile/ })
    await userEvent.click(within(document.querySelector('.tabs')!).getByRole('button', { name: /Paramètres/ }))
    await userEvent.click(screen.getByRole('button', { name: /Supprimer l'application/ }))
    const dialog = screen.getByRole('dialog')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Supprimer définitivement' }))
    await waitFor(() => expect(del).toHaveBeenCalled())
    expect(await screen.findByText("Échec de la suppression de l'application.")).toBeInTheDocument()
    expect(screen.getByRole('heading', { level: 1, name: /Boutique Mobile/ })).toBeInTheDocument()
  })
  it('create app from switcher navigates to the new app', async () => {
    const created: Application = { id: 7, ownerId: 1, name: 'Nouvelle', description: '', createdAt: '2026-06-05T00:00:00Z' }
    vi.spyOn(api, 'createApplication').mockResolvedValue(created)
    renderAt('/applications/1')
    await screen.findByRole('heading', { level: 1, name: /Boutique Mobile/ })
    await userEvent.click(screen.getByRole('button', { name: /Changer d'application/ }))
    await userEvent.click(screen.getByText('Nouvelle application'))
    await userEvent.type(screen.getByLabelText("Nom de l'application"), 'Nouvelle')
    await userEvent.click(screen.getByRole('button', { name: 'Créer' }))
    await waitFor(() => expect(api.createApplication).toHaveBeenCalledWith('jwt', 'Nouvelle', ''))
  })
  it('unknown app id redirects to the /applications list', async () => {
    renderAt('/applications/999')
    await waitFor(() => expect(screen.getByRole('link', { name: /Boutique Mobile/ })).toBeInTheDocument())
    expect(screen.getByRole('heading', { level: 1, name: 'Applications' })).toBeInTheDocument()
  })
  it('a slow stale detail response never overwrites the current app', async () => {
    // app 1's detail hangs; app 2's resolves immediately
    let releaseApp1!: (d: AppDetail) => void
    const detail2: AppDetail = { apiKey: 'ax_live_k2', consumerUsername: 'app_2', subscriptions: [], events: [] }
    vi.spyOn(api, 'getApplicationDetail').mockImplementation((_t, id) =>
      id === 1 ? new Promise<AppDetail>(res => { releaseApp1 = res }) : Promise.resolve(detail2))
    renderAt('/applications/1')
    await screen.findByRole('heading', { level: 1, name: /Boutique Mobile/ })
    // navigate to app 2 via the switcher while app 1's request is in flight
    await userEvent.click(screen.getByRole('button', { name: /Changer d'application/ }))
    await userEvent.click(screen.getByText('Analytics interne'))
    await screen.findByRole('heading', { level: 1, name: /Analytics interne/ })
    await userEvent.click(screen.getByRole('button', { name: /^Identifiants$/ }))
    expect(screen.getByTestId('key-prod').textContent).toMatch(/k2$/) // masked: first8+•…+last2
    // the stale response for app 1 lands now — it must be discarded
    releaseApp1(detail)
    await waitFor(() => expect(screen.getByTestId('key-prod').textContent).toMatch(/k2$/))
    expect(screen.getByTestId('key-prod').textContent).not.toMatch(/k1$/)
  })
})
