import { useEffect, useState } from 'react'
import type { Product, Application, Plan } from '../api/types'
import { getApplications, getPlans, createApplication, subscribe } from '../api/client'
import { useAuth } from '../auth/AuthProvider'

export function SubscribeModal({ product, onClose }: { product: Product; onClose: () => void }) {
  const { token } = useAuth()
  const [apps, setApps] = useState<Application[]>([])
  const [plans, setPlans] = useState<Plan[]>([])
  const [appId, setAppId] = useState<number | 'new'>('new')
  const [newName, setNewName] = useState('')
  const [planId, setPlanId] = useState<number | null>(null)
  const [apiKey, setApiKey] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (!token) return
    Promise.all([getApplications(token), getPlans()])
      .then(([a, p]) => {
        setApps(a); setPlans(p)
        if (a.length) setAppId(a[0].id)
        if (p.length) setPlanId(p[0].id)
      })
      .catch(() => setErr('Impossible de charger les applications et les plans.'))
  }, [token])

  async function onSubmit() {
    if (!token || planId == null) return
    setErr(''); setBusy(true)
    try {
      let targetApp = appId
      if (targetApp === 'new') {
        const created = await createApplication(token, newName || 'Mon application', '')
        targetApp = created.id
      }
      const cred = await subscribe(token, targetApp as number, product.id, planId)
      setApiKey(cred.apiKey)
    } catch (e) {
      setErr(e instanceof Error ? e.message : "Échec de l'abonnement")
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={e => e.stopPropagation()} role="dialog" aria-label="S'abonner">
        <h2>S'abonner à {product.name}</h2>
        {apiKey ? (
          <div className="keybox">
            <p className="rescount">Votre clé d'API (copiez-la, elle ne sera plus affichée intégralement) :</p>
            <code className="apikey">{apiKey}</code>
            <button className="subbtn" onClick={() => navigator.clipboard?.writeText(apiKey)}>Copier</button>
            <button className="subbtn ghost" onClick={onClose}>Fermer</button>
          </div>
        ) : (
          <>
            <label>Application
              <select value={String(appId)} onChange={e => setAppId(e.target.value === 'new' ? 'new' : Number(e.target.value))} aria-label="Application">
                {apps.map(a => <option key={a.id} value={a.id}>{a.name}</option>)}
                <option value="new">+ Nouvelle application</option>
              </select>
            </label>
            {appId === 'new' && (
              <label>Nom de l'application
                <input value={newName} onChange={e => setNewName(e.target.value)} placeholder="Mon application" aria-label="Nom de l'application" />
              </label>
            )}
            <label>Plan
              <select value={planId ?? ''} onChange={e => setPlanId(Number(e.target.value))} aria-label="Plan">
                {plans.map(p => <option key={p.id} value={p.id}>{p.name} — {p.rateLimit}/{p.windowSeconds}s</option>)}
              </select>
            </label>
            {err && <p className="autherr" role="alert">{err}</p>}
            <div className="modal-actions">
              <button className="subbtn ghost" onClick={onClose}>Annuler</button>
              <button className="subbtn" onClick={onSubmit} disabled={busy || planId == null}>{busy ? '…' : "Confirmer l'abonnement"}</button>
            </div>
          </>
        )}
      </div>
    </div>
  )
}
