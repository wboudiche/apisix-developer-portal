import { fr } from './fr'
import { en } from './en'
import type { Messages } from './fr'

export type Lang = 'fr' | 'en'
const catalogs: Record<Lang, Messages> = { fr, en }

function lookup(cat: Messages, key: string): string | undefined {
  return key.split('.').reduce<unknown>((o, k) => (o && typeof o === 'object' ? (o as Record<string, unknown>)[k] : undefined), cat) as string | undefined
}

export function translate(lang: Lang, key: string, vars?: Record<string, string | number>): string {
  // plural: if a `count`/`n` var is present and `${key}_one|_other` exist, pick by it
  const n = vars?.n ?? vars?.count
  let raw: string | undefined
  if (typeof n === 'number') {
    const suffix = n === 1 ? '_one' : '_other'
    raw = lookup(catalogs[lang], key + suffix) ?? lookup(fr, key + suffix)
  }
  raw = raw ?? lookup(catalogs[lang], key) ?? lookup(fr, key)
  if (raw === undefined) {
    if (import.meta.env?.DEV) console.warn(`[i18n] missing key: ${key}`)
    return key
  }
  return vars ? raw.replace(/\{(\w+)\}/g, (_, k) => String(vars[k] ?? `{${k}}`)) : raw
}
