import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { OverviewTab } from './OverviewTab'
import { UsageTab } from './UsageTab'
import { SettingsTab } from './SettingsTab'
import type { AppDetail, Application } from '../../api/types'
import type { ModalSpec } from '../../components/ConfirmModal'

beforeEach(() => {
  Object.assign(navigator, { clipboard: { writeText: vi.fn().mockResolvedValue(undefined) } })
})

const app: Application = { id: 4, ownerId: 1, name: 'Boutique Mobile', description: 'desc app', createdAt: '2026-03-12T00:00:00Z' }
const detail: AppDetail = {
  apiKey: 'ax_live_real_key_0001', consumerUsername: 'app_4',
  subscriptions: [{ productId: 9, productName: 'Orders API', version: '2.1.0', contextPath: '/orders', planId: 3, planName: 'Gold', status: 'active' }],
}

describe('OverviewTab', () => {
  it('renders demo stats and a quickstart with the real gateway path + key', () => {
    const notify = vi.fn()
    render(<OverviewTab detail={detail} notify={notify} />)
    expect(screen.getByText("Requêtes · aujourd'hui")).toBeInTheDocument()
    expect(screen.getByText(/9080\/orders/)).toBeInTheDocument()
    expect(screen.getByText(/ax_live_real_key_0001/)).toBeInTheDocument()
  })
  it('falls back to blueprint sample without an active subscription', () => {
    render(<OverviewTab detail={{ ...detail, subscriptions: [] }} notify={() => {}} />)
    expect(screen.getByText(/ax_live_a3f9c1e7b240d8e5f6/)).toBeInTheDocument()
  })
  it('copy button copies the curl command', async () => {
    const notify = vi.fn()
    render(<OverviewTab detail={detail} notify={notify} />)
    await userEvent.click(screen.getByRole('button', { name: 'Copier' }))
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(expect.stringContaining('ax_live_real_key_0001'))
    expect(notify).toHaveBeenCalledWith('Commande copiée')
  })
})

describe('UsageTab', () => {
  it('renders 14 chart columns and the per-API demo table', () => {
    render(<UsageTab />)
    expect(screen.getAllByTestId('chart-col')).toHaveLength(14)
    expect(screen.getByText('Orders API')).toBeInTheDocument()
    expect(screen.getByText('248 910')).toBeInTheDocument()
  })
})

describe('SettingsTab', () => {
  it('prefills real name/description and saving shows the demo toast', async () => {
    const notify = vi.fn()
    render(<SettingsTab app={app} notify={notify} openModal={() => {}} />)
    expect(screen.getByLabelText("Nom de l'application")).toHaveValue('Boutique Mobile')
    expect(screen.getByLabelText('Description')).toHaveValue('desc app')
    await userEvent.click(screen.getByRole('button', { name: /Enregistrer/ }))
    expect(notify).toHaveBeenCalledWith('Modifications enregistrées (démo)')
  })
  it('resyncs the form when a different app is shown (switcher navigation)', () => {
    const { rerender } = render(<SettingsTab app={app} notify={() => {}} openModal={() => {}} />)
    const app2: Application = { ...app, id: 5, name: 'Autre App', description: 'autre desc' }
    rerender(<SettingsTab app={app2} notify={() => {}} openModal={() => {}} />)
    expect(screen.getByLabelText("Nom de l'application")).toHaveValue('Autre App')
    expect(screen.getByLabelText('Description')).toHaveValue('autre desc')
  })
  it('delete app goes through the danger modal then demo toast', async () => {
    const notify = vi.fn()
    let lastModal: ModalSpec | null = null
    render(<SettingsTab app={app} notify={notify} openModal={s => { lastModal = s }} />)
    await userEvent.click(screen.getByRole('button', { name: /Supprimer l'application/ }))
    expect(lastModal!.danger).toBe(true)
    lastModal!.onConfirm()
    expect(notify).toHaveBeenCalledWith('Application supprimée (démo)')
  })
})
