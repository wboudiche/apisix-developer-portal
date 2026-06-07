import { useCallback, useEffect, useRef, useState } from 'react'
import type { AdminSubscription } from '../../api/types'
import { adminGetSubscriptions, adminApproveSubscription, adminRejectSubscription } from '../../api/client'
import { useAuth } from '../../auth/AuthProvider'
import { AdminShell } from './AdminShell'
import { Toast, useToast } from '../../components/Toast'

// Blueprint avatar initials: capitals of the app name, else first two letters.
const subInitials = (name: string) =>
  name.replace(/[^A-Z]/g, '').slice(0, 2) || name.slice(0, 2).toUpperCase()

const KNOWN_TAGS = ['Free', 'Silver', 'Gold']

export function ApprovalsPage() {
  const { token } = useAuth()
  const [subs, setSubs] = useState<AdminSubscription[]>([])
  const [loaded, setLoaded] = useState(false)
  const [err, setErr] = useState('')
  const { toast, notify } = useToast()

  // Monotonic guard: only the latest list request may write state.
  const reqSeq = useRef(0)
  const reload = useCallback(() => {
    if (!token) return
    const seq = ++reqSeq.current
    adminGetSubscriptions(token, 'pending')
      .then(l => { if (seq === reqSeq.current) { setSubs(l); setLoaded(true) } })
      .catch(() => { if (seq === reqSeq.current) setErr('Impossible de charger les abonnements.') })
  }, [token])
  useEffect(reload, [reload])

  async function approve(s: AdminSubscription) {
    if (!token) return
    try {
      await adminApproveSubscription(token, s.id)
      notify(`Abonnement de ${s.applicationName} approuvé — consumer APISIX créé`)
      reload()
    } catch (e) { notify(e instanceof Error ? e.message : "Échec de l'approbation.", 'warn') }
  }

  async function reject(s: AdminSubscription) {
    if (!token) return
    try {
      await adminRejectSubscription(token, s.id)
      notify(`Demande de ${s.applicationName} refusée`, 'warn')
      reload()
    } catch (e) { notify(e instanceof Error ? e.message : 'Échec du refus.', 'warn') }
  }

  return (
    <AdminShell
      active="approvals"
      title="Abonnements en attente"
      description="Validez ou refusez les demandes d'abonnement des applications. À l'approbation, APISIX crée le consumer et active la politique de débit du plan choisi."
      counts={{ pending: subs.length }}
    >
      {err && <p className="autherr" role="alert">{err}</p>}

      {loaded && subs.length === 0 ? (
        <div className="aempty">
          <div className="aico">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} aria-hidden="true"><path d="M20 6L9 17l-5-5" strokeLinecap="round" strokeLinejoin="round" /></svg>
          </div>
          <h4>File d'attente vide</h4>
          <p>Aucun abonnement en attente de validation.</p>
        </div>
      ) : (
        <div className="rows">
          {subs.map(s => (
            <div className="sub-card" key={s.id}>
              <div className="app-av">{subInitials(s.applicationName)}</div>
              <div className="sub-main">
                <div className="ttl"><b>{s.applicationName}</b><span className="arr">→</span>{s.productName}</div>
                <div className="sub-meta">
                  <span className={`plan-tag ${KNOWN_TAGS.includes(s.planName) ? s.planName : ''}`}>{s.planName}</span>
                  <span>demandé par <span className="who2">{s.ownerEmail}</span></span>
                  ·
                  <time>{s.createdAt.slice(0, 10)}</time>
                </div>
              </div>
              <div className="sub-acts">
                <button className="btn-reject" onClick={() => reject(s)}>Refuser</button>
                <button className="btn-approve" onClick={() => approve(s)}>Approuver</button>
              </div>
            </div>
          ))}
        </div>
      )}

      <Toast msg={toast?.msg ?? null} kind={toast?.kind} />
    </AdminShell>
  )
}
