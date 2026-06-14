import { frNum } from './helpers'

// Formatting + scaling helpers for the usage cards and traffic chart. All take
// already-fetched numbers (the API does the aggregation) and are pure so they
// can be unit-tested without rendering.

/** Exact grouped integer, e.g. 18402 → "18 402". */
export function formatCount(n: number): string {
  if (!Number.isFinite(n) || n <= 0) return '0'
  return frNum(Math.round(n))
}

/** Compact integer for high-volume cards: 421000 → "421 K", 1.25M → "1,3 M". */
export function formatCompact(n: number): string {
  if (!Number.isFinite(n) || n <= 0) return '0'
  if (n < 1000) return String(Math.round(n))
  if (n < 1_000_000) return `${frNum(Math.round(n / 1000))} K`
  return `${frNum(Math.round(n / 100_000) / 10)} M`
}

/** Whole-millisecond latency, e.g. 86.4 → "86". */
export function formatMs(ms: number): string {
  if (!Number.isFinite(ms) || ms <= 0) return '0'
  return String(Math.round(ms))
}

/** 0..1 error rate → percent value (no unit), e.g. 0.0021 → "0,21". */
export function formatPercent(rate: number): string {
  if (!Number.isFinite(rate) || rate <= 0) return '0'
  return frNum(Math.round(rate * 100 * 100) / 100)
}

/** Bar heights in percent, largest bar = 100; all-zero input → all zeros. */
export function chartHeights(values: number[]): number[] {
  const max = Math.max(0, ...values)
  if (max <= 0) return values.map(() => 0)
  return values.map(v => Math.round((Math.max(0, v) / max) * 100))
}
