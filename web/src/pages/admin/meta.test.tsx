import { describe, it, expect } from 'vitest'
import { slugify, catMeta, CAT_META, planRate, planPreview } from './meta'

describe('slugify', () => {
  it('matches the blueprint behavior', () => {
    expect(slugify('CurrencyConverterAPI')).toBe('currencyconverter')
    expect(slugify('  Phone Verification ')).toBe('phone-verification')
    expect(slugify('My-API')).toBe('my')
  })
})

describe('catMeta', () => {
  it('returns the blueprint meta for known categories', () => {
    expect(catMeta('Finance')).toBe(CAT_META.Finance)
    expect(catMeta('Data').color).toBe('var(--c-data)')
  })
  it('falls back deterministically for unknown categories', () => {
    const a = catMeta('Logistique')
    expect(a).toEqual(catMeta('Logistique')) // stable
    expect(a.color).toMatch(/^var\(--c-/)
  })
})

describe('plan rates', () => {
  it('formats sustained rates like the blueprint rows', () => {
    expect(planRate(60, 60)).toBe('≈ 1 req/s soutenu')
    expect(planRate(30, 60)).toBe('≈ 0.50 req/s soutenu')
  })
  it('formats the composer preview', () => {
    expect(planPreview(100, 60)).toBe('≈ 1.7 req/s soutenu')
    expect(planPreview(30, 60)).toBe('≈ 0.50 req/s soutenu')
  })
})
