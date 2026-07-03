import { it, expect, vi, afterEach, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { QuotaMeter } from './QuotaMeter'
import { LanguageProvider } from '../../i18n/LanguageProvider'
import * as api from '../../api/client'

beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('lang', 'fr')
})
afterEach(() => vi.restoreAllMocks())

function renderMeter(token: string, appId: number) {
  return render(<LanguageProvider><QuotaMeter token={token} appId={appId} /></LanguageProvider>)
}

it('renders the bar and approx label when available', async () => {
  vi.spyOn(api, 'getQuota').mockResolvedValue({ hasQuota: true, used: 612, limit: 1000, windowSeconds: 60, available: true })
  renderMeter('jwt', 7)
  expect(await screen.findByText(/612/)).toBeInTheDocument()
  expect(screen.getByText(/1000/)).toBeInTheDocument()
  expect(screen.getByText(/60\s*s/)).toBeInTheDocument()
})

it('shows métriques indisponibles when not available', async () => {
  vi.spyOn(api, 'getQuota').mockResolvedValue({ hasQuota: true, limit: 1000, windowSeconds: 60, available: false })
  renderMeter('jwt', 7)
  expect(await screen.findByText(/indisponibles/i)).toBeInTheDocument()
})

it('renders nothing when the app has no active subscription', async () => {
  vi.spyOn(api, 'getQuota').mockResolvedValue({ hasQuota: false })
  const { container } = renderMeter('jwt', 7)
  await waitFor(() => expect(api.getQuota).toHaveBeenCalled())
  expect(container.textContent).toBe('')
})
