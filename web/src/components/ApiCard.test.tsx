import { describe, it, expect, vi } from 'vitest'
import { render } from '@testing-library/react'
import { ApiCard } from './ApiCard'
import type { Product } from '../api/types'

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
}

describe('ApiCard', () => {
  it('renders an SVG icon inside .thumb .ico (not a text monogram)', () => {
    const { container } = render(<ApiCard p={product} onSubscribe={vi.fn()} />)
    const svg = container.querySelector('.thumb .ico svg')
    expect(svg).not.toBeNull()
    // Ensure the old 2-letter monogram text is NOT rendered
    expect(container.querySelector('.thumb .ico')?.textContent).toBe('')
  })

  it('sets the --tint CSS custom property on the card element', () => {
    const { container } = render(<ApiCard p={product} onSubscribe={vi.fn()} />)
    const card = container.querySelector('.card')
    const style = card?.getAttribute('style') ?? ''
    expect(style).toContain('--tint')
  })
})
