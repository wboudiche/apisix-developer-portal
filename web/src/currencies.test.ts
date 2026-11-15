import { describe, it, expect } from 'vitest'
import { parsePrice, formatPriceInput, currencyName } from './currencies'
import { formatMoney, minorDigits } from './money'

describe('parsePrice (EUR, 2 decimals)', () => {
  it.each([
    ['', 0], ['0', 0], ['49', 4900], ['49.9', 4990], ['49,90', 4990], ['49.', 4900],
    ['1 299,00', 129900], ['1 299,5', 129950], [' 7 ', 700],
  ])('%j → %d cents', (text, cents) => expect(parsePrice(text, 'EUR')).toBe(cents))

  it.each(['-1', '49,999', 'abc', '4e3', '1.2.3', '.5', '99999999999'])('rejects %j', text =>
    expect(parsePrice(text, 'EUR')).toBeNull())
})

describe('parsePrice (TND, 3 decimals)', () => {
  it.each([['49', 49000], ['49,9', 49900], ['49,999', 49999], ['0,005', 5]])('%j → %d millimes', (text, m) =>
    expect(parsePrice(text, 'TND')).toBe(m))
  it('rejects a fourth decimal', () => expect(parsePrice('49,9999', 'TND')).toBeNull())
})

describe('formatPriceInput', () => {
  it('uses the language decimal separator and the currency scale', () => {
    expect(formatPriceInput(4990, 'EUR', 'fr')).toBe('49,90')
    expect(formatPriceInput(4990, 'EUR', 'en')).toBe('49.90')
    expect(formatPriceInput(49900, 'TND', 'fr')).toBe('49,900')
  })
})

describe('formatMoney', () => {
  it('scales by the currency minor unit', () => {
    expect(minorDigits('EUR')).toBe(2)
    expect(minorDigits('TND')).toBe(3)
    expect(formatMoney(4990, 'EUR', 'en')).toBe('€49.90')
    expect(formatMoney(49900, 'TND', 'en')).toMatch(/^TND\s49\.900$/)
  })
})

describe('currencyName', () => {
  it('prefixes the code to the localized name', () => {
    expect(currencyName('EUR', 'en')).toBe('EUR — Euro')
    expect(currencyName('TND', 'fr')).toMatch(/^TND — dinar tunisien/i)
  })
})
