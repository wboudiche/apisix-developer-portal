import type { ReactElement } from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { OverviewTab } from './OverviewTab'
import { UsageTab } from './UsageTab'
import { SettingsTab } from './SettingsTab'
import { LanguageProvider } from '../../i18n/LanguageProvider'
import type { AppDetail, Application, Usage } from '../../api/types'
import type { ModalSpec } from '../../components/ConfirmModal'

function renderT(ui: ReactElement) {
  return render(<LanguageProvider>{ui}</LanguageProvider>)
}

// Stub the metrics hook so these tab tests stay focused on the tab's own
// concerns (quickstart, feed, chart presence); the cards/chart rendering itself
// is covered by UsageCards/UsageChart tests.
const stubUsage: Usage = {
  summary: { requestsToday: 18402, monthToDate: 421000, p95Ms: 86, errorRate: 0.0021 },
  series: [
    { t: '2026-06-14T10:00:00Z', requests: 10, errors: 0 },
    { t: '2026-06-14T11:00:00Z', requests: 20, errors: 1 },
  ],
}
vi.mock('./useUsage', () => ({ useUsage: () => ({ status: 'ready', usage: stubUsage }) }))

beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('lang', 'fr')
  Object.assign(navigator, { clipboard: { writeText: vi.fn().mockResolvedValue(undefined) } })
})

const app: Application = { id: 4, ownerId: 1, name: 'Boutique Mobile', description: 'desc app', createdAt: '2026-03-12T00:00:00Z' }
const detail: AppDetail = {
  apiKey: 'ax_live_real_key_0001', consumerUsername: 'app_4',
  subscriptions: [{ productId: 9, productName: 'Orders API', version: '2.1.0', contextPath: '/orders', planId: 3, planName: 'Gold', status: 'active' }],
  events: [
    { kind: 'subscribed', productName: 'Orders API', planName: 'Gold', createdAt: '2026-03-12T00:00:00Z' },
  ],
}

describe('OverviewTab', () => {
  it('renders the real metric cards and a quickstart with the real gateway path + key', () => {
    const notify = vi.fn()
    renderT(<OverviewTab detail={detail} token="t" appId={4} notify={notify} />)
    expect(screen.getByText("Requêtes · aujourd'hui")).toBeInTheDocument()
    expect(screen.getByText(/9080\/orders/)).toBeInTheDocument()
    expect(screen.getByText(/ax_live_real_key_0001/)).toBeInTheDocument()
  })
  it('falls back to blueprint sample without an active subscription', () => {
    renderT(<OverviewTab detail={{ ...detail, subscriptions: [] }} token="t" appId={4} notify={() => {}} />)
    expect(screen.getByText(/ax_live_a3f9c1e7b240d8e5f6/)).toBeInTheDocument()
  })
  it('copy button copies the curl command', async () => {
    const notify = vi.fn()
    renderT(<OverviewTab detail={detail} token="t" appId={4} notify={notify} />)
    await userEvent.click(screen.getByRole('button', { name: 'Copier' }))
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(expect.stringContaining('ax_live_real_key_0001'))
    expect(notify).toHaveBeenCalledWith('Commande copiée')
  })
  it('renders the real activity feed from detail.events', () => {
    renderT(<OverviewTab detail={detail} token="t" appId={4} notify={() => {}} />)
    expect(screen.getByText('Abonnement')).toBeInTheDocument()
    expect(screen.getByText(/à Orders API · plan Gold/)).toBeInTheDocument()
  })
  it('shows an empty state when there is no activity', () => {
    renderT(<OverviewTab detail={{ ...detail, events: [] }} token="t" appId={4} notify={() => {}} />)
    expect(screen.getByText(/Aucune activité pour le moment/)).toBeInTheDocument()
  })
})

describe('UsageTab', () => {
  it('renders the traffic chart (one column per series point)', () => {
    renderT(<UsageTab token="t" appId={4} />)
    expect(screen.getAllByTestId('chart-col')).toHaveLength(stubUsage.series.length)
  })
})

describe('SettingsTab', () => {
  it('prefills real name/description and saving shows the demo toast', async () => {
    const notify = vi.fn()
    renderT(<SettingsTab app={app} notify={notify} openModal={() => {}} />)
    expect(screen.getByLabelText("Nom de l'application")).toHaveValue('Boutique Mobile')
    expect(screen.getByLabelText('Description')).toHaveValue('desc app')
    await userEvent.click(screen.getByRole('button', { name: /Enregistrer/ }))
    expect(notify).toHaveBeenCalledWith('Modifications enregistrées (démo)')
  })
  it('resyncs the form when a different app is shown (switcher navigation)', () => {
    const { rerender } = renderT(<SettingsTab app={app} notify={() => {}} openModal={() => {}} />)
    const app2: Application = { ...app, id: 5, name: 'Autre App', description: 'autre desc' }
    rerender(<LanguageProvider><SettingsTab app={app2} notify={() => {}} openModal={() => {}} /></LanguageProvider>)
    expect(screen.getByLabelText("Nom de l'application")).toHaveValue('Autre App')
    expect(screen.getByLabelText('Description')).toHaveValue('autre desc')
  })
  it('delete app goes through the danger modal then demo toast', async () => {
    const notify = vi.fn()
    let lastModal: ModalSpec | null = null
    renderT(<SettingsTab app={app} notify={notify} openModal={s => { lastModal = s }} />)
    await userEvent.click(screen.getByRole('button', { name: /Supprimer l'application/ }))
    expect(lastModal!.danger).toBe(true)
    lastModal!.onConfirm()
    expect(notify).toHaveBeenCalledWith('Application supprimée (démo)')
  })
})
