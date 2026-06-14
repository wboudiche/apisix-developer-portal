import type { UsageState } from './useUsage'
import type { UsageRange } from '../../api/types'
import { chartHeights, formatCount, formatCompact } from './usage'

const RANGES: { key: UsageRange; label: string }[] = [
  { key: '24h', label: '24 h' },
  { key: '7d', label: '7 j' },
  { key: '30d', label: '30 j' },
]

const TITLES: Record<UsageRange, string> = {
  '24h': 'Trafic · dernières 24 h',
  '7d': 'Trafic · 7 derniers jours',
  '30d': 'Trafic · 30 derniers jours',
}

// Skeleton bar count while loading — a plausible-looking placeholder.
const SKELETON_BARS = Array.from({ length: 16 })

// UsageChart renders the traffic bar chart with a range selector. Loading shows
// skeleton bars; error shows an explicit unavailable notice; an empty series
// (no traffic in the window) shows a message rather than an empty grid.
export function UsageChart({ state, range, onRange }: { state: UsageState; range: UsageRange; onRange: (r: UsageRange) => void }) {
  const series = state.status === 'ready' ? state.usage.series : []
  const heights = chartHeights(series.map(p => p.requests))
  const total = series.reduce((sum, p) => sum + p.requests, 0)

  return (
    <div className="dcard">
      <div className="ch">
        <h3>{TITLES[range]}</h3>
        <p>Toutes API confondues, environnement production.</p>
        <div className="right">
          <div className="rangesel" role="group" aria-label="Période">
            {RANGES.map(r => (
              <button
                key={r.key}
                className={r.key === range ? 'on' : ''}
                aria-pressed={r.key === range}
                onClick={() => onRange(r.key)}
              >
                {r.label}
              </button>
            ))}
          </div>
        </div>
      </div>
      <div className="cb">
        {state.status === 'error' ? (
          <p className="usage-unavail" role="status">Métriques indisponibles pour le moment.</p>
        ) : state.status === 'loading' ? (
          <div className="chart" aria-busy="true">
            {SKELETON_BARS.map((_, i) => (
              <div className="col" key={i}><div className="bw skeleton" /></div>
            ))}
          </div>
        ) : series.length === 0 ? (
          <p className="usage-empty">Aucun trafic sur cette période.</p>
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
            <p className="chart-cap">{formatCompact(total)} requêtes sur la période.</p>
          </>
        )}
      </div>
    </div>
  )
}
