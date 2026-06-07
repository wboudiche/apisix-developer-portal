import { useCallback, useEffect, useState } from 'react'
import type { Plan } from '../../api/types'
import { adminGetPlans, adminCreatePlan, adminUpdatePlan, adminDeletePlan, ApiError } from '../../api/client'
import { useAuth } from '../../auth/AuthProvider'
import { AdminShell } from './AdminShell'
import { Composer } from './Composer'
import { planRate, planPreview } from './meta'
import { Toast, useToast } from '../../components/Toast'
import { ConfirmModal, type ModalSpec } from '../../components/ConfirmModal'

const TIERS = ['Free', 'Silver', 'Gold']

function PlusIcon() {
  return <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} aria-hidden="true"><path d="M12 5v14M5 12h14" strokeLinecap="round" /></svg>
}

export function PlansPage() {
  const { token } = useAuth()
  const [plans, setPlans] = useState<Plan[]>([])
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<Plan | null>(null)
  const [name, setName] = useState('')
  const [limit, setLimit] = useState(100)
  const [windowS, setWindowS] = useState(60)
  const [modal, setModal] = useState<ModalSpec | null>(null)
  const [err, setErr] = useState('')
  const { toast, notify } = useToast()

  const reload = useCallback(() => {
    if (!token) return
    adminGetPlans(token).then(setPlans).catch(() => setErr('Impossible de charger les plans.'))
  }, [token])
  useEffect(reload, [reload])

  function openCreate() { setEditing(null); setName(''); setLimit(100); setWindowS(60); setOpen(true) }
  function openEdit(p: Plan) { setEditing(p); setName(p.name); setLimit(p.rateLimit); setWindowS(p.windowSeconds); setOpen(true) }

  async function submit() {
    if (!token || !name.trim()) return
    const payload: Plan = { id: editing?.id ?? 0, name: name.trim(), rateLimit: limit || 100, windowSeconds: windowS || 60 }
    try {
      if (editing) { await adminUpdatePlan(token, editing.id, payload); notify(`Plan ${payload.name} enregistré`) }
      else { await adminCreatePlan(token, payload); notify(`Plan ${payload.name} créé`) }
      setOpen(false)
      reload()
    } catch (e) { notify(e instanceof Error ? e.message : "Échec de l'enregistrement.", 'warn') }
  }

  function askDelete(p: Plan) {
    setModal({
      title: 'Supprimer le plan ?',
      body: `Le plan ${p.name} (${p.rateLimit} req/${p.windowSeconds}s) ne pourra plus être choisi pour de nouveaux abonnements.`,
      confirmLabel: 'Supprimer', danger: true,
      onConfirm: () => {
        if (!token) return
        adminDeletePlan(token, p.id)
          .then(() => { notify(`Plan ${p.name} supprimé`, 'warn'); reload() })
          .catch(e => notify(
            e instanceof ApiError && e.status === 409
              ? 'Suppression impossible : des abonnements utilisent ce plan.'
              : 'Échec de la suppression.',
            'warn'))
      },
    })
  }

  return (
    <AdminShell
      active="plans"
      title="Plans de débit"
      description={<>Chaque plan applique une politique <code>limit-count</code> : un nombre de requêtes autorisé sur une fenêtre glissante. Les abonnements lient une application à une API selon un plan.</>}
      counts={{ plans: plans.length }}
      action={
        <button className="btn btn-primary" onClick={() => open ? setOpen(false) : openCreate()}>
          <PlusIcon />Nouveau plan
        </button>
      }
    >
      {err && <p className="autherr" role="alert">{err}</p>}

      <Composer
        open={open}
        title={editing ? 'Modifier le plan' : 'Créer un plan'}
        hint="mappé sur limit-count à la sauvegarde"
        submitLabel={editing ? 'Enregistrer' : 'Créer le plan'}
        onSubmit={submit}
        onCancel={() => setOpen(false)}
        footLeft={<span className="preview">{planPreview(limit, windowS)}</span>}
      >
        <div className="grid2 plans">
          <div className="field">
            <label htmlFor="p-name">Nom du plan</label>
            <input id="p-name" className="ipt" placeholder="Platinum" autoComplete="off" autoFocus
              value={name} onChange={e => setName(e.target.value)} />
          </div>
          <div className="field">
            <label htmlFor="p-limit">Limite <span className="opt">requêtes</span></label>
            <input id="p-limit" className="ipt mono" type="number" min={1}
              value={limit} onChange={e => setLimit(Number(e.target.value))} />
          </div>
          <div className="field">
            <label htmlFor="p-window">Fenêtre <span className="opt">secondes</span></label>
            <input id="p-window" className="ipt mono" type="number" min={1}
              value={windowS} onChange={e => setWindowS(Number(e.target.value))} />
          </div>
        </div>
      </Composer>

      <div className="list-head"><h3>Plans disponibles</h3></div>
      <div className="rows">
        {plans.length === 0 && (
          <div className="aempty"><h4>Aucun plan</h4><p>Créez un premier plan de débit.</p></div>
        )}
        {plans.map(p => (
          <div className={`row plan ${TIERS.includes(p.name) ? `tier-${p.name}` : ''}`} key={p.id}>
            <div className="swatch">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="M13 2L3 14h7l-1 8 10-12h-7z" /></svg>
            </div>
            <div className="main">
              <div className="nm"><b>{p.name}</b><span className="limit">{p.rateLimit} req / {p.windowSeconds}s</span></div>
              <div className="meta">{planRate(p.rateLimit, p.windowSeconds)} · politique <span className="up">limit-count</span></div>
            </div>
            <div className="actions">
              <button className="iact" title="Modifier" aria-label="Modifier" onClick={() => openEdit(p)}>
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="M12 20h9M16.5 3.5a2.1 2.1 0 013 3L7 19l-4 1 1-4z" /></svg>
              </button>
              <button className="iact del" title="Supprimer" aria-label="Supprimer" onClick={() => askDelete(p)}>
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="M3 6h18M8 6V4a2 2 0 012-2h4a2 2 0 012 2v2m2 0v14a2 2 0 01-2 2H7a2 2 0 01-2-2V6" /></svg>
              </button>
            </div>
          </div>
        ))}
      </div>

      <Toast msg={toast?.msg ?? null} kind={toast?.kind} />
      <ConfirmModal spec={modal} onClose={() => setModal(null)} />
    </AdminShell>
  )
}
