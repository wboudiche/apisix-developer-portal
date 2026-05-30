import { useEffect, useState } from 'react'
import type { Plan } from '../api/types'
import { adminGetPlans, adminCreatePlan, adminUpdatePlan, adminDeletePlan } from '../api/client'
import { useAuth } from '../auth/AuthProvider'
import { AdminNav } from '../components/AdminNav'
import '../styles/catalog.css'

const emptyPlan: Plan = { id: 0, name: '', rateLimit: 100, windowSeconds: 60 }
const field: React.CSSProperties = { height: 38, padding: '0 12px', border: '1px solid var(--border-2)', borderRadius: 10, background: 'var(--bg)', color: 'var(--fg)' }

export function AdminPlansPage() {
  const { token } = useAuth()
  const [plans, setPlans] = useState<Plan[]>([])
  const [form, setForm] = useState<Plan>(emptyPlan)
  const [editingId, setEditingId] = useState<number | null>(null)
  const [err, setErr] = useState('')

  function reload() {
    if (!token) return
    adminGetPlans(token).then(setPlans).catch(() => setErr('Impossible de charger les plans.'))
  }
  useEffect(reload, [token])

  async function onSubmit() {
    if (!token) return
    setErr('')
    try {
      if (editingId == null) await adminCreatePlan(token, form)
      else await adminUpdatePlan(token, editingId, form)
      setForm(emptyPlan); setEditingId(null); reload()
    } catch (e) { setErr(e instanceof Error ? e.message : "Échec de l'enregistrement.") }
  }

  async function onDelete(p: Plan) {
    if (!token) return
    setErr('')
    try { await adminDeletePlan(token, p.id); reload() }
    catch (e) { setErr(e instanceof Error ? e.message : "Échec de la suppression.") }
  }

  return (
    <>
      <AdminNav active="plans" />
      <div className="content">
        <div className="chead"><div className="titlewrap"><h1>Plans</h1></div></div>
        {err && <p className="autherr" role="alert">{err}</p>}

        <div className="card" style={{ padding: 18, marginBottom: 22, display: 'grid', gap: 10, gridTemplateColumns: '1fr 1fr 1fr' }}>
          <label style={{ display: 'grid', gap: 4 }}>Nom du plan<input aria-label="Nom du plan" style={field} value={form.name} onChange={e => setForm(f => ({ ...f, name: e.target.value }))} /></label>
          <label style={{ display: 'grid', gap: 4 }}>Limite (requêtes)<input aria-label="Limite (requêtes)" type="number" style={field} value={form.rateLimit} onChange={e => setForm(f => ({ ...f, rateLimit: Number(e.target.value) }))} /></label>
          <label style={{ display: 'grid', gap: 4 }}>Fenêtre (secondes)<input aria-label="Fenêtre (secondes)" type="number" style={field} value={form.windowSeconds} onChange={e => setForm(f => ({ ...f, windowSeconds: Number(e.target.value) }))} /></label>
          <div style={{ gridColumn: '1 / -1', display: 'flex', gap: 10 }}>
            <button className="subbtn" onClick={onSubmit}>{editingId == null ? 'Créer le plan' : 'Enregistrer'}</button>
            {editingId != null && <button className="subbtn ghost" onClick={() => { setForm(emptyPlan); setEditingId(null) }}>Annuler</button>}
          </div>
        </div>

        {plans.length === 0 && <p className="rescount">Aucun plan.</p>}
        {plans.map(p => (
          <div key={p.id} className="cfoot" style={{ justifyContent: 'space-between', padding: '10px 0', borderBottom: '1px solid var(--border)' }}>
            <span><b>{p.name}</b> · <span className="pill">{p.rateLimit} req / {p.windowSeconds}s</span></span>
            <span style={{ display: 'flex', gap: 8 }}>
              <button className="subbtn ghost" onClick={() => { setEditingId(p.id); setForm({ ...p }) }} aria-label={`Modifier ${p.name}`}>Modifier</button>
              <button className="subbtn ghost" onClick={() => onDelete(p)} aria-label={`Supprimer ${p.name}`}>Supprimer</button>
            </span>
          </div>
        ))}
      </div>
    </>
  )
}
