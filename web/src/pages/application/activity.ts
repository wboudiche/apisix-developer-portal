import type { AppEvent } from '../../api/types'

// FeedIcon names map to the SVG paths in OverviewTab's FEED_ICONS table.
export type FeedIcon = 'check' | 'rotate' | 'alert' | 'plus'

export interface FeedItem {
  icon: FeedIcon
  lead: string
  rest: string
  when: string
}

const MONTHS = ['janv.', 'févr.', 'mars', 'avr.', 'mai', 'juin', 'juil.', 'août', 'sept.', 'oct.', 'nov.', 'déc.']

// formatRelative renders a French relative time, falling back to an absolute
// date past a week. `now` is injectable so the output is testable.
export function formatRelative(iso: string, now: Date = new Date()): string {
  const then = new Date(iso)
  const ms = now.getTime() - then.getTime()
  if (Number.isNaN(ms)) return ''
  const min = Math.floor(ms / 60000)
  if (min < 1) return "à l'instant"
  if (min < 60) return `il y a ${min} min`
  const h = Math.floor(min / 60)
  if (h < 24) return `il y a ${h} h`
  const d = Math.floor(h / 24)
  if (d < 7) return `il y a ${d} j`
  return `${then.getDate()} ${MONTHS[then.getMonth()]} ${then.getFullYear()}`
}

// describe maps an activity event to its feed presentation. Product/plan names
// may be empty (the product was deleted, or the event has no product); the text
// degrades gracefully in that case.
export function describe(e: AppEvent, now: Date = new Date()): FeedItem {
  const when = formatRelative(e.createdAt, now)
  const p = e.productName
  switch (e.kind) {
    case 'app_created':
      return { icon: 'plus', lead: 'Application créée', rest: '', when }
    case 'subscribed':
      return { icon: 'check', lead: 'Abonnement', rest: p ? ` à ${p}${e.planName ? ` · plan ${e.planName}` : ''}` : '', when }
    case 'approved':
      return { icon: 'check', lead: 'Abonnement activé', rest: p ? ` · ${p}` : '', when }
    case 'rejected':
      return { icon: 'alert', lead: 'Abonnement refusé', rest: p ? ` · ${p}` : '', when }
    case 'unsubscribed':
      return { icon: 'rotate', lead: 'Désabonnement', rest: p ? ` de ${p}` : '', when }
    default:
      return { icon: 'plus', lead: 'Activité', rest: '', when }
  }
}
