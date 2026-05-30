import { useEffect, useState } from 'react'
import type { AdminSubscription } from '../api/types'
import { adminGetSubscriptions, adminApproveSubscription, adminRejectSubscription } from '../api/client'
import { useAuth } from '../auth/AuthProvider'
import { AdminNav } from '../components/AdminNav'
import '../styles/catalog.css'

export function AdminApprovalsPage() {
  const { token } = useAuth()
  const [subs, setSubs] = useState<AdminSubscription[]>([])
  const [err, setErr] = useState('')

  function reload() {
    if (!token) return
    adminGetSubscriptions(token, 'pending').then(setSubs).catch(() => setErr('Impossible de charger les abonnements.'))
  }
  useEffect(reload, [token])

  async function act(id: number, fn: (t: string, i: number) => Promise<void>) {
    if (!token) return
    setErr('')
    try { await fn(token, id); reload() }
    catch (e) { setErr(e instanceof Error ? e.message : "Échec de l'opération.") }
  }

  return (
    <>
      <AdminNav active="approvals" />
      <div className="content">
        <div className="chead"><div className="titlewrap"><h1>Abonnements en attente</h1></div></div>
        {err && <p className="autherr" role="alert">{err}</p>}
        {subs.length === 0 && <p className="rescount">Aucun abonnement en attente.</p>}
        {subs.map(s => (
          <div key={s.id} className="cfoot" style={{ justifyContent: 'space-between', padding: '12px 0', borderBottom: '1px solid var(--border)' }}>
            <span>
              <b>{s.productName}</b> <span className="ctx">{s.version}</span> · {s.planName}
              {' — '}<span>{s.applicationName}</span> <span className="pill">{s.ownerEmail}</span>
            </span>
            <span style={{ display: 'flex', gap: 8 }}>
              <button className="subbtn" onClick={() => act(s.id, adminApproveSubscription)}>Approuver</button>
              <button className="subbtn ghost" onClick={() => act(s.id, adminRejectSubscription)}>Rejeter</button>
            </span>
          </div>
        ))}
      </div>
    </>
  )
}
