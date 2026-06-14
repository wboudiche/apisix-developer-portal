import { describe, it, expect } from 'vitest'
import { formatCount, formatCompact, formatMs, formatPercent, chartHeights } from './usage'

// Whitespace-insensitive: frNum uses locale-specific thousands separators whose
// exact codepoint we don't want to pin tests to.
const ns = (s: string) => s.replace(/\s/g, '')

describe('formatCount', () => {
  it('renders an exact grouped integer', () => {
    expect(ns(formatCount(18402))).toBe('18402')
  })
  it('shows zero for no traffic', () => {
    expect(formatCount(0)).toBe('0')
  })
})

describe('formatCompact', () => {
  it('passes small numbers through', () => {
    expect(formatCompact(500)).toBe('500')
  })
  it('abbreviates thousands with K', () => {
    const out = formatCompact(421000)
    expect(out).toContain('421')
    expect(out).toContain('K')
  })
  it('abbreviates millions with M and one decimal', () => {
    const out = formatCompact(1250000)
    expect(out).toContain('M')
    expect(ns(out)).toContain('1,3')
  })
})

describe('formatMs', () => {
  it('rounds to a whole millisecond', () => {
    expect(formatMs(86.4)).toBe('86')
    expect(formatMs(0)).toBe('0')
  })
})

describe('formatPercent', () => {
  it('converts a 0..1 rate to a percent value', () => {
    expect(ns(formatPercent(0.0021))).toBe('0,21')
    expect(formatPercent(0)).toBe('0')
    expect(formatPercent(0.5)).toBe('50')
  })
})

describe('chartHeights', () => {
  it('scales the largest bar to 100% and others proportionally', () => {
    expect(chartHeights([5, 10, 0])).toEqual([50, 100, 0])
  })
  it('returns all-zero heights when there is no traffic (no divide-by-zero)', () => {
    expect(chartHeights([0, 0])).toEqual([0, 0])
  })
  it('handles an empty series', () => {
    expect(chartHeights([])).toEqual([])
  })
})
