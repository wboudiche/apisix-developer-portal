import { describe, it, expect } from 'vitest'
import { describe as describeEvent, formatRelative } from './activity'
import type { AppEvent } from '../../api/types'
import { translate } from '../../i18n/t'

const NOW = new Date('2026-06-11T12:00:00Z')
const t = (key: string, vars?: Record<string, string | number>) => translate('fr', key, vars)

describe('formatRelative', () => {
  it('renders minutes, hours, days, then an absolute date', () => {
    expect(formatRelative('2026-06-11T11:59:40Z', t, 'fr', NOW)).toBe("à l'instant")
    expect(formatRelative('2026-06-11T11:30:00Z', t, 'fr', NOW)).toBe('il y a 30 min')
    expect(formatRelative('2026-06-11T09:00:00Z', t, 'fr', NOW)).toBe('il y a 3 h')
    expect(formatRelative('2026-06-09T12:00:00Z', t, 'fr', NOW)).toBe('il y a 2 j')
    expect(formatRelative('2026-03-12T00:00:00Z', t, 'fr', NOW)).toBe('12 mars 2026')
  })
  it('returns empty string for an unparseable date', () => {
    expect(formatRelative('not-a-date', t, 'fr', NOW)).toBe('')
  })
})

describe('describe', () => {
  const base = { productName: '', planName: '', createdAt: '2026-06-11T11:30:00Z' }

  it('maps subscribed with product + plan', () => {
    const e: AppEvent = { ...base, kind: 'subscribed', productName: 'Inventory API', planName: 'Gold' }
    expect(describeEvent(e, t, 'fr', NOW)).toMatchObject({ icon: 'check', lead: 'Abonnement', rest: ' à Inventory API · plan Gold' })
  })
  it('maps app_created with no product text', () => {
    expect(describeEvent({ ...base, kind: 'app_created' }, t, 'fr', NOW)).toMatchObject({ icon: 'plus', lead: 'Application créée', rest: '' })
  })
  it('degrades gracefully when the product name is missing (deleted product)', () => {
    expect(describeEvent({ ...base, kind: 'unsubscribed' }, t, 'fr', NOW)).toMatchObject({ icon: 'rotate', lead: 'Désabonnement', rest: '' })
  })
  it('maps approved and rejected', () => {
    expect(describeEvent({ ...base, kind: 'approved', productName: 'Orders API' }, t, 'fr', NOW)).toMatchObject({ icon: 'check', lead: 'Abonnement activé', rest: ' · Orders API' })
    expect(describeEvent({ ...base, kind: 'rejected', productName: 'Orders API' }, t, 'fr', NOW)).toMatchObject({ icon: 'alert', lead: 'Abonnement refusé', rest: ' · Orders API' })
  })
  it('describes a key_rotated event', () => {
    const item = describeEvent({ kind: 'key_rotated', productName: '', planName: '', createdAt: '2026-06-27T10:00:00Z' }, t, 'fr', new Date('2026-06-27T10:01:00Z'))
    expect(item.lead).toMatch(/Clé régénérée/)
    expect(item.icon).toBe('rotate')
  })
})
