import { describe, it, expect, vi } from 'vitest'
import { render } from '@testing-library/react'
import { CategoryRail } from './CategoryRail'

const categories = [
  { name: 'Finance', count: 2 },
  { name: 'Engineering', count: 2 },
]

describe('CategoryRail', () => {
  it('renders a .dot inside every .cat button (including "Toutes")', () => {
    const { container } = render(
      <CategoryRail
        categories={categories}
        active={null}
        onPick={vi.fn()}
      />,
    )
    // 2 category buttons + 1 "Toutes les catégories" = 3 total .cat buttons
    const dots = container.querySelectorAll('.cat .dot')
    expect(dots.length).toBe(categories.length + 1)
  })

  it('each .dot has an inline background style', () => {
    const { container } = render(
      <CategoryRail
        categories={categories}
        active={null}
        onPick={vi.fn()}
      />,
    )
    const dots = container.querySelectorAll<HTMLElement>('.cat .dot')
    dots.forEach(dot => {
      expect(dot.getAttribute('style')).toBeTruthy()
    })
  })

  it('renders the .rail-note with the correct total', () => {
    const { container } = render(
      <CategoryRail
        categories={categories}
        active={null}
        onPick={vi.fn()}
      />,
    )
    const note = container.querySelector('.rail-note')
    expect(note).not.toBeNull()
    // total = 2 + 2 = 4
    expect(note?.textContent).toContain('4 APIs publiées')
  })

  it('renders the collapse button with aria-label="Fermer"', () => {
    const { container } = render(
      <CategoryRail
        categories={categories}
        active={null}
        onPick={vi.fn()}
      />,
    )
    const btn = container.querySelector('button[aria-label="Fermer"]')
    expect(btn).not.toBeNull()
  })

  it('the collapse button contains an svg chevron', () => {
    const { container } = render(
      <CategoryRail
        categories={categories}
        active={null}
        onPick={vi.fn()}
      />,
    )
    const btn = container.querySelector('button[aria-label="Fermer"]')
    expect(btn?.querySelector('svg')).not.toBeNull()
  })
})
