import { useLang } from './i18n/LanguageProvider'

/**
 * Number of decimals in the currency's minor unit (EUR → 2 for cents,
 * TND → 3 for millimes). Prices (`priceCents`) are stored as an integer count
 * of that minor unit, so this is the scale between storage and display.
 */
export function minorDigits(currency: string): number {
  try {
    return new Intl.NumberFormat('en', { style: 'currency', currency }).resolvedOptions().maximumFractionDigits ?? 2
  } catch {
    return 2
  }
}

/** Formats an amount given in the currency's minor unit (see minorDigits). */
export function formatMoney(minor: number, currency: string, lang: 'fr' | 'en'): string {
  return new Intl.NumberFormat(lang === 'en' ? 'en-US' : 'fr-FR', {
    style: 'currency',
    currency,
  }).format(minor / 10 ** minorDigits(currency))
}

// priceLabel renders a plan price: the free label when cents===0, else the
// formatted amount plus an optional per-period suffix. Callers pass the i18n
// strings so this stays framework-agnostic.
export function priceLabel(cents: number, currency: string, lang: 'fr' | 'en', freeLabel: string, perSuffix = ''): string {
  return cents === 0 ? freeLabel : formatMoney(cents, currency, lang) + perSuffix
}

export function useFormatMoney() {
  const { lang } = useLang()
  return (cents: number, currency: string) => formatMoney(cents, currency, lang)
}
