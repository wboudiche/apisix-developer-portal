import type { AppDetail } from '../../api/types'
import { copyText } from './helpers'
import { DEMO_STATS, DEMO_FEED, DEMO_QUICKSTART } from './demo'

const STAT_ICONS: Record<string, string> = {
  pulse: 'M3 12h4l3 8 4-16 3 8h4',
  calendar: 'M3 9h18M8 4v16M3 4h18v16H3z',
  clock: 'M12 7v5l3 2M12 21a9 9 0 100-18 9 9 0 000 18z',
  alert: 'M12 9v4M12 17h.01M10.3 4.3L2.5 18a2 2 0 001.7 3h15.6a2 2 0 001.7-3L13.7 4.3a2 2 0 00-3.4 0z',
}
const FEED_ICONS: Record<string, string> = {
  check: 'M20 6L9 17l-5-5',
  rotate: 'M21 2v6h-6M3 12a9 9 0 0115-6.7L21 8M3 22v-6h6M21 12a9 9 0 01-15 6.7L3 16',
  alert: 'M12 9v4M12 17h.01M10.3 4.3L2.5 18a2 2 0 001.7 3h15.6a2 2 0 001.7-3L13.7 4.3a2 2 0 00-3.4 0z',
  plus: 'M12 5v14M5 12h14',
}

function Arrow({ dir }: { dir: 'up' | 'down' }) {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2.5} aria-hidden="true">
      <path d={dir === 'up' ? 'M6 15l6-6 6 6' : 'M18 9l-6 6-6-6'} strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

export function OverviewTab({ detail, notify }: { detail: AppDetail; notify: (msg: string) => void }) {
  // Quickstart uses the first ACTIVE subscription's real gateway path + real key;
  // the blueprint sample otherwise.
  const active = detail.subscriptions.find(s => s.status === 'active')
  const path = active ? active.contextPath : DEMO_QUICKSTART.path
  const key = active ? detail.apiKey : DEMO_QUICKSTART.key
  const curl = `curl http://localhost:9080${path} -H "apikey: ${key}"`

  function copyCurl() {
    void copyText(curl).then(() => notify('Commande copiée'))
  }

  return (
    <section className="panel">
      {/* DEMO metrics — no metrics pipeline yet (see demo.ts) */}
      <div className="stats">
        {DEMO_STATS.map(s => (
          <div className="stat" key={s.label}>
            <div className="k">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} aria-hidden="true"><path d={STAT_ICONS[s.icon]} strokeLinecap="round" strokeLinejoin="round" /></svg>
              {s.label}
            </div>
            <div className="v">{s.value}{s.unit && <> <small>{s.unit}</small></>}</div>
            <div className={`d ${s.delta.dir}`}>{s.delta.arrow && <Arrow dir={s.delta.arrow} />}{s.delta.text}</div>
          </div>
        ))}
      </div>

      <div className="twocol">
        <div className="dcard">
          <div className="ch">
            <div>
              <h3>Démarrage rapide</h3>
              <p>Authentification par clé API — un seul en-tête <span className="mono">apikey</span>.</p>
            </div>
          </div>
          <div className="cb">
            <div className="code">
              <div className="cbar"><i /><i /><i /><span>requête — production</span>
                <button className="copy" onClick={copyCurl}>Copier</button>
              </div>
              <pre><span className="c"># Un seul en-tête, c'est tout</span>{'\n'}<span className="cmd">curl</span> http://localhost:9080{path} \{'\n'}  <span className="flag">-H</span> <span className="str">"apikey: {key}"</span></pre>
            </div>
            <p style={{ fontSize: 13, color: 'var(--muted)', marginTop: 14, lineHeight: 1.55 }}>
              La clé est liée à un <b style={{ color: 'var(--fg)' }}>consumer</b> APISIX et au plan choisi à l'abonnement (<span className="mono">key-auth</span> + <span className="mono">limit-count</span>). Utilisez la clé <b style={{ color: 'var(--fg)' }}>Sandbox</b> pour tester sans consommer votre quota production.
            </p>
          </div>
        </div>

        <div className="dcard">
          <div className="ch"><h3>Activité récente</h3></div>
          <div className="cb" style={{ paddingTop: 6, paddingBottom: 6 }}>
            {/* DEMO feed — no activity log yet (see demo.ts) */}
            <ul className="feed">
              {DEMO_FEED.map(f => (
                <li key={f.lead + f.when}>
                  <span className="fi"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} aria-hidden="true"><path d={FEED_ICONS[f.icon]} strokeLinecap="round" strokeLinejoin="round" /></svg></span>
                  <span className="ft"><b>{f.lead}</b>{f.rest}<small>{f.when}</small></span>
                </li>
              ))}
            </ul>
          </div>
        </div>
      </div>
    </section>
  )
}
