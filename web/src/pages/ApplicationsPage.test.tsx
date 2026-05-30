import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { ApplicationsPage } from './ApplicationsPage'
import { ThemeProvider } from '../theme/ThemeProvider'
import { AuthProvider } from '../auth/AuthProvider'
import * as api from '../api/client'

beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('token', 'tok')
  localStorage.setItem('user', JSON.stringify({ id: 5, email: 'a@b.c', name: '', role: 'developer' }))
  vi.restoreAllMocks()
})

function renderPage() {
  return render(<MemoryRouter><ThemeProvider><AuthProvider><ApplicationsPage /></AuthProvider></ThemeProvider></MemoryRouter>)
}

describe('ApplicationsPage', () => {
  it('lists the user apps and shows a selected app key + subscriptions', async () => {
    vi.spyOn(api, 'getApplications').mockResolvedValue([{ id: 9, name: 'My App', ownerId: 5, description: '', createdAt: '' }])
    vi.spyOn(api, 'getApplicationDetail').mockResolvedValue({
      apiKey: 'KEY-9', consumerUsername: 'app_9',
      subscriptions: [{ productId: 3, productName: 'PizzaShackAPI', version: '1.0.0', contextPath: '/pizzashack', planId: 2, planName: 'Silver', status: 'pending' }],
    })
    renderPage()
    await waitFor(() => expect(screen.getByText('My App')).toBeInTheDocument())
    await waitFor(() => expect(screen.getByText('KEY-9')).toBeInTheDocument())
    expect(screen.getByText('PizzaShackAPI')).toBeInTheDocument()
    expect(screen.getByText(/En attente/i)).toBeInTheDocument()
  })
})
