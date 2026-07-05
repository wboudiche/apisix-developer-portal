import { describe, it, expect } from 'vitest'
import { formatMoney, priceLabel } from './money'

describe('money', () => {
  it('formats cents as localized currency', () => {
    expect(formatMoney(2900, 'EUR', 'en')).toBe('€29.00')
    // fr-FR uses a non-breaking space + comma; assert the pieces to avoid whitespace flakiness
    const fr = formatMoney(2900, 'EUR', 'fr')
    expect(fr).toMatch(/29,00/)
    expect(fr).toContain('€')
  })
  it('priceLabel returns the free label for 0', () => {
    expect(priceLabel(0, 'EUR', 'fr', 'Gratuit', '/mois')).toBe('Gratuit')
  })
  it('priceLabel appends the per-suffix for a nonzero price', () => {
    expect(priceLabel(2900, 'EUR', 'en', 'Free', '/mo')).toBe('€29.00/mo')
  })
})
