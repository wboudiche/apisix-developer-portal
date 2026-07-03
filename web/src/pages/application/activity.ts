import type { AppEvent } from '../../api/types'
import type { Lang } from '../../i18n/t'
import { formatDate } from './helpers'

// FeedIcon names map to the SVG paths in OverviewTab's FEED_ICONS table.
export type FeedIcon = 'check' | 'rotate' | 'alert' | 'plus'

export interface FeedItem {
  icon: FeedIcon
  lead: string
  rest: string
  when: string
}

type TFunc = (key: string, vars?: Record<string, string | number>) => string

// formatRelative renders a localized relative time, falling back to an absolute
// date past a week. `now` is injectable so the output is testable.
export function formatRelative(iso: string, t: TFunc, lang: Lang, now: Date = new Date()): string {
  const then = new Date(iso)
  const ms = now.getTime() - then.getTime()
  if (Number.isNaN(ms)) return ''
  const min = Math.floor(ms / 60000)
  if (min < 1) return t('activity.justNow')
  if (min < 60) return t('activity.minutesAgo', { n: min })
  const h = Math.floor(min / 60)
  if (h < 24) return t('activity.hoursAgo', { n: h })
  const d = Math.floor(h / 24)
  if (d < 7) return t('activity.daysAgo', { n: d })
  return formatDate(iso, lang)
}

// describe maps an activity event to its feed presentation. Product/plan names
// may be empty (the product was deleted, or the event has no product); the text
// degrades gracefully in that case.
export function describe(e: AppEvent, t: TFunc, lang: Lang, now: Date = new Date()): FeedItem {
  const when = formatRelative(e.createdAt, t, lang, now)
  const p = e.productName
  switch (e.kind) {
    case 'app_created':
      return { icon: 'plus', lead: t('activity.appCreated'), rest: '', when }
    case 'subscribed':
      return {
        icon: 'check',
        lead: t('activity.subscribed'),
        rest: p ? t('activity.subscribedTo', { product: p }) + (e.planName ? t('activity.planSuffix', { plan: e.planName }) : '') : '',
        when,
      }
    case 'approved':
      return { icon: 'check', lead: t('activity.approved'), rest: p ? t('activity.productSuffix', { product: p }) : '', when }
    case 'rejected':
      return { icon: 'alert', lead: t('activity.rejected'), rest: p ? t('activity.productSuffix', { product: p }) : '', when }
    case 'unsubscribed':
      return { icon: 'rotate', lead: t('activity.unsubscribed'), rest: p ? t('activity.unsubscribedFrom', { product: p }) : '', when }
    case 'key_rotated':
      return { icon: 'rotate', lead: t('activity.keyRotated'), rest: '', when }
    default:
      return { icon: 'plus', lead: t('activity.generic'), rest: '', when }
  }
}
