import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { AuthShell } from './AuthShell'
import * as api from '../api/client'
import type { Product } from '../api/types'

const product = (id: number, category: string): Product => ({
  id, name: `P${id}`, slug: `p${id}`, category, version: '1.0.0',
  contextPath: `/p${id}`, description: '', tags: [], icon: 'globe', rating: 4, ratingCount: 0,
})

beforeEach(() => vi.restoreAllMocks())

describe('AuthShell', () => {
  it('renders children and the vitrine content', async () => {
    vi.spyOn(api, 'getProducts').mockResolvedValue({ items: [], total: 0, page: 1, pageSize: 20 })
    render(<AuthShell><p>FORM HERE</p></AuthShell>)
    expect(screen.getByText('FORM HERE')).toBeInTheDocument()
    expect(screen.getByText('Vos API, un seul portail.')).toBeInTheDocument()
    // Blueprint v2 feature list
    expect(screen.getByText('Catalogue unifié')).toBeInTheDocument()
    expect(screen.getByText('Clés en libre-service')).toBeInTheDocument()
    expect(screen.getByText('Quotas & abonnements')).toBeInTheDocument()
    expect(await screen.findByText('disponibilité')).toBeInTheDocument()
  })

  it('shows live API and category counts from the catalog', async () => {
    const prods = [product(1, 'Finance'), product(2, 'Finance'), product(3, 'Engineering')]
    vi.spyOn(api, 'getProducts').mockResolvedValue({ items: prods, total: prods.length, page: 1, pageSize: 20 })
    render(<AuthShell><p>x</p></AuthShell>)
    expect(await screen.findByText('3')).toBeInTheDocument()   // 3 APIs
    expect(await screen.findByText('2')).toBeInTheDocument()   // 2 categories
  })

  it('falls back to blueprint numbers when the catalog fetch fails', async () => {
    vi.spyOn(api, 'getProducts').mockRejectedValue(new Error('down'))
    render(<AuthShell><p>x</p></AuthShell>)
    expect(await screen.findByText('9')).toBeInTheDocument()
    expect(await screen.findByText('4')).toBeInTheDocument()
  })
})
