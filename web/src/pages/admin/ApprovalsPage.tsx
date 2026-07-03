import { useCallback, useEffect, useRef, useState } from 'react'
import type { AdminSubscription } from '../../api/types'
import { adminGetSubscriptions, adminApproveSubscription, adminRejectSubscription } from '../../api/client'
import { useAuth } from '../../auth/AuthProvider'
import { AdminShell } from './AdminShell'
import { Toast, useToast } from '../../components/Toast'
import { Pagination } from '../../components/Pagination'
import { useT } from '../../i18n/LanguageProvider'

// Blueprint avatar initials: capitals of the app name, else first two letters.
const subInitials = (name: string) =>
  name.replace(/[^A-Z]/g, '').slice(0, 2) || name.slice(0, 2).toUpperCase()

const KNOWN_TAGS = ['Free', 'Silver', 'Gold']

export function ApprovalsPage() {
  const { token } = useAuth()
  const t = useT()
  const [subs, setSubs] = useState<AdminSubscription[]>([])
  const [loaded, setLoaded] = useState(false)
  const [err, setErr] = useState('')
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const pageSize = 20
  const { toast, notify } = useToast()

  // Monotonic guard: only the latest list request may write state.
  const reqSeq = useRef(0)
  const reload = useCallback(() => {
    if (!token) return
    const seq = ++reqSeq.current
    adminGetSubscriptions(token, 'pending', { page, pageSize })
      .then(r => { if (seq === reqSeq.current) { setSubs(r.items); setTotal(r.total); setLoaded(true) } })
      .catch(() => { if (seq === reqSeq.current) setErr(t('admin.loadSubscriptionsError')) })
  }, [token, page])
  useEffect(reload, [reload])

  async function approve(s: AdminSubscription) {
    if (!token) return
    try {
      await adminApproveSubscription(token, s.id)
      notify(t('admin.approveNotify', { name: s.applicationName }))
      reload()
    } catch (e) { notify(e instanceof Error ? e.message : t('admin.approveFailed'), 'warn') }
  }

  async function reject(s: AdminSubscription) {
    if (!token) return
    try {
      await adminRejectSubscription(token, s.id)
      notify(t('admin.rejectNotify', { name: s.applicationName }), 'warn')
      reload()
    } catch (e) { notify(e instanceof Error ? e.message : t('admin.rejectFailed'), 'warn') }
  }

  return (
    <AdminShell
      active="approvals"
      title={t('admin.approvalsPageTitle')}
      description={t('admin.approvalsDescription')}
      counts={{ pending: subs.length }}
    >
      {err && <p className="autherr" role="alert">{err}</p>}

      {loaded && subs.length === 0 ? (
        <div className="aempty">
          <div className="aico">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} aria-hidden="true"><path d="M20 6L9 17l-5-5" strokeLinecap="round" strokeLinejoin="round" /></svg>
          </div>
          <h4>{t('admin.emptyQueueHeading')}</h4>
          <p>{t('admin.emptyQueueBody')}</p>
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
                  <span>{t('admin.requestedByPrefix')}<span className="who2">{s.ownerEmail}</span></span>
                  ·
                  <time>{s.createdAt.slice(0, 10)}</time>
                </div>
              </div>
              <div className="sub-acts">
                <button className="btn-reject" onClick={() => reject(s)}>{t('admin.rejectAction')}</button>
                <button className="btn-approve" onClick={() => approve(s)}>{t('admin.approveAction')}</button>
              </div>
            </div>
          ))}
        </div>
      )}

      <Pagination page={page} pageSize={pageSize} total={total} onPage={setPage} />
      <Toast msg={toast?.msg ?? null} kind={toast?.kind} />
    </AdminShell>
  )
}
