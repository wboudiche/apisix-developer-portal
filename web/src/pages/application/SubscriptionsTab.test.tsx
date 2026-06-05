import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { SubscriptionsTab } from './SubscriptionsTab'
import type { SubscriptionView, Plan } from '../../api/types'

const subs: SubscriptionView[] = [
  { productId: 1, productName: 'Orders API', version: '2.1.0', contextPath: '/orders', planId: 3, planName: 'Gold', status: 'active' },
  { productId: 2, productName: 'Inventory API', version: '1.4.0', contextPath: '/inventory', planId: 1, planName: 'Free', status: 'pending' },
]
const plans: Plan[] = [
  { id: 1, name: 'Free', rateLimit: 60, windowSeconds: 60 },
  { id: 3, name: 'Gold', rateLimit: 1000, windowSeconds: 60 },
]

function setup() {
  const onResiliate = vi.fn()
  render(<MemoryRouter><SubscriptionsTab subs={subs} plans={plans} onResiliate={onResiliate} /></MemoryRouter>)
  return { onResiliate }
}

describe('SubscriptionsTab', () => {
  it('renders one row per subscription with real plan rate and status', () => {
    setup()
    expect(screen.getByText('Orders API')).toBeInTheDocument()
    expect(screen.getByText('/orders · v2.1.0')).toBeInTheDocument()
    expect(screen.getByText('1 000 / min')).toBeInTheDocument()
    expect(screen.getByText('Active')).toBeInTheDocument()
    expect(screen.getByText('En attente')).toBeInTheDocument()
  })
  it('keeps the blueprint Gérer placeholder', () => {
    setup()
    expect(screen.getAllByText('Gérer')).toHaveLength(2)
  })
  it('résilier delegates to the page callback', async () => {
    const { onResiliate } = setup()
    await userEvent.click(screen.getAllByText('Résilier')[0])
    expect(onResiliate).toHaveBeenCalledWith(1, 'Orders API')
  })
  it('shows the empty state when there are no subscriptions', () => {
    render(<MemoryRouter><SubscriptionsTab subs={[]} plans={plans} onResiliate={() => {}} /></MemoryRouter>)
    expect(screen.getByText(/Aucun abonnement/)).toBeInTheDocument()
  })
})
