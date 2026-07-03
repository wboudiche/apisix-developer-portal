import { describe, it, expect } from 'vitest'
import { appRef, initials, maskKey, rateLabel, statusPill, subsCountKey, formatDate } from './helpers'
import { translate } from '../../i18n/t'

const t = (key: string, vars?: Record<string, string | number>) => translate('fr', key, vars)

describe('helpers', () => {
  it('formats the app reference', () => expect(appRef(7)).toBe('app_7'))
  it('derives initials from one or two words', () => {
    expect(initials('Boutique Mobile')).toBe('BM')
    expect(initials('analytics')).toBe('A')
    expect(initials('  ')).toBe('?')
  })
  it('masks a key keeping first 8 and last 2 chars', () => {
    expect(maskKey('ax_live_a3f9c1e7b240')).toBe('ax_live_' + '•'.repeat(10) + '40')
    expect(maskKey('short')).toBe('short')
  })
  it('labels plan rates, minute window as /min', () => {
    expect(rateLabel({ id: 1, name: 'Gold', rateLimit: 1000, windowSeconds: 60 })).toBe('1 000 / min')
    expect(rateLabel({ id: 2, name: 'X', rateLimit: 50, windowSeconds: 10 })).toBe('50 / 10s')
    expect(rateLabel(undefined)).toBe('—')
  })
  it('maps subscription status to pill class/label', () => {
    expect(statusPill('active', t)).toEqual({ cls: 'ok', label: 'Active' })
    expect(statusPill('pending', t)).toEqual({ cls: 'warn', label: 'En attente' })
    expect(statusPill('rejected', t)).toEqual({ cls: 'muted', label: 'Rejeté' })
  })
  it('treats 0 and 1 as singular for the subscription-count key', () => {
    expect(t(subsCountKey(0), { count: 0 })).toBe('0 abonnement')
    expect(t(subsCountKey(1), { count: 1 })).toBe('1 abonnement')
    expect(t(subsCountKey(2), { count: 2 })).toBe('2 abonnements')
  })
  it('formats dates in french and tolerates garbage', () => {
    expect(formatDate('2026-03-12T10:00:00Z')).toMatch(/mars/)
    expect(formatDate('nope')).toBe('—')
  })
  it('formats dates in english when lang="en"', () => {
    expect(formatDate('2026-03-12T10:00:00Z', 'en')).toMatch(/March/)
    expect(formatDate('nope', 'en')).toBe('—')
  })
})
