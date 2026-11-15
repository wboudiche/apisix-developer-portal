import { describe, it, expect } from 'vitest'
import { parsePrice, formatPriceInput, currencyName } from './currencies'

describe('parsePrice', () => {
  it.each([
    ['', 0], ['0', 0], ['49', 4900], ['49.9', 4990], ['49,90', 4990], ['49.', 4900],
    ['1 299,00', 129900], ['1 299,5', 129950], [' 7 ', 700],
  ])('%j → %d cents', (text, cents) => expect(parsePrice(text)).toBe(cents))

  it.each(['-1', '49,999', 'abc', '4e3', '1.2.3', '.5', '99999999999'])('rejects %j', text =>
    expect(parsePrice(text)).toBeNull())
})

describe('formatPriceInput', () => {
  it('uses the language decimal separator', () => {
    expect(formatPriceInput(4990, 'fr')).toBe('49,90')
    expect(formatPriceInput(4990, 'en')).toBe('49.90')
  })
})

describe('currencyName', () => {
  it('prefixes the code to the localized name', () => {
    expect(currencyName('EUR', 'en')).toBe('EUR — Euro')
    expect(currencyName('EUR', 'fr')).toMatch(/^EUR — euro/i)
  })
})
