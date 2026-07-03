import type { ReactElement } from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { UsageChart } from './UsageChart'
import { LanguageProvider } from '../../i18n/LanguageProvider'
import type { Usage } from '../../api/types'

const usage: Usage = {
  summary: { requestsToday: 0, monthToDate: 0, p95Ms: 0, errorRate: 0 },
  series: [
    { t: '2026-06-14T10:00:00Z', requests: 10, errors: 0 },
    { t: '2026-06-14T11:00:00Z', requests: 20, errors: 1 },
  ],
}

beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('lang', 'fr')
})

function renderChart(ui: ReactElement) {
  return render(<LanguageProvider>{ui}</LanguageProvider>)
}

describe('UsageChart', () => {
  it('renders the three range options and marks the active one', () => {
    renderChart(<UsageChart state={{ status: 'loading' }} range="7d" onRange={() => {}} />)
    const active = screen.getByRole('button', { name: '7 j' })
    expect(active.className).toContain('on')
  })

  it('calls onRange when a different range is clicked', async () => {
    const onRange = vi.fn()
    renderChart(<UsageChart state={{ status: 'ready', usage }} range="24h" onRange={onRange} />)
    await userEvent.click(screen.getByRole('button', { name: '30 j' }))
    expect(onRange).toHaveBeenCalledWith('30d')
  })

  it('renders one bar per series point when ready', () => {
    renderChart(<UsageChart state={{ status: 'ready', usage }} range="24h" onRange={() => {}} />)
    expect(screen.getAllByTestId('chart-col')).toHaveLength(2)
  })

  it('shows an empty-state message when there is no traffic', () => {
    const empty: Usage = { summary: usage.summary, series: [] }
    renderChart(<UsageChart state={{ status: 'ready', usage: empty }} range="24h" onRange={() => {}} />)
    expect(screen.getByText(/aucun trafic/i)).toBeInTheDocument()
    expect(screen.queryByTestId('chart-col')).not.toBeInTheDocument()
  })

  it('shows an explicit unavailable state on error', () => {
    renderChart(<UsageChart state={{ status: 'error' }} range="24h" onRange={() => {}} />)
    expect(screen.getByText(/indisponibles?/i)).toBeInTheDocument()
  })
})
