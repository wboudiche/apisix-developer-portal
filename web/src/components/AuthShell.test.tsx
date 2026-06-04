import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { AuthShell } from './AuthShell'
import * as api from '../api/client'
import type { Product } from '../api/types'

const product = (id: number, category: string): Product => ({
  id, name: `P${id}`, slug: `p${id}`, category, version: '1.0.0',
  contextPath: `/p${id}`, description: '', tags: [], icon: 'globe', rating: 4,
})

beforeEach(() => vi.restoreAllMocks())

describe('AuthShell', () => {
  it('renders children and the vitrine content', async () => {
    vi.spyOn(api, 'getProducts').mockResolvedValue([])
    render(<AuthShell><p>FORM HERE</p></AuthShell>)
    expect(screen.getByText('FORM HERE')).toBeInTheDocument()
    expect(screen.getByText('Vos API, un seul portail.')).toBeInTheDocument()
    expect(await screen.findByText('disponibilité')).toBeInTheDocument()
  })

  it('shows live API and category counts from the catalog', async () => {
    vi.spyOn(api, 'getProducts').mockResolvedValue([
      product(1, 'Finance'), product(2, 'Finance'), product(3, 'Engineering'),
    ])
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
