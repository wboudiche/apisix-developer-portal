import type { AppDetail } from '../../api/types'
import { copyText } from './helpers'
import { describe as describeEvent } from './activity'
import { DEMO_QUICKSTART } from './demo'
import { useUsage } from './useUsage'
import { UsageCards } from './UsageCards'
import { useT } from '../../i18n/LanguageProvider'

const FEED_ICONS: Record<string, string> = {
  check: 'M20 6L9 17l-5-5',
  rotate: 'M21 2v6h-6M3 12a9 9 0 0115-6.7L21 8M3 22v-6h6M21 12a9 9 0 01-15 6.7L3 16',
  alert: 'M12 9v4M12 17h.01M10.3 4.3L2.5 18a2 2 0 001.7 3h15.6a2 2 0 001.7-3L13.7 4.3a2 2 0 00-3.4 0z',
  plus: 'M12 5v14M5 12h14',
}

export function OverviewTab({ detail, token, appId, notify }: { detail: AppDetail; token: string; appId: number; notify: (msg: string) => void }) {
  const t = useT()
  // Cards load asynchronously so the page shell (quickstart, activity feed)
  // renders immediately; the 24h window backs the "today"/p95/error cards.
  const usage = useUsage(token, appId, '24h')

  // Quickstart uses the first ACTIVE subscription's real gateway path + real key;
  // the blueprint sample otherwise.
  const active = detail.subscriptions.find(s => s.status === 'active')
  const path = active ? active.contextPath : DEMO_QUICKSTART.path
  const key = active ? detail.apiKey : DEMO_QUICKSTART.key
  const curl = `curl http://localhost:9080${path} -H "apikey: ${key}"`

  function copyCurl() {
    void copyText(curl).then(() => notify(t('app.copyCurlNotify')))
  }

  return (
    <section className="panel">
      <UsageCards state={usage} />

      <div className="twocol">
        <div className="dcard">
          <div className="ch">
            <h3>{t('app.quickstartTitle')}</h3>
            <p>{t('app.quickstartAuthPre')}<span className="mono">apikey</span>{t('app.quickstartAuthPost')}</p>
          </div>
          <div className="cb">
            <div className="code">
              <div className="cbar"><i /><i /><i /><span>{t('app.requestLabel', { name: active ? active.productName : DEMO_QUICKSTART.apiName })}</span>
                <button className="copy" onClick={copyCurl}>{t('subscribeModal.copy')}</button>
              </div>
              <pre><span className="c">{t('app.curlComment')}</span>{'\n'}<span className="cmd">curl</span> http://localhost:9080{path} \{'\n'}  <span className="flag">-H</span> <span className="str">"apikey: {key}"</span></pre>
            </div>
            <p style={{ fontSize: 13, color: 'var(--muted)', marginTop: 14, lineHeight: 1.55 }}>
              {t('app.quickstartInfoPre')}<b style={{ color: 'var(--fg)' }}>consumer</b>{t('app.quickstartInfoMid')}<span className="mono">key-auth</span> + <span className="mono">limit-count</span>{t('app.quickstartInfoMid2')}<b style={{ color: 'var(--fg)' }}>Sandbox</b>{t('app.quickstartInfoPost')}
            </p>
          </div>
        </div>

        <div className="dcard">
          <div className="ch"><h3>{t('app.recentActivityTitle')}</h3></div>
          <div className="cb" style={{ paddingTop: 6, paddingBottom: 6 }}>
            {detail.events.length === 0 ? (
              <p style={{ fontSize: 13, color: 'var(--muted)', padding: '14px 4px', lineHeight: 1.55 }}>
                {t('app.noActivity')}
              </p>
            ) : (
              <ul className="feed">
                {detail.events.map((e, i) => {
                  const f = describeEvent(e)
                  return (
                    <li key={i}>
                      <span className="fi"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} aria-hidden="true"><path d={FEED_ICONS[f.icon]} strokeLinecap="round" strokeLinejoin="round" /></svg></span>
                      <span className="ft"><b>{f.lead}</b>{f.rest}<small>{f.when}</small></span>
                    </li>
                  )
                })}
              </ul>
            )}
          </div>
        </div>
      </div>
    </section>
  )
}
