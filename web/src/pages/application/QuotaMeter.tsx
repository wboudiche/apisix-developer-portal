import { useEffect, useState } from 'react'
import { getQuota } from '../../api/client'
import type { Quota } from '../../api/types'
import { useT } from '../../i18n/LanguageProvider'

export function QuotaMeter({ token, appId }: { token: string; appId: number }) {
  const t = useT()
  const [quota, setQuota] = useState<Quota | null>(null)
  useEffect(() => {
    let alive = true
    getQuota(token, appId).then(q => { if (alive) setQuota(q) }).catch(() => { if (alive) setQuota(null) })
    return () => { alive = false }
  }, [token, appId])

  if (!quota || !quota.hasQuota) return null

  if (!quota.available) {
    return (
      <div className="quota-meter">
        <div className="qm-row"><span className="qm-title">{t('app.quotaTitle')}</span><span className="qm-na">{t('app.quotaUnavailable')}</span></div>
        <div className="qm-sub">{t('app.quotaLimit', { limit: quota.limit ?? 0, window: quota.windowSeconds ?? 0 })}</div>
      </div>
    )
  }

  const used = quota.used ?? 0
  const limit = quota.limit ?? 0
  const pct = limit > 0 ? Math.min(100, Math.round((used / limit) * 100)) : 0
  const level = pct >= 100 ? 'danger' : pct >= 80 ? 'warn' : 'ok'
  return (
    <div className="quota-meter">
      <div className="qm-row">
        <span className="qm-title">{t('app.quotaTitle')}</span>
        <span className="qm-count">≈ {used} / {limit}</span>
      </div>
      <div className="qm-bar"><span className={`qm-fill ${level}`} style={{ width: `${pct}%` }} /></div>
      <div className="qm-sub">{t('app.quotaWindow', { window: quota.windowSeconds ?? 0 })}</div>
    </div>
  )
}
