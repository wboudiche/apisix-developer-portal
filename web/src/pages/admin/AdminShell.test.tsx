import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { AdminShell } from './AdminShell'
import { AuthProvider } from '../../auth/AuthProvider'
import * as api from '../../api/client'

beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('token', 'jwt')
  localStorage.setItem('user', JSON.stringify({ id: 1, email: 'a@b.c', name: 'Admin', role: 'admin' }))
  vi.restoreAllMocks()
  vi.spyOn(api, 'adminGetProducts').mockResolvedValue([{ name: 'P', slug: 'p', category: '', version: '', contextPath: '/p', description: '', tags: [], icon: '', upstreamUrl: '', published: true }])
  vi.spyOn(api, 'adminGetPlans').mockResolvedValue([])
  vi.spyOn(api, 'adminGetSubscriptions').mockResolvedValue([
    { id: 1, applicationName: 'A', ownerEmail: 'a@b.c', productName: 'P', version: '1', planName: 'Free', status: 'pending', createdAt: '2026-06-06T00:00:00Z' },
    { id: 2, applicationName: 'B', ownerEmail: 'b@b.c', productName: 'P', version: '1', planName: 'Free', status: 'pending', createdAt: '2026-06-06T00:00:00Z' },
  ])
})

function renderShell(counts?: { products?: number; plans?: number; pending?: number }) {
  return render(
    <MemoryRouter>
      <AuthProvider>
        <AdminShell active="products" title="Produits" description="desc" counts={counts}>
          <p>CHILD</p>
        </AdminShell>
      </AuthProvider>
    </MemoryRouter>
  )
}

describe('AdminShell', () => {
  it('renders nav, head and children with fetched counts', async () => {
    renderShell()
    expect(screen.getByText('CHILD')).toBeInTheDocument()
    expect(screen.getByRole('heading', { level: 1, name: 'Produits' })).toBeInTheDocument()
    const prodLink = await screen.findByRole('link', { name: /Produits/ })
    expect(prodLink).toHaveClass('active')
    expect(await screen.findByText('2')).toBeInTheDocument()   // pending badge
  })
  it('a provided count overrides fetching for that tab', async () => {
    renderShell({ products: 9 })
    expect(await screen.findByText('9')).toBeInTheDocument()
    expect(api.adminGetProducts).not.toHaveBeenCalled()
  })
})
