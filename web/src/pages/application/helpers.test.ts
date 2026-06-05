import { describe, it, expect } from 'vitest'
import { appRef, initials, maskKey, rateLabel, statusPill, frDate } from './helpers'

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
    expect(statusPill('active')).toEqual({ cls: 'ok', label: 'Active' })
    expect(statusPill('pending')).toEqual({ cls: 'warn', label: 'En attente' })
    expect(statusPill('rejected')).toEqual({ cls: 'muted', label: 'Rejeté' })
  })
  it('formats dates in french and tolerates garbage', () => {
    expect(frDate('2026-03-12T10:00:00Z')).toMatch(/mars/)
    expect(frDate('nope')).toBe('—')
  })
})
