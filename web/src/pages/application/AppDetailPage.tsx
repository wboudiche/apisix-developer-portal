import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, Navigate, useNavigate, useParams } from 'react-router-dom'
import { getApplications, getApplicationDetail, getPlans, createApplication, unsubscribe } from '../../api/client'
import { useAuth } from '../../auth/AuthProvider'
import { TopBar } from '../../components/TopBar'
import type { Application, AppDetail, Plan } from '../../api/types'
import { appRef, initials, useFormatDate, glyphGradient, subsCountKey } from './helpers'
import { AppSwitcher, CreateAppModal } from './AppSwitcher'
import { ConfirmModal, type ModalSpec } from '../../components/ConfirmModal'
import { Toast } from '../../components/Toast'
import { OverviewTab } from './OverviewTab'
import { CredentialsTab } from './CredentialsTab'
import { SubscriptionsTab } from './SubscriptionsTab'
import { UsageTab } from './UsageTab'
import { SettingsTab } from './SettingsTab'
import { useT } from '../../i18n/LanguageProvider'
import '../../styles/appdetail.css'

type TabKey = 'overview' | 'creds' | 'subs' | 'usage' | 'settings'
const TAB_KEYS: TabKey[] = ['overview', 'creds', 'subs', 'usage', 'settings']
const TAB_LABEL_KEYS: Record<TabKey, string> = {
  overview: 'app.tabOverview', creds: 'app.tabCreds', subs: 'app.tabSubs', usage: 'app.tabUsage', settings: 'app.tabSettings',
}

function initialTab(): TabKey {
  try {
    const saved = localStorage.getItem('app:tab') as TabKey | null
    return saved && TAB_KEYS.includes(saved) ? saved : 'overview'
  } catch { return 'overview' }
}

