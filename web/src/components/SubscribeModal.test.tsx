import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SubscribeModal } from './SubscribeModal'
import { AuthProvider } from '../auth/AuthProvider'
import * as api from '../api/client'
import type { Product } from '../api/types'

const product: Product = { id: 3, name: 'PizzaShackAPI', slug: 'pizzashackapi', category: 'Engineering', version: '1.0.0', contextPath: '/pizzashack', description: 'demo', tags: [], icon: 'pi', rating: 4.5 }

beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('token', 'tok')
  localStorage.setItem('user', JSON.stringify({ id: 5, email: 'a@b.c', name: '', role: 'developer' }))
  vi.restoreAllMocks()
})

function renderModal() {
  return render(<AuthProvider><SubscribeModal product={product} onClose={() => {}} /></AuthProvider>)
}

describe('SubscribeModal', () => {
  it('loads apps + plans, subscribes, and shows the issued key', async () => {
    vi.spyOn(api, 'getApplications').mockResolvedValue([{ id: 9, name: 'My App', ownerId: 5, description: '', createdAt: '' }])
    vi.spyOn(api, 'getPlans').mockResolvedValue([{ id: 2, name: 'Silver', rateLimit: 300, windowSeconds: 60 }])
    const sub = vi.spyOn(api, 'subscribe').mockResolvedValue({ applicationId: 9, apiKey: 'SECRET-KEY', consumerUsername: 'app_9' })

    renderModal()
    await waitFor(() => expect(screen.getByText('My App')).toBeInTheDocument())
    await userEvent.click(screen.getByRole('button', { name: /s'abonner|subscribe|confirmer/i }))

    await waitFor(() => expect(screen.getByText('SECRET-KEY')).toBeInTheDocument())
    expect(sub).toHaveBeenCalledWith('tok', 9, 3, 2)
  })

  it('shows the server error when subscribe fails', async () => {
    vi.spyOn(api, 'getApplications').mockResolvedValue([{ id: 9, name: 'My App', ownerId: 5, description: '', createdAt: '' }])
    vi.spyOn(api, 'getPlans').mockResolvedValue([{ id: 2, name: 'Silver', rateLimit: 300, windowSeconds: 60 }])
    vi.spyOn(api, 'subscribe').mockRejectedValue(new Error('provisioning failed'))
    renderModal()
    await waitFor(() => expect(screen.getByText('My App')).toBeInTheDocument())
    await userEvent.click(screen.getByRole('button', { name: /s'abonner|subscribe|confirmer/i }))
    await waitFor(() => expect(screen.getByText('provisioning failed')).toBeInTheDocument())
  })
})
