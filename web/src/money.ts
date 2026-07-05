import { useLang } from './i18n/LanguageProvider'

export function formatMoney(cents: number, currency: string, lang: 'fr' | 'en'): string {
  return new Intl.NumberFormat(lang === 'en' ? 'en-US' : 'fr-FR', {
    style: 'currency',
    currency,
  }).format(cents / 100)
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
