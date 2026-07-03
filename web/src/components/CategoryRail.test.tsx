import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, fireEvent } from '@testing-library/react'
import type { ComponentProps } from 'react'
import { CategoryRail } from './CategoryRail'
import { LanguageProvider } from '../i18n/LanguageProvider'

beforeEach(() => {
  // jsdom's navigator.language defaults to 'en-US', which would auto-detect to
  // English; force French so existing assertions (against French strings) hold.
  localStorage.setItem('lang', 'fr')
})

function renderRail(props: ComponentProps<typeof CategoryRail>) {
  return render(<LanguageProvider><CategoryRail {...props} /></LanguageProvider>)
}

const categories = [
  { name: 'Finance', count: 2 },
  { name: 'Engineering', count: 2 },
]

describe('CategoryRail', () => {
  it('renders a .dot inside every .cat button (including "Toutes")', () => {
    const { container } = renderRail({
      categories,
      active: null,
      onPick: vi.fn(),
    })
    // 2 category buttons + 1 "Toutes les catégories" = 3 total .cat buttons
    const dots = container.querySelectorAll('.cat .dot')
    expect(dots.length).toBe(categories.length + 1)
  })

  it('each .dot has an inline background style', () => {
    const { container } = renderRail({
      categories,
      active: null,
      onPick: vi.fn(),
    })
    const dots = container.querySelectorAll<HTMLElement>('.cat .dot')
    dots.forEach(dot => {
      expect(dot.getAttribute('style')).toBeTruthy()
    })
  })

  it('renders the .rail-note with the correct total', () => {
    const { container } = renderRail({
      categories,
      active: null,
      onPick: vi.fn(),
    })
    const note = container.querySelector('.rail-note')
    expect(note).not.toBeNull()
    // total = 2 + 2 = 4
    expect(note?.textContent).toContain('4 APIs publiées')
  })

  it('renders the collapse button with aria-label="Fermer"', () => {
    const { container } = renderRail({
      categories,
      active: null,
      onPick: vi.fn(),
    })
    const btn = container.querySelector('button[aria-label="Fermer"]')
    expect(btn).not.toBeNull()
  })

  it('the collapse button contains an svg chevron', () => {
    const { container } = renderRail({
      categories,
      active: null,
      onPick: vi.fn(),
    })
    const btn = container.querySelector('button[aria-label="Fermer"]')
    expect(btn?.querySelector('svg')).not.toBeNull()
  })

  // ── NEW: open/closed class ─────────────────────────────────────────────────

  it('adds "open" class to .rail when open=true', () => {
    const { container } = renderRail({
      categories,
      active: null,
      onPick: vi.fn(),
      open: true,
    })
    const rail = container.querySelector('.rail')
    expect(rail?.classList.contains('open')).toBe(true)
    expect(rail?.classList.contains('closed')).toBe(false)
  })

  it('adds "closed" class to .rail when open=false', () => {
    const { container } = renderRail({
      categories,
      active: null,
      onPick: vi.fn(),
      open: false,
    })
    const rail = container.querySelector('.rail')
    expect(rail?.classList.contains('closed')).toBe(true)
    expect(rail?.classList.contains('open')).toBe(false)
  })

  it('defaults to open class (open prop omitted)', () => {
    const { container } = renderRail({
      categories,
      active: null,
      onPick: vi.fn(),
    })
    const rail = container.querySelector('.rail')
    expect(rail?.classList.contains('open')).toBe(true)
  })

  // ── NEW: collapse button calls onClose ─────────────────────────────────────

  it('collapse button calls onClose when clicked', () => {
    const onClose = vi.fn()
    const { container } = renderRail({
      categories,
      active: null,
      onPick: vi.fn(),
      onClose,
    })
    const btn = container.querySelector('button[aria-label="Fermer"]') as HTMLButtonElement
    fireEvent.click(btn)
    expect(onClose).toHaveBeenCalledTimes(1)
  })
})
