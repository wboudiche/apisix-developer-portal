// Currencies a plan may be priced in. All use two decimals, matching the
// cents-based storage. Keep in sync with SupportedCurrencies in
// internal/admin/plan.go.
export const CURRENCIES = ['EUR', 'USD', 'GBP', 'CHF', 'CAD', 'AUD', 'SEK', 'NOK', 'DKK', 'PLN', 'MAD'] as const

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
 * Parses a human price ("49", "49.9", "49,90", "1 299,00") into cents.
 * Returns null when the text isn't a non-negative amount with at most two
 * decimals. Empty text means 0 (free).
 */
export function parsePrice(text: string): number | null {
  const s = text.replace(/[\s\u00a0\u202f]/g, '').replace(',', '.')
  if (s === '') return 0
  if (!/^\d+(\.\d{0,2})?$/.test(s)) return null
  const [whole, frac = ''] = s.split('.')
  const cents = Number(whole) * 100 + Number(frac.padEnd(2, '0'))
  return Number.isSafeInteger(cents) && cents <= 2_147_483_647 ? cents : null
}

/** Cents → editable text in the UI language's decimal style ("49,00" / "49.00"). */
export function formatPriceInput(cents: number, lang: 'fr' | 'en'): string {
  const s = (cents / 100).toFixed(2)
  return lang === 'fr' ? s.replace('.', ',') : s
}
