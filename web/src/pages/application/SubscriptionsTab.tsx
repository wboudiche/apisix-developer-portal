import { Link } from 'react-router-dom'
import type { SubscriptionView, Plan } from '../../api/types'
import { initials, rateLabel, statusPill } from './helpers'
import { demoBarWidth, demoRpm } from './demo'

const IG_COLORS = ['var(--c-marketing)', 'var(--c-finance)', 'var(--c-eng)', 'var(--c-admin)']
const PLAN_DOTS: Record<string, string> = { Gold: 'var(--warn)', Silver: 'var(--c-admin)', Free: 'var(--c-finance)' }

function PlusIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} aria-hidden="true">
      <path d="M12 5v14M5 12h14" strokeLinecap="round" />
    </svg>
  )
}

export function SubscriptionsTab({ subs, plans, onResiliate }: {
  subs: SubscriptionView[]
  plans: Plan[]
  onResiliate: (productId: number, productName: string) => void
}) {
  return (
    <section className="panel">
      <div className="dcard">
        <div className="ch">
          <div>
            <h3>API abonnées</h3>
            <p>Chaque abonnement lie cette application à une API, à un palier de débit.</p>
          </div>
          <div className="right">
            <Link className="btn primary sm" to="/"><PlusIcon />Abonner une API</Link>
          </div>
        </div>
        <div className="cb" style={{ padding: 0 }}>
          {subs.length === 0 ? (
            <p style={{ padding: 20, fontSize: 14, color: 'var(--muted)' }}>Aucun abonnement. Parcourez le catalogue pour abonner cette application à une API.</p>
          ) : (
            <div className="tblwrap">
              <table className="tbl">
                <thead>
                  <tr><th>API</th><th>Plan</th><th>Débit</th><th>Consommation (rpm)</th><th>Statut</th><th></th></tr>
                </thead>
                <tbody>
                  {subs.map((s, i) => {
                    const pill = statusPill(s.status)
                    const width = demoBarWidth(s.productId)
                    return (
                      <tr key={s.productId}>
                        <td>
                          <div className="apicell">
                            <span className="ig" style={{ background: IG_COLORS[i % IG_COLORS.length] }}>{initials(s.productName)}</span>
                            <span><span className="nm">{s.productName}</span><span className="cx">{s.contextPath} · v{s.version}</span></span>
                          </div>
                        </td>
                        <td><span className="plan-pill"><span className="dot" style={{ background: PLAN_DOTS[s.planName] ?? 'var(--c-finance)' }} />{s.planName}</span></td>
                        <td className="rate">{rateLabel(plans.find(p => p.id === s.planId))}</td>
                        <td>
                          {/* DEMO: no per-subscription metrics yet (see demo.ts) */}
                          <div className={`bar ${width > 85 ? 'hi' : ''}`}><i style={{ width: `${width}%` }} /></div>
                          <div className="rowsub">{demoRpm(s.productId)} rpm · pic 24h</div>
                        </td>
                        <td><span className={`stpill ${pill.cls}`}><span className="led" />{pill.label}</span></td>
                        <td>
                          <div className="rowact">
                            {/* Blueprint placeholder kept per user choice (spec 2026-06-05) */}
                            <a className="linkbtn">Gérer</a>
                            <a className="linkbtn danger" onClick={() => onResiliate(s.productId, s.productName)}>Résilier</a>
                          </div>
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>
    </section>
  )
}
