import { describe, it, expect, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { UsageCards } from './UsageCards'
import { LanguageProvider } from '../../i18n/LanguageProvider'
import type { Usage } from '../../api/types'

const usage: Usage = {
  summary: { requestsToday: 18402, monthToDate: 421000, p95Ms: 86, errorRate: 0.0021 },
  series: [],
}

beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('lang', 'fr')
})

function renderCards(state: Parameters<typeof UsageCards>[0]['state']) {
  return render(<LanguageProvider><UsageCards state={state} /></LanguageProvider>)
}

describe('UsageCards', () => {
  it('shows the four card labels in every state', () => {
    renderCards({ status: 'loading' })
    expect(screen.getByText("Requêtes · aujourd'hui")).toBeInTheDocument()
    expect(screen.getByText('Ce mois-ci')).toBeInTheDocument()
    expect(screen.getByText('Latence p95')).toBeInTheDocument()
    expect(screen.getByText("Taux d'erreur")).toBeInTheDocument()
  })

  it('renders skeletons while loading (no numbers yet)', () => {
    renderCards({ status: 'loading' })
    expect(screen.getAllByTestId('stat-skeleton')).toHaveLength(4)
  })

  it('renders the real values when ready', () => {
    renderCards({ status: 'ready', usage })
    expect(screen.getByText(/18\s*402/)).toBeInTheDocument()
    expect(screen.getByText(/421\s*K/)).toBeInTheDocument()
    expect(screen.getByText('86')).toBeInTheDocument()
    expect(screen.getByText(/0,21/)).toBeInTheDocument()
    expect(screen.queryByTestId('stat-skeleton')).not.toBeInTheDocument()
  })

  it('shows an explicit unavailable state on error, never demo numbers', () => {
    renderCards({ status: 'error' })
    expect(screen.getByText(/indisponibles?/i)).toBeInTheDocument()
  })
})
