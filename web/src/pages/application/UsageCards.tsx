import type { UsageState } from './useUsage'
import type { UsageSummary } from '../../api/types'
import { formatCount, formatCompact, formatMs, formatPercent } from './usage'
import { useT } from '../../i18n/LanguageProvider'

const STAT_ICONS: Record<string, string> = {
  pulse: 'M3 12h4l3 8 4-16 3 8h4',
  calendar: 'M3 9h18M8 4v16M3 4h18v16H3z',
  clock: 'M12 7v5l3 2M12 21a9 9 0 100-18 9 9 0 000 18z',
  alert: 'M12 9v4M12 17h.01M10.3 4.3L2.5 18a2 2 0 001.7 3h15.6a2 2 0 001.7-3L13.7 4.3a2 2 0 00-3.4 0z',
}

type Card = { icon: keyof typeof STAT_ICONS; labelKey: string; unit?: string; value: (s: UsageSummary) => string }

const CARDS: Card[] = [
  { icon: 'pulse', labelKey: 'app.statToday', value: s => formatCount(s.requestsToday) },
  { icon: 'calendar', labelKey: 'app.statMonth', value: s => formatCompact(s.monthToDate) },
  { icon: 'clock', labelKey: 'app.statP95', unit: 'ms', value: s => formatMs(s.p95Ms) },
  { icon: 'alert', labelKey: 'app.statErrorRate', unit: '%', value: s => formatPercent(s.errorRate) },
]

// UsageCards renders the four Overview stat cards from fetched metrics. It shows
// a skeleton while loading and an explicit unavailable notice on error — never a
// demo/placeholder number, which would read as real data.
export function UsageCards({ state }: { state: UsageState }) {
  const t = useT()
  return (
    <>
      {state.status === 'error' && (
        <p className="usage-unavail" role="status">
          {t('app.metricsUnavailable')}
        </p>
      )}
      <div className="stats" aria-busy={state.status === 'loading'}>
        {CARDS.map(c => (
          <div className="stat" key={c.labelKey}>
            <div className="k">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} aria-hidden="true"><path d={STAT_ICONS[c.icon]} strokeLinecap="round" strokeLinejoin="round" /></svg>
              {t(c.labelKey)}
            </div>
            {state.status === 'ready' ? (
              <div className="v">{c.value(state.usage.summary)}{c.unit && <> <small>{c.unit}</small></>}</div>
            ) : state.status === 'error' ? (
              <div className="v muted">—</div>
            ) : (
              <div className="v"><span className="skeleton" data-testid="stat-skeleton" aria-hidden="true" /></div>
            )}
          </div>
        ))}
      </div>
    </>
  )
}
