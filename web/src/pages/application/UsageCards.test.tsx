import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { UsageCards } from './UsageCards'
import type { Usage } from '../../api/types'

const usage: Usage = {
  summary: { requestsToday: 18402, monthToDate: 421000, p95Ms: 86, errorRate: 0.0021 },
  series: [],
}

describe('UsageCards', () => {
  it('shows the four card labels in every state', () => {
    render(<UsageCards state={{ status: 'loading' }} />)
    expect(screen.getByText("Requêtes · aujourd'hui")).toBeInTheDocument()
    expect(screen.getByText('Ce mois-ci')).toBeInTheDocument()
    expect(screen.getByText('Latence p95')).toBeInTheDocument()
    expect(screen.getByText("Taux d'erreur")).toBeInTheDocument()
  })

  it('renders skeletons while loading (no numbers yet)', () => {
    render(<UsageCards state={{ status: 'loading' }} />)
    expect(screen.getAllByTestId('stat-skeleton')).toHaveLength(4)
  })

  it('renders the real values when ready', () => {
    render(<UsageCards state={{ status: 'ready', usage }} />)
    expect(screen.getByText(/18\s*402/)).toBeInTheDocument()
    expect(screen.getByText(/421\s*K/)).toBeInTheDocument()
    expect(screen.getByText('86')).toBeInTheDocument()
    expect(screen.getByText(/0,21/)).toBeInTheDocument()
    expect(screen.queryByTestId('stat-skeleton')).not.toBeInTheDocument()
  })

  it('shows an explicit unavailable state on error, never demo numbers', () => {
    render(<UsageCards state={{ status: 'error' }} />)
    expect(screen.getByText(/indisponibles?/i)).toBeInTheDocument()
  })
})
