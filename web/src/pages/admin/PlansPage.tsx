import { useCallback, useEffect, useRef, useState } from 'react'
import type { Plan } from '../../api/types'
import { adminGetPlans, adminCreatePlan, adminUpdatePlan, adminDeletePlan, ApiError } from '../../api/client'
import { useAuth } from '../../auth/AuthProvider'
import { AdminShell } from './AdminShell'
import { Composer } from './Composer'
import { planRate, planPreview } from './meta'
import { Toast, useToast } from '../../components/Toast'
import { Pagination } from '../../components/Pagination'
import { ConfirmModal, type ModalSpec } from '../../components/ConfirmModal'
import { useT, useLang } from '../../i18n/LanguageProvider'
import { priceLabel } from '../../money'

const TIERS = ['Free', 'Silver', 'Gold']

function PlusIcon() {
  return <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} aria-hidden="true"><path d="M12 5v14M5 12h14" strokeLinecap="round" /></svg>
}

export function PlansPage() {
  const { token } = useAuth()
  const t = useT()
  const { lang } = useLang()
  const [plans, setPlans] = useState<Plan[]>([])
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const pageSize = 20
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<Plan | null>(null)
  const [name, setName] = useState('')
  const [limit, setLimit] = useState(100)
  const [windowS, setWindowS] = useState(60)
  const [priceCents, setPriceCents] = useState(0)
  const [currency, setCurrency] = useState('EUR')
  const [modal, setModal] = useState<ModalSpec | null>(null)
  const [err, setErr] = useState('')
  const { toast, notify } = useToast()

  // Monotonic guard: only the latest list request may write state.
  const reqSeq = useRef(0)
  const reload = useCallback(() => {
    if (!token) return
    const seq = ++reqSeq.current
    adminGetPlans(token, { page, pageSize })
      .then(r => { if (seq === reqSeq.current) { setPlans(r.items); setTotal(r.total) } })
      .catch(() => { if (seq === reqSeq.current) setErr(t('admin.loadPlansError')) })
  }, [token, page])
  useEffect(reload, [reload])

  function openCreate() { setEditing(null); setName(''); setLimit(100); setWindowS(60); setPriceCents(0); setCurrency('EUR'); setOpen(true) }
  function openEdit(p: Plan) { setEditing(p); setName(p.name); setLimit(p.rateLimit); setWindowS(p.windowSeconds); setPriceCents(p.priceCents); setCurrency(p.currency); setOpen(true) }

  async function submit() {
    if (!token || !name.trim()) return
    const payload: Plan = { id: editing?.id ?? 0, name: name.trim(), rateLimit: limit || 100, windowSeconds: windowS || 60, priceCents: priceCents || 0, currency: (currency || 'EUR').toUpperCase() }
    try {
      if (editing) { await adminUpdatePlan(token, editing.id, payload); notify(t('admin.planSavedNotify', { name: payload.name })) }
      else { await adminCreatePlan(token, payload); notify(t('admin.planCreatedNotify', { name: payload.name })) }
      setOpen(false)
      reload()
    } catch (e) { notify(e instanceof Error ? e.message : t('admin.saveFailed'), 'warn') }
  }

  function askDelete(p: Plan) {
    setModal({
      title: t('admin.deletePlanTitle'),
      body: t('admin.deletePlanBody', { name: p.name, rateLimit: p.rateLimit, windowSeconds: p.windowSeconds }),
      confirmLabel: t('common.delete'), danger: true,
      onConfirm: () => {
        if (!token) return
        adminDeletePlan(token, p.id)
          .then(() => { notify(t('admin.planDeletedNotify', { name: p.name }), 'warn'); reload() })
          .catch(e => notify(
            e instanceof ApiError && e.status === 409
              ? t('admin.deletePlanConflict')
              : t('admin.deleteFailed'),
            'warn'))
      },
    })
  }

  return (
    <AdminShell
      active="plans"
      title={t('admin.plansPageTitle')}
      description={<>{t('admin.plansDescriptionPre')}<code>limit-count</code>{t('admin.plansDescriptionPost')}</>}
      counts={{ plans: plans.length }}
      action={
        <button className="btn btn-primary" onClick={() => open ? setOpen(false) : openCreate()}>
          <PlusIcon />{t('admin.newPlanCta')}
        </button>
      }
    >
      {err && <p className="autherr" role="alert">{err}</p>}

      <Composer
        open={open}
        title={editing ? t('admin.editPlanTitle') : t('admin.createPlanTitle')}
        hint={t('admin.planComposerHint')}
        submitLabel={editing ? t('common.save') : t('admin.createPlanCta')}
        onSubmit={submit}
        onCancel={() => setOpen(false)}
        footLeft={<span className="preview">{planPreview(limit, windowS, t)}</span>}
      >
        <div className="grid2 plans">
          <div className="field">
            <label htmlFor="p-name">{t('admin.planNameLabel')}</label>
            <input id="p-name" className="ipt" placeholder={t('admin.planNamePlaceholderEx')} autoComplete="off" autoFocus
              value={name} onChange={e => setName(e.target.value)} />
          </div>
          <div className="field">
            <label htmlFor="p-limit">{t('admin.limitLabel')} <span className="opt">{t('admin.requestsUnit')}</span></label>
            <input id="p-limit" className="ipt mono" type="number" min={1}
              value={limit} onChange={e => setLimit(Number(e.target.value))} />
          </div>
          <div className="field">
            <label htmlFor="p-window">{t('admin.windowLabel')} <span className="opt">{t('admin.secondsUnit')}</span></label>
            <input id="p-window" className="ipt mono" type="number" min={1}
              value={windowS} onChange={e => setWindowS(Number(e.target.value))} />
          </div>
          <div className="field">
            <label htmlFor="p-price">{t('admin.plan.priceLabel')}</label>
            <input id="p-price" className="ipt mono" type="number" min={0}
              value={priceCents} onChange={e => setPriceCents(Number(e.target.value))} />
          </div>
          <div className="field">
            <label htmlFor="p-currency">{t('admin.plan.currencyLabel')}</label>
            <input id="p-currency" className="ipt mono" maxLength={3}
              value={currency} onChange={e => setCurrency(e.target.value.toUpperCase())} />
          </div>
        </div>
      </Composer>

      <div className="list-head"><h3>{t('admin.availablePlansHeading')}</h3></div>
      <div className="rows">
        {plans.length === 0 && (
          <div className="aempty"><h4>{t('admin.noPlansHeading')}</h4><p>{t('admin.createFirstPlanHint')}</p></div>
        )}
        {plans.map(p => (
          <div className={`row plan ${TIERS.includes(p.name) ? `tier-${p.name}` : ''}`} key={p.id}>
            <div className="swatch">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="M13 2L3 14h7l-1 8 10-12h-7z" /></svg>
            </div>
            <div className="main">
              <div className="nm"><b>{p.name}</b><span className="limit">{p.rateLimit} req / {p.windowSeconds}s</span><span className="price">{priceLabel(p.priceCents, p.currency, lang, t('billing.free'), t('billing.perMonthSuffix'))}</span></div>
              <div className="meta">{planRate(p.rateLimit, p.windowSeconds, t)}{t('admin.policySuffix')}<span className="up">limit-count</span></div>
            </div>
            <div className="actions">
              <button className="iact" title={t('admin.editAction')} aria-label={t('admin.editAction')} onClick={() => openEdit(p)}>
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="M12 20h9M16.5 3.5a2.1 2.1 0 013 3L7 19l-4 1 1-4z" /></svg>
              </button>
              <button className="iact del" title={t('common.delete')} aria-label={t('common.delete')} onClick={() => askDelete(p)}>
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="M3 6h18M8 6V4a2 2 0 012-2h4a2 2 0 012 2v2m2 0v14a2 2 0 01-2 2H7a2 2 0 01-2-2V6" /></svg>
              </button>
            </div>
          </div>
        ))}
      </div>
      <Pagination page={page} pageSize={pageSize} total={total} onPage={setPage} />

      <Toast msg={toast?.msg ?? null} kind={toast?.kind} />
      <ConfirmModal spec={modal} onClose={() => setModal(null)} />
    </AdminShell>
  )
}
