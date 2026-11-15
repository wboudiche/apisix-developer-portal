import { minorDigits } from './money'

// Currencies a plan may be priced in. Prices are stored in each currency's
// minor unit (cents, or millimes for TND). Keep in sync with
// SupportedCurrencies in internal/admin/plan.go.
export const CURRENCIES = ['EUR', 'USD', 'GBP', 'CHF', 'CAD', 'AUD', 'SEK', 'NOK', 'DKK', 'PLN', 'MAD', 'TND'] as const

/** "EUR — euro" in the UI language; falls back to the bare code. */
export function currencyName(code: string, lang: 'fr' | 'en'): string {
  try {
    const name = new Intl.DisplayNames([lang], { type: 'currency' }).of(code)
    return name && name !== code ? `${code} — ${name}` : code
  } catch {
    return code
  }
}

/**
 * Parses a human price ("49", "49.9", "49,90", "1 299,00") into the
 * currency's minor unit (cents; millimes for TND). Returns null when the text
 * isn't a non-negative amount with at most as many decimals as the currency
 * allows. Empty text means 0 (free).
 */
export function parsePrice(text: string, currency: string): number | null {
  const digits = minorDigits(currency)
  const s = text.replace(/[\s\u00a0\u202f]/g, '').replace(',', '.')
  if (s === '') return 0
  if (!new RegExp(`^\\d+(\\.\\d{0,${digits}})?$`).test(s)) return null
  const [whole, frac = ''] = s.split('.')
  const minor = Number(whole) * 10 ** digits + Number(frac.padEnd(digits, '0') || '0')
  return Number.isSafeInteger(minor) && minor <= 2_147_483_647 ? minor : null
}

/** Minor units → editable text in the UI language's decimal style ("49,00" / "49.00"; "49,900" for TND). */
export function formatPriceInput(minor: number, currency: string, lang: 'fr' | 'en'): string {
  const digits = minorDigits(currency)
  const s = (minor / 10 ** digits).toFixed(digits)
  return lang === 'fr' ? s.replace('.', ',') : s
}
