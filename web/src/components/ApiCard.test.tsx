import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { ApiCard } from './ApiCard'
import { LanguageProvider } from '../i18n/LanguageProvider'
import type { Product } from '../api/types'

beforeEach(() => {
  localStorage.clear()
  // jsdom's navigator.language defaults to 'en-US', which would auto-detect to
  // English; force French so existing assertions (against French strings) hold.
  localStorage.setItem('lang', 'fr')
})

const product: Product = {
  id: 1,
  name: 'PizzaShackAPI',
  slug: 'pizzashackapi',
  category: 'Engineering',
  version: '1.0.0',
  contextPath: '/pizzashack',
  description: 'A pizza ordering API',
  tags: ['pizza', 'food'],
  icon: 'pizza',
  rating: 4.5,
  ratingCount: 3,
}
const baseProduct = product

describe('ApiCard', () => {
  it('renders an SVG icon inside .thumb .ico (not a text monogram)', () => {
    const { container } = render(<MemoryRouter><LanguageProvider><ApiCard p={product} onSubscribe={vi.fn()} /></LanguageProvider></MemoryRouter>)
    const svg = container.querySelector('.thumb .ico svg')
    expect(svg).not.toBeNull()
    // Ensure the old 2-letter monogram text is NOT rendered
    expect(container.querySelector('.thumb .ico')?.textContent).toBe('')
  })

  it('sets the --tint CSS custom property on the card element', () => {
    const { container } = render(<MemoryRouter><LanguageProvider><ApiCard p={product} onSubscribe={vi.fn()} /></LanguageProvider></MemoryRouter>)
    const card = container.querySelector('.card')
    const style = card?.getAttribute('style') ?? ''
    expect(style).toContain('--tint')
  })

  it('renders .crow1 .stars with exactly 5 star SVGs (excluding hidden defs svg)', () => {
    const { container } = render(<MemoryRouter><LanguageProvider><ApiCard p={product} onSubscribe={vi.fn()} /></LanguageProvider></MemoryRouter>)
    const stars = container.querySelector('.crow1 .stars')
    expect(stars).not.toBeNull()
    // The .stars span contains 1 hidden defs svg + 5 visible star svgs
    const starSvgs = Array.from(stars?.querySelectorAll('svg') ?? []).filter(
      s => s.hasAttribute('viewBox') && s.getAttribute('viewBox') === '0 0 24 24'
    )
    expect(starSvgs.length).toBe(5)
  })

  it('subscribe button contains a + icon svg and the text "S\'abonner"', () => {
    const { container } = render(<MemoryRouter><LanguageProvider><ApiCard p={product} onSubscribe={vi.fn()} /></LanguageProvider></MemoryRouter>)
    const btn = container.querySelector('button.subbtn')
    expect(btn).not.toBeNull()
    expect(btn?.querySelector('svg')).not.toBeNull()
    expect(btn?.textContent).toContain("S'abonner")
  })

  it('half-star (rating 4.5): exactly 4 full stars and 1 half-star (url gradient fill)', () => {
    const { container } = render(<MemoryRouter><LanguageProvider><ApiCard p={product} onSubscribe={vi.fn()} /></LanguageProvider></MemoryRouter>)
    // product.rating = 4.5 — filter to the 5 visible star svgs (viewBox="0 0 24 24")
    const starSvgs = Array.from(container.querySelectorAll('.crow1 .stars > svg')).filter(
      s => s.getAttribute('viewBox') === '0 0 24 24'
    )
    expect(starSvgs.length).toBe(5)
    const fullStars = starSvgs.filter(s => s.getAttribute('fill') === 'currentColor')
    const halfStars = starSvgs.filter(s => s.getAttribute('fill') === 'url(#star-half)')
    const emptyStars = starSvgs.filter(s => s.getAttribute('fill') === 'none')
    expect(fullStars.length).toBe(4)
    expect(halfStars.length).toBe(1)
    expect(emptyStars.length).toBe(0)
  })

  it('links the card title to the product detail page', () => {
    const p = { id: 1, name: 'Orders API', slug: 'orders', category: 'Data', version: '1.0.0', contextPath: '/orders', description: '', tags: [], icon: '', rating: 4, ratingCount: 0 }
    render(<MemoryRouter><LanguageProvider><ApiCard p={p} onSubscribe={() => {}} /></LanguageProvider></MemoryRouter>)
    expect(screen.getByRole('link', { name: /Orders API/ })).toHaveAttribute('href', '/catalog/orders')
  })

  it('shows "Pas encore noté" when ratingCount is 0', () => {
    const p = { id: 1, name: 'X', slug: 'x', category: 'C', version: '1', contextPath: '/x', description: '', tags: [], icon: '', rating: 0, ratingCount: 0 }
    render(<MemoryRouter><LanguageProvider><ApiCard p={p} onSubscribe={() => {}} /></LanguageProvider></MemoryRouter>)
    expect(screen.getByText(/Pas encore noté/i)).toBeInTheDocument()
  })

  it('shows an OAuth2 badge for oauth2 products', () => {
    const p = { id: 1, name: 'X', slug: 'x', category: 'C', version: '1', contextPath: '/x', description: '', tags: [], icon: '', rating: 0, ratingCount: 0, authType: 'oauth2' }
    render(<MemoryRouter><LanguageProvider><ApiCard p={p} onSubscribe={() => {}} /></LanguageProvider></MemoryRouter>)
    expect(screen.getByText('OAuth2')).toBeInTheDocument()
  })

  it('shows no OAuth2 badge for key-auth products', () => {
    const p = { id: 1, name: 'X', slug: 'x', category: 'C', version: '1', contextPath: '/x', description: '', tags: [], icon: '', rating: 0, ratingCount: 0, authType: 'key-auth' }
    render(<MemoryRouter><LanguageProvider><ApiCard p={p} onSubscribe={() => {}} /></LanguageProvider></MemoryRouter>)
    expect(screen.queryByText('OAuth2')).not.toBeInTheDocument()
  })

  it('shows a lifecycle badge for a deprecated product', () => {
    // LifecycleBadge is i18n'd (Task 2); force French so this default-locale assertion holds.
    render(<MemoryRouter><LanguageProvider><ApiCard p={{ ...baseProduct, lifecycleStatus: 'deprecated' }} onSubscribe={() => {}} /></LanguageProvider></MemoryRouter>)
    expect(screen.getByText('Déprécié')).toBeInTheDocument()
  })

  it('shows no lifecycle badge for an active product', () => {
    render(<MemoryRouter><LanguageProvider><ApiCard p={{ ...baseProduct, lifecycleStatus: 'active' }} onSubscribe={() => {}} /></LanguageProvider></MemoryRouter>)
    expect(screen.queryByText('Déprécié')).not.toBeInTheDocument()
  })

  it('disables the subscribe button for a deprecated product, with an explanatory title', () => {
    const { container } = render(<MemoryRouter><LanguageProvider><ApiCard p={{ ...baseProduct, lifecycleStatus: 'deprecated' }} onSubscribe={() => {}} /></LanguageProvider></MemoryRouter>)
    const btn = container.querySelector('button.subbtn')
    expect(btn).toBeDisabled()
    expect(btn).toHaveAttribute('title', "Cette API n'accepte plus de nouveaux abonnements")
  })

  it('disables the subscribe button for a sunset product', () => {
    const { container } = render(<MemoryRouter><LanguageProvider><ApiCard p={{ ...baseProduct, lifecycleStatus: 'sunset' }} onSubscribe={() => {}} /></LanguageProvider></MemoryRouter>)
    const btn = container.querySelector('button.subbtn')
    expect(btn).toBeDisabled()
  })

  it('leaves the subscribe button enabled for an active product', () => {
    const { container } = render(<MemoryRouter><LanguageProvider><ApiCard p={{ ...baseProduct, lifecycleStatus: 'active' }} onSubscribe={() => {}} /></LanguageProvider></MemoryRouter>)
    const btn = container.querySelector('button.subbtn')
    expect(btn).not.toBeDisabled()
    expect(btn).not.toHaveAttribute('title')
  })
})
