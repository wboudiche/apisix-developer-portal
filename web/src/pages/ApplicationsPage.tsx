import { useEffect, useState } from 'react'
import type { Application, AppDetail } from '../api/types'
import { getApplications, getApplicationDetail, createApplication, unsubscribe } from '../api/client'
import { useAuth } from '../auth/AuthProvider'
import { TopBar } from '../components/TopBar'
import '../styles/catalog.css'

export function ApplicationsPage() {
  const { token } = useAuth()
  const [apps, setApps] = useState<Application[]>([])
  const [selected, setSelected] = useState<number | null>(null)
  const [detail, setDetail] = useState<AppDetail | null>(null)
  const [newName, setNewName] = useState('')
  const [err, setErr] = useState('')

  function reloadApps() {
    if (!token) return
    getApplications(token).then(a => {
      setApps(a)
      if (a.length && selected == null) setSelected(a[0].id)
    }).catch(() => setErr('Impossible de charger les applications.'))
  }
  useEffect(reloadApps, [token])

  useEffect(() => {
    if (!token || selected == null) { setDetail(null); return }
    getApplicationDetail(token, selected).then(setDetail).catch(() => setDetail(null))
  }, [token, selected])

  async function onCreate() {
    if (!token || !newName) return
    const a = await createApplication(token, newName, '')
    setNewName(''); setSelected(a.id); reloadApps()
  }
  async function onUnsub(productId: number) {
    if (!token || selected == null) return
    await unsubscribe(token, selected, productId)
    getApplicationDetail(token, selected).then(setDetail)
  }

  return (
    <>
      <TopBar search="" onSearch={() => {}} />
      <div className="content">
        <div className="chead"><div className="titlewrap"><h1>Mes applications</h1></div></div>
        {err && <p className="autherr" role="alert">{err}</p>}
        <div style={{ display: 'flex', gap: 10, marginBottom: 18 }}>
          <input value={newName} onChange={e => setNewName(e.target.value)} placeholder="Nom de la nouvelle application" aria-label="Nom de la nouvelle application"
            style={{ height: 40, padding: '0 12px', border: '1px solid var(--border-2)', borderRadius: 10, background: 'var(--bg)', color: 'var(--fg)' }} />
          <button className="subbtn" onClick={onCreate}>Créer</button>
        </div>
        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginBottom: 18 }}>
          {apps.map(a => (
            <button key={a.id} className={`tag ${selected === a.id ? 'active' : ''}`} onClick={() => setSelected(a.id)}>{a.name}</button>
          ))}
        </div>
        {detail && (
          <div className="card" style={{ padding: 22 }}>
            <div className="cmeta"><span className="pill">Clé d&apos;API</span></div>
            <code className="apikey">{detail.apiKey || '— aucune (abonnez une API)'}</code>
            <h3 style={{ marginTop: 18 }}>Abonnements</h3>
            {detail.subscriptions.length === 0 && <p className="rescount">Aucun abonnement.</p>}
            {detail.subscriptions.map(s => (
              <div key={s.productId} className="cfoot" style={{ justifyContent: 'space-between' }}>
                <span><span>{s.productName}</span> <span className="ctx">{s.contextPath}</span> · {s.planName}</span>
                <button className="subbtn ghost" onClick={() => onUnsub(s.productId)}>Se désabonner</button>
              </div>
            ))}
          </div>
        )}
      </div>
    </>
  )
}
