import { useEffect, useState } from 'react'
import type { Product, Application, Plan } from '../api/types'
import { getApplications, getPlans, createApplication, subscribe } from '../api/client'
import { useAuth } from '../auth/AuthProvider'
import { useT } from '../i18n/LanguageProvider'

export function SubscribeModal({ product, onClose }: { product: Product; onClose: () => void }) {
  const t = useT()
  const { token } = useAuth()
  const [apps, setApps] = useState<Application[]>([])
  const [plans, setPlans] = useState<Plan[]>([])
  const [appId, setAppId] = useState<number | 'new'>('new')
  const [newName, setNewName] = useState('')
  const [planId, setPlanId] = useState<number | null>(null)
  const [apiKey, setApiKey] = useState('')
  const [copied, setCopied] = useState(false)
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (!token) return
    Promise.all([getApplications(token), getPlans()])
      .then(([r, p]) => {
        setApps(r.items); setPlans(p.items)
        if (r.items.length) setAppId(r.items[0].id)
        if (p.items.length) setPlanId(p.items[0].id)
      })
      .catch(() => setErr(t('subscribeModal.loadError')))
  }, [token])

  async function copyKey() {
    try { await navigator.clipboard?.writeText(apiKey) } catch { /* ignore */ }
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  async function onSubmit() {
    if (!token || planId == null) return
    setErr(''); setBusy(true)
    try {
      let targetApp = appId
      if (targetApp === 'new') {
        const created = await createApplication(token, newName || t('subscribeModal.appNamePlaceholder'), '')
        targetApp = created.id
      }
      const cred = await subscribe(token, targetApp as number, product.id, planId)
      setApiKey(cred.apiKey)
    } catch (e) {
      setErr(e instanceof Error ? e.message : t('subscribeModal.subscribeError'))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={e => e.stopPropagation()} role="dialog" aria-label={t('subscribeModal.dialogLabel')}>
        <h2>{t('subscribeModal.heading', { name: product.name })}</h2>
        {apiKey ? (
          <div className="keybox">
            <p className="rescount">{t('subscribeModal.keyIntro')}</p>
            <code className="apikey">{apiKey}</code>
            <button className="subbtn" onClick={copyKey}>{copied ? t('subscribeModal.copied') : t('subscribeModal.copy')}</button>
            <button className="subbtn ghost" onClick={onClose}>{t('common.close')}</button>
          </div>
        ) : (
          <>
            <label>{t('subscribeModal.applicationLabel')}
              <select value={String(appId)} onChange={e => setAppId(e.target.value === 'new' ? 'new' : Number(e.target.value))} aria-label={t('subscribeModal.applicationLabel')}>
                {apps.map(a => <option key={a.id} value={a.id}>{a.name}</option>)}
                <option value="new">{t('subscribeModal.newApplication')}</option>
              </select>
            </label>
            {appId === 'new' && (
              <label>{t('subscribeModal.appNameLabel')}
                <input value={newName} onChange={e => setNewName(e.target.value)} placeholder={t('subscribeModal.appNamePlaceholder')} aria-label={t('subscribeModal.appNameLabel')} />
              </label>
            )}
            <label>{t('subscribeModal.planLabel')}
              <select value={planId ?? ''} onChange={e => setPlanId(Number(e.target.value))} aria-label={t('subscribeModal.planLabel')}>
                {plans.map(p => <option key={p.id} value={p.id}>{p.name} — {p.rateLimit}/{p.windowSeconds}s</option>)}
              </select>
            </label>
            {err && <p className="autherr" role="alert">{err}</p>}
            <div className="modal-actions">
              <button className="subbtn ghost" onClick={onClose}>{t('common.cancel')}</button>
              <button className="subbtn" onClick={onSubmit} disabled={busy || planId == null}>{busy ? '…' : t('subscribeModal.confirmSubscribe')}</button>
            </div>
          </>
        )}
      </div>
    </div>
  )
}
