import { DEMO_CHART, DEMO_USAGE_ROWS } from './demo'
import { frNum } from './helpers'

// Entirely DEMO — no metrics pipeline yet (see demo.ts).
export function UsageTab() {
  const max = Math.max(...DEMO_CHART.values)
  return (
    <section className="panel">
      <div className="dcard">
        <div className="ch">
          <div>
            <h3>Requêtes · 14 derniers jours</h3>
            <p>Toutes API confondues, environnement production.</p>
          </div>
          <div className="right"><span className="stpill muted"><span className="led" />421 K ce mois</span></div>
        </div>
        <div className="cb">
          <div className="chart">
            {DEMO_CHART.values.map((v, i) => (
              <div className="col" key={i} data-testid="chart-col">
                <div className="bw" data-v={`${frNum(v * 1000)} req`} style={{ height: `${Math.round((v / max) * 100)}%` }} />
                <small>{DEMO_CHART.labels[i]}</small>
              </div>
            ))}
          </div>
        </div>
      </div>

      <div className="dcard" style={{ marginTop: 20 }}>
        <div className="ch"><h3>Répartition par API</h3></div>
        <div className="cb" style={{ padding: 0 }}>
          <table className="tbl">
            <thead><tr><th>API</th><th>Requêtes (mois)</th><th>Part</th><th>Erreurs</th></tr></thead>
            <tbody>
              {DEMO_USAGE_ROWS.map(r => (
                <tr key={r.name}>
                  <td><div className="apicell"><span className="ig" style={{ background: r.bg }}>{r.ini}</span><span className="nm">{r.name}</span></div></td>
                  <td className="mono">{r.requests}</td>
                  <td><div className="bar"><i style={{ width: `${r.share}%` }} /></div></td>
                  <td className="mono" style={{ color: r.errColor }}>{r.errors}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </section>
  )
}
