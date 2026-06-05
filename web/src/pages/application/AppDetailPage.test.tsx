import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { AppDetailPage } from './AppDetailPage'
import { ApplicationsIndex } from './ApplicationsIndex'
import { AuthProvider } from '../../auth/AuthProvider'
import * as api from '../../api/client'
import type { Application, AppDetail, Plan } from '../../api/types'

const apps: Application[] = [
  { id: 1, ownerId: 1, name: 'Boutique Mobile', description: 'desc', createdAt: '2026-03-12T00:00:00Z' },
  { id: 2, ownerId: 1, name: 'Analytics interne', description: '', createdAt: '2026-04-02T00:00:00Z' },
]
const detail: AppDetail = {
  apiKey: 'ax_live_k1', consumerUsername: 'app_1',
  subscriptions: [
    { productId: 9, productName: 'Orders API', version: '2.1.0', contextPath: '/orders', planId: 3, planName: 'Gold', status: 'active' },
    { productId: 5, productName: 'Inventory API', version: '1.4.0', contextPath: '/inventory', planId: 1, planName: 'Free', status: 'pending' },
  ],
}
const plans: Plan[] = [
  { id: 1, name: 'Free', rateLimit: 60, windowSeconds: 60 },
  { id: 3, name: 'Gold', rateLimit: 1000, windowSeconds: 60 },
]

beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('token', 'jwt')
  localStorage.setItem('user', JSON.stringify({ id: 1, email: 'a@b.c', name: 'Dev', role: 'developer' }))
  vi.restoreAllMocks()
  vi.spyOn(api, 'getApplications').mockResolvedValue(apps)
  vi.spyOn(api, 'getApplicationDetail').mockResolvedValue(detail)
  vi.spyOn(api, 'getPlans').mockResolvedValue(plans)
  Object.assign(navigator, { clipboard: { writeText: vi.fn().mockResolvedValue(undefined) } })
})

function renderAt(path: string) {
  return render(
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
  )
}

describe('ApplicationsIndex', () => {
  it('redirects to the first application', async () => {
    renderAt('/applications')
    await waitFor(() => expect(screen.getByText('Boutique Mobile', { selector: 'h1, h1 *' })).toBeInTheDocument())
  })
  it('shows the create form when no apps exist', async () => {
    vi.spyOn(api, 'getApplications').mockResolvedValue([])
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
  it('unknown app id redirects to /applications', async () => {
    renderAt('/applications/999')
    await waitFor(() => expect(screen.getByRole('heading', { level: 1, name: /Boutique Mobile/ })).toBeInTheDocument())
  })
  it('a slow stale detail response never overwrites the current app', async () => {
    // app 1's detail hangs; app 2's resolves immediately
    let releaseApp1!: (d: AppDetail) => void
    const detail2: AppDetail = { apiKey: 'ax_live_k2', consumerUsername: 'app_2', subscriptions: [] }
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
