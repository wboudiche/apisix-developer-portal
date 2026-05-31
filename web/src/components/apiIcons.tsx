import type React from 'react'

// Inner SVG markup for each icon key. These are trusted static design constants
// (not user input), so dangerouslySetInnerHTML is acceptable here.
const ICONS: Record<string, string> = {
  seo:      '<path d="M3 17l5-5 4 3 6-7" stroke-linecap="round" stroke-linejoin="round"/><circle cx="18" cy="8" r="0"/><path d="M3 21h18" stroke-linecap="round" opacity=".5"/>',
  reviews:  '<path d="M12 3.5l2.6 5.3 5.9.9-4.3 4.1 1 5.8-5.2-2.7-5.2 2.7 1-5.8L3.5 9.7l5.9-.9z" stroke-linejoin="round"/>',
  stock:    '<path d="M4 19V5M20 19H4" stroke-linecap="round"/><rect x="7" y="11" width="3" height="5" rx="1"/><rect x="13" y="7" width="3" height="9" rx="1"/>',
  test:     '<path d="M9 3h6M10 3v6l-5 8.5A2 2 0 006.7 21h10.6a2 2 0 001.7-3.5L14 9V3" stroke-linejoin="round"/><path d="M8 15h8" stroke-linecap="round"/>',
  keyword:  '<path d="M3 7.5l8-4.5 8 4.5v9l-8 4.5-8-4.5z" stroke-linejoin="round" opacity=".55"/><path d="M14 9.5a2.5 2.5 0 11-3.5 3.5L7 16.5" stroke-linecap="round" stroke-linejoin="round"/>',
  people:   '<circle cx="9" cy="8" r="3"/><path d="M3.5 20a5.5 5.5 0 0111 0" stroke-linecap="round"/><path d="M16 5.5a3 3 0 010 5.5M17 14.5a5.5 5.5 0 013.5 5.5" stroke-linecap="round"/>',
  currency: '<path d="M4 9a8 8 0 0114-3M20 15A8 8 0 016 18" stroke-linecap="round"/><path d="M17 3v3.5h-3.5M7 21v-3.5h3.5" stroke-linecap="round" stroke-linejoin="round"/><path d="M12 8v8M14 10.2c0-1-1-1.7-2.2-1.7s-2 .7-2 1.6c0 2.3 4.4 1.2 4.4 3.5 0 1-1 1.7-2.2 1.7s-2.2-.7-2.2-1.7" stroke-linecap="round"/>',
  phone:    '<rect x="6" y="2.5" width="12" height="19" rx="3"/><path d="M10.5 18.5h3" stroke-linecap="round"/><path d="M9 9.5l2 2 4-4" stroke-linecap="round" stroke-linejoin="round"/>',
  pizza:    '<path d="M12 3c5 0 9 3 9 3L12 21 3 6s4-3 9-3z" stroke-linejoin="round"/><circle cx="10" cy="9" r="1.1" fill="currentColor" stroke="none"/><circle cx="14" cy="10" r="1.1" fill="currentColor" stroke="none"/><circle cx="12" cy="14" r="1.1" fill="currentColor" stroke="none"/>',
}

const ICONS_FALLBACK = '<rect x="4" y="4" width="16" height="16" rx="3"/>'

export function ApiIcon({ name }: { name: string }) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.7}
      dangerouslySetInnerHTML={{ __html: ICONS[name] ?? ICONS_FALLBACK }}
    />
  )
}

interface TintVars {
  '--tint': string
  '--tint-d': string
  '--tint-sh'?: string
  '--tint-line'?: string
}

const CATS: Record<string, TintVars> = {
  Administration: {
    '--tint':      'var(--c-admin)',
    '--tint-d':    'oklch(44% 0.035 262)',
    '--tint-sh':   'oklch(56% 0.035 262 /.35)',
    '--tint-line': 'oklch(56% 0.035 262 /.14)',
  },
  Finance: {
    '--tint':      'var(--c-finance)',
    '--tint-d':    'oklch(42% 0.09 158)',
    '--tint-sh':   'oklch(54% 0.09 158 /.35)',
    '--tint-line': 'oklch(54% 0.09 158 /.14)',
  },
  Marketing: {
    '--tint':      'var(--c-marketing)',
    '--tint-d':    'oklch(46% 0.13 30)',
    '--tint-sh':   'oklch(58% 0.13 30 /.35)',
    '--tint-line': 'oklch(58% 0.13 30 /.14)',
  },
  Engineering: {
    '--tint':      'var(--c-eng)',
    '--tint-d':    'oklch(43% 0.11 288)',
    '--tint-sh':   'oklch(55% 0.11 288 /.35)',
    '--tint-line': 'oklch(55% 0.11 288 /.14)',
  },
}

export function categoryTint(category: string): React.CSSProperties {
  const vars: TintVars = CATS[category] ?? {
    '--tint':   'var(--accent)',
    '--tint-d': 'var(--accent-d)',
  }
  return vars as React.CSSProperties
}

const CAT_DOT_COLORS: Record<string, string> = {
  Administration: 'var(--c-admin)',
  Finance:        'var(--c-finance)',
  Marketing:      'var(--c-marketing)',
  Engineering:    'var(--c-eng)',
}

export function categoryDotColor(category: string | null | undefined): string {
  if (!category) return 'var(--accent)'
  return CAT_DOT_COLORS[category] ?? 'var(--accent)'
}
