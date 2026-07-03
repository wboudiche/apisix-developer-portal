import type { UsageState } from './useUsage'
import type { UsageRange } from '../../api/types'
import { chartHeights, formatCount, formatCompact } from './usage'
import { useT } from '../../i18n/LanguageProvider'

const RANGES: { key: UsageRange; labelKey: string }[] = [
  { key: '24h', labelKey: 'app.rangeLabel24h' },
  { key: '7d', labelKey: 'app.rangeLabel7d' },
  { key: '30d', labelKey: 'app.rangeLabel30d' },
]

const TITLE_KEYS: Record<UsageRange, string> = {
  '24h': 'app.chartTitle24h',
  '7d': 'app.chartTitle7d',
  '30d': 'app.chartTitle30d',
}

// Skeleton bar count while loading — a plausible-looking placeholder.
const SKELETON_BARS = Array.from({ length: 16 })

// UsageChart renders the traffic bar chart with a range selector. Loading shows
// skeleton bars; error shows an explicit unavailable notice; an empty series
// (no traffic in the window) shows a message rather than an empty grid.
export function UsageChart({ state, range, onRange }: { state: UsageState; range: UsageRange; onRange: (r: UsageRange) => void }) {
  const t = useT()
  const series = state.status === 'ready' ? state.usage.series : []
  const heights = chartHeights(series.map(p => p.requests))
  const total = series.reduce((sum, p) => sum + p.requests, 0)

  return (
    <div className="dcard">
      <div className="ch">
        <h3>{t(TITLE_KEYS[range])}</h3>
        <p>{t('app.chartSubtitle')}</p>
        <div className="right">
          <div className="rangesel" role="group" aria-label={t('app.periodAriaLabel')}>
            {RANGES.map(r => (
              <button
                key={r.key}
                className={r.key === range ? 'on' : ''}
                aria-pressed={r.key === range}
                onClick={() => onRange(r.key)}
              >
                {t(r.labelKey)}
              </button>
            ))}
          </div>
        </div>
      </div>
      <div className="cb">
        {state.status === 'error' ? (
          <p className="usage-unavail" role="status">{t('app.metricsUnavailable')}</p>
        ) : state.status === 'loading' ? (
          <div className="chart" aria-busy="true">
            {SKELETON_BARS.map((_, i) => (
              <div className="col" key={i}><div className="bw skeleton" /></div>
            ))}
          </div>
        ) : series.length === 0 ? (
          <p className="usage-empty">{t('app.noTraffic')}</p>
        ) : (
          <>
            <div className="chart">
              {series.map((p, i) => (
                <div className="col" key={p.t} data-testid="chart-col">
                  <div
                    className="bw"
                    data-v={`${formatCount(p.requests)} req${p.errors > 0 ? ` · ${formatCount(p.errors)} err` : ''}`}
                    style={{ height: `${heights[i]}%` }}
                  />
                </div>
              ))}
            </div>
            <p className="chart-cap">{t('app.trafficCaption', { total: formatCompact(total) })}</p>
          </>
        )}
      </div>
    </div>
  )
}