export function AppDetailPage() {
  const { token } = useAuth()
  const { id } = useParams()
  const nav = useNavigate()
  const appId = Number(id)
  const t = useT()
  const formatDate = useFormatDate()

  const [apps, setApps] = useState<Application[] | null>(null)
  const [detail, setDetail] = useState<AppDetail | null>(null)
  const [plans, setPlans] = useState<Plan[]>([])
  const [tab, setTabState] = useState<TabKey>(initialTab)
  const [toastMsg, setToastMsg] = useState<string | null>(null)
  const [modal, setModal] = useState<ModalSpec | null>(null)
  const [createOpen, setCreateOpen] = useState(false)
  const [err, setErr] = useState('')
  const toastTimer = useRef<ReturnType<typeof setTimeout>>(undefined)
  // Monotonic guard: only the latest detail request may write state, so a slow
  // response for app A can't overwrite app B after rapid switcher navigation.
  const detailReq = useRef(0)

  useEffect(() => () => { clearTimeout(toastTimer.current); detailReq.current++ }, [])

  const notify = useCallback((msg: string) => {
    setToastMsg(msg)
    clearTimeout(toastTimer.current)
    toastTimer.current = setTimeout(() => setToastMsg(null), 1900)
  }, [])

  function setTab(t: TabKey) {
    setTabState(t)
    try { localStorage.setItem('app:tab', t) } catch { /* private mode */ }
  }

  const reloadDetail = useCallback(() => {
    if (!token || !Number.isFinite(appId)) return
    const seq = ++detailReq.current
    getApplicationDetail(token, appId)
      .then(d => { if (seq === detailReq.current) setDetail(d) })
      .catch(() => { if (seq === detailReq.current) setErr(t('app.loadAppError')) })
  }, [token, appId, t])

  useEffect(() => {
    if (!token) return
    getApplications(token).then(r => setApps(r.items)).catch(() => setErr(t('app.loadAppsError')))
    getPlans().then(r => setPlans(r.items)).catch(() => { /* rates show as — */ })
  }, [token, t])

  useEffect(() => { setDetail(null); setErr(''); reloadDetail() }, [reloadDetail])

  if (!token) return <Navigate to="/login" replace />
  if (apps && !apps.some(a => a.id === appId)) return <Navigate to="/applications" replace />

  const app = apps?.find(a => a.id === appId) ?? null
  const subs = detail?.subscriptions ?? []
  const overall = subs.some(s => s.status === 'active')
    ? { cls: 'ok', label: t('app.statusActive') }
    : subs.some(s => s.status === 'pending')
      ? { cls: 'warn', label: t('app.statusPending') }
      : { cls: 'muted', label: t('app.statusNone') }

  function onResiliate(productId: number, productName: string) {
    setModal({
      title: t('app.unsubscribeConfirmTitle', { name: productName }),
      body: t('app.unsubscribeConfirmBody', { name: productName }),
      confirmLabel: t('app.unsubscribeAction'), danger: true,
      onConfirm: () => {
        if (!token) return
        unsubscribe(token, appId, productId)
          .then(() => { notify(t('app.unsubscribedNotify', { name: productName })); reloadDetail() })
          .catch(() => notify(t('app.unsubscribeFailed')))
      },
    })
  }

  async function onCreateApp(name: string) {
    if (!token) return
    const a = await createApplication(token, name, '')
    setCreateOpen(false)
    notify(t('app.appCreatedNotify'))
    const next = await getApplications(token)
    setApps(next.items)
    nav(`/applications/${a.id}`)
  }

  return (
    <>
      <TopBar search="" onSearch={() => {}} />
      <div className="appdetail">
        <div className="crumbs">
          <Link to="/">{t('app.breadcrumbPortal')}</Link>
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} aria-hidden="true"><path d="M9 6l6 6-6 6" strokeLinecap="round" strokeLinejoin="round" /></svg>
          <Link to="/applications">{t('nav.applications')}</Link>
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} aria-hidden="true"><path d="M9 6l6 6-6 6" strokeLinecap="round" strokeLinejoin="round" /></svg>
          <span style={{ color: 'var(--fg)', fontWeight: 500 }}>{app?.name ?? '…'}</span>
        </div>

        {err && <p className="autherr" role="alert">{err}</p>}

        {app && (
          <div className="apphead">
            <div className="glyph" style={{ background: glyphGradient(app.id) }}>{initials(app.name)}</div>
            <div className="htext">
              <h1>
                {app.name}
                <span className={`stpill ${overall.cls}`}><span className="led" />{overall.label}</span>
              </h1>
              <div className="meta">
                <span>ID&nbsp;<span className="mono">{appRef(app.id)}</span></span>
                <span className="sep" />
                <span>{t(subsCountKey(subs.length), { count: subs.length })}</span>
                <span className="sep" />
                <span>{t('app.createdOnPrefix')}<span className="mono">{formatDate(app.createdAt)}</span></span>
                <span className="sep" />
                {apps && <AppSwitcher apps={apps} currentId={app.id} onCreate={() => setCreateOpen(true)} />}
              </div>
            </div>
            <div className="actions">
              <button className="btn ghost" onClick={() => setTab('settings')}>
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} aria-hidden="true"><circle cx="12" cy="12" r="3" /><path d="M12 3v2.5M12 18.5V21M21 12h-2.5M5.5 12H3M18 6l-1.8 1.8M7.8 16.2L6 18M18 18l-1.8-1.8M7.8 7.8L6 6" strokeLinecap="round" /></svg>
                {t('app.tabSettings')}
              </button>
              {tab !== 'subs' && (
                <button className="btn primary" onClick={() => nav('/')}>
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} aria-hidden="true"><path d="M12 5v14M5 12h14" strokeLinecap="round" /></svg>
                  {t('app.subscribeApiCta')}
                </button>
              )}
            </div>
          </div>
        )}

        <div className="tabs">
          {TAB_KEYS.map(k => (
            <button key={k} className={tab === k ? 'on' : ''} onClick={() => setTab(k)}>
              {t(TAB_LABEL_KEYS[k])}
              {k === 'subs' && <span className="badge">{subs.length}</span>}
            </button>
          ))}
        </div>

        {detail && app && (
          <>
            {tab === 'overview' && <OverviewTab detail={detail} token={token} appId={appId} notify={notify} />}
            {tab === 'creds' && (
              <CredentialsTab
                apiKey={detail.apiKey}
                appId={appId}
                token={token}
                lastRotatedAt={detail.events.find(e => e.kind === 'key_rotated')?.createdAt}
                notify={notify}
                openModal={setModal}
                onRotated={reloadDetail}
                sandboxEnabled={detail.sandboxEnabled}
                sandboxGatewayUrl={detail.sandboxGatewayUrl}
                sandboxEligible={subs.some(s => s.sandboxAvailable)}
                oauthEligible={detail.oauthEligible}
                oidcClientId={detail.oidcClientId}
                oidcIssuer={detail.oidcIssuer}
              />
            )}
            {tab === 'subs' && <SubscriptionsTab subs={subs} plans={plans} onResiliate={onResiliate} />}
            {tab === 'usage' && <UsageTab token={token} appId={appId} />}
            {tab === 'settings' && <SettingsTab app={app} notify={notify} openModal={setModal} />}
          </>
        )}
      </div>

      <Toast msg={toastMsg} />
      <ConfirmModal spec={modal} onClose={() => setModal(null)} />
      <CreateAppModal open={createOpen} onClose={() => setCreateOpen(false)} onCreate={onCreateApp} />
    </>
  )
}
