import { useState } from 'react'
import type { AppDetail, SubscriptionView } from '../../api/types'
import { copyText } from './helpers'
import { describe as describeEvent } from './activity'
import { DEMO_QUICKSTART } from './demo'
import { useUsage } from './useUsage'
import { UsageCards } from './UsageCards'
import { useT, useLang } from '../../i18n/LanguageProvider'

const FEED_ICONS: Record<string, string> = {
  check: 'M20 6L9 17l-5-5',
  rotate: 'M21 2v6h-6M3 12a9 9 0 0115-6.7L21 8M3 22v-6h6M21 12a9 9 0 01-15 6.7L3 16',
  alert: 'M12 9v4M12 17h.01M10.3 4.3L2.5 18a2 2 0 001.7 3h15.6a2 2 0 001.7-3L13.7 4.3a2 2 0 00-3.4 0z',
  plus: 'M12 5v14M5 12h14',
}

export function OverviewTab({ detail, token, appId, notify }: { detail: AppDetail; token: string; appId: number; notify: (msg: string) => void }) {
  const t = useT()
  const { lang } = useLang()
  // Cards load asynchronously so the page shell (quickstart, activity feed)
  // renders immediately; the 24h window backs the "today"/p95/error cards.
  const usage = useUsage(token, appId, '24h')

  // Quickstart shows one example per ACTIVE subscription — each may use a
  // different auth method (apikey vs. OAuth2) — or the blueprint sample when
  // the app has none yet. selectedProductId tracks the picked tab; falling
  // back to the first active subscription keeps it valid as subscriptions change.
  const activeSubs = detail.subscriptions.filter(s => s.status === 'active')
  const [selectedProductId, setSelectedProductId] = useState<number | null>(null)
  const selected: SubscriptionView | null =
    activeSubs.find(s => s.productId === selectedProductId) ?? activeSubs[0] ?? null
  const isOAuth = selected?.authType === 'oauth2'

  const path = selected ? selected.contextPath : DEMO_QUICKSTART.path
  const requestName = selected ? selected.productName : DEMO_QUICKSTART.apiName
  const curl = isOAuth
    ? `curl http://localhost:9080${path} -H "Authorization: Bearer ${t('app.quickstartOAuthTokenPlaceholder')}"`
    : `curl http://localhost:9080${path} -H "apikey: ${selected ? detail.apiKey : DEMO_QUICKSTART.key}"`

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
            {isOAuth
              ? <p>{t('app.quickstartOAuthAuthPre')}<span className="mono">Authorization: Bearer</span>{t('app.quickstartOAuthAuthPost')}</p>
              : <p>{t('app.quickstartAuthPre')}<span className="mono">apikey</span>{t('app.quickstartAuthPost')}</p>}
          </div>
          <div className="cb">
            {activeSubs.length > 1 && (
              <div className="qs-tabs" role="group" aria-label={t('app.quickstartSubscriptionsLabel')}>
                {activeSubs.map(s => (
                  <button key={s.productId} type="button" className={selected?.productId === s.productId ? 'on' : ''}
                    aria-pressed={selected?.productId === s.productId} onClick={() => setSelectedProductId(s.productId)}>
                    {s.productName}
                  </button>
                ))}
              </div>
            )}
            <div className="code">
              <div className="cbar"><i /><i /><i /><span>{t('app.requestLabel', { name: requestName })}</span>
                <button className="copy" onClick={copyCurl}>{t('subscribeModal.copy')}</button>
              </div>
              {isOAuth
                ? <pre><span className="c">{t('app.quickstartOAuthComment')}</span>{'\n'}<span className="cmd">curl</span> http://localhost:9080{path} \{'\n'}  <span className="flag">-H</span> <span className="str">"Authorization: Bearer {t('app.quickstartOAuthTokenPlaceholder')}"</span></pre>
                : <pre><span className="c">{t('app.curlComment')}</span>{'\n'}<span className="cmd">curl</span> http://localhost:9080{path} \{'\n'}  <span className="flag">-H</span> <span className="str">"apikey: {selected ? detail.apiKey : DEMO_QUICKSTART.key}"</span></pre>}
            </div>
            {isOAuth
              ? <p style={{ fontSize: 13, color: 'var(--muted)', marginTop: 14, lineHeight: 1.55 }}>
                  {t('app.quickstartOAuthInfoPre')}<span className="mono">{detail.oidcClientId || '—'}</span>{t('app.quickstartOAuthInfoMid')}<span className="mono">{detail.oidcIssuer || '—'}</span>{t('app.quickstartOAuthInfoPost')}
                </p>
              : <p style={{ fontSize: 13, color: 'var(--muted)', marginTop: 14, lineHeight: 1.55 }}>
                  {t('app.quickstartInfoPre')}<b style={{ color: 'var(--fg)' }}>consumer</b>{t('app.quickstartInfoMid')}<span className="mono">key-auth</span> + <span className="mono">limit-count</span>{t('app.quickstartInfoMid2')}<b style={{ color: 'var(--fg)' }}>Sandbox</b>{t('app.quickstartInfoPost')}
                </p>}
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
                  const f = describeEvent(e, t, lang)
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
