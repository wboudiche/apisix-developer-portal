import { useEffect, useState, type FormEvent } from 'react'
import { Link, Navigate, useNavigate } from 'react-router-dom'
import { getApplications, createApplication, getTeams } from '../../api/client'
import { useAuth } from '../../auth/AuthProvider'
import { TopBar } from '../../components/TopBar'
import type { Application, Team } from '../../api/types'
import { appRef, initials, frDate, glyphGradient } from './helpers'
import '../../styles/appdetail.css'

export function ApplicationsIndex() {
  const { token } = useAuth()
  const nav = useNavigate()
  const [apps, setApps] = useState<Application[] | null>(null)
  const [name, setName] = useState('')
  const [creating, setCreating] = useState(false)
  const [err, setErr] = useState('')
  const [teams, setTeams] = useState<Team[]>([])
  const [teamId, setTeamId] = useState<number | ''>('')

  useEffect(() => {
    if (!token) return
    getApplications(token).then(r => setApps(r.items)).catch(() => setErr('Impossible de charger les applications.'))
    getTeams(token).then(ts => {
      setTeams(ts)
      const personal = ts.find(t => t.personal)
      if (personal) setTeamId(personal.id)
    }).catch(() => {})
  }, [token])

  if (!token) return <Navigate to="/login" replace />

  async function onCreate(e: FormEvent) {
    e.preventDefault()
    if (!token || !name.trim()) return
    try {
      const a = await createApplication(token, name.trim(), '', typeof teamId === 'number' ? teamId : undefined)
      nav(`/applications/${a.id}`)
    } catch {
      setErr('Création impossible. Réessayez.')
    }
  }

  const createForm = (
    <form onSubmit={onCreate} className="applist-create">
      <input
        aria-label="Nom de la nouvelle application" placeholder="Nom de la nouvelle application"
        value={name} onChange={e => setName(e.target.value)} autoFocus
      />
      <label htmlFor="app-team">Équipe</label>
      <select id="app-team" value={teamId} onChange={e => setTeamId(Number(e.target.value))}>
        {teams.map(t => <option key={t.id} value={t.id}>{t.name}{t.personal ? ' (personnelle)' : ''}</option>)}
      </select>
      <button className="btn primary" type="submit">Créer</button>
    </form>
  )

  return (
    <>
      <TopBar search="" onSearch={() => {}} />
      <div className="appdetail">
        <div className="applist-head">
          <div>
            <h1 className="applist-title">Applications</h1>
            <p className="applist-sub">Chaque application porte sa clé d’API et ses abonnements aux API du catalogue.</p>
          </div>
          {apps && apps.length > 0 && (
            <button className="btn primary" onClick={() => setCreating(c => !c)}>+ Nouvelle application</button>
          )}
        </div>

        {err && <p className="autherr" role="alert">{err}</p>}

        {apps && apps.length > 0 && creating && createForm}

        {apps && apps.length === 0 && (
          <div className="dcard" style={{ maxWidth: 520, margin: '40px auto', padding: 26 }}>
            <h3 style={{ fontFamily: 'var(--font-display)', fontSize: 19, fontWeight: 700 }}>Créez votre première application</h3>
            <p style={{ fontSize: 14, color: 'var(--muted)', marginTop: 8, lineHeight: 1.5 }}>
              Une application porte sa clé d’API et ses abonnements aux API du catalogue.
            </p>
            <div style={{ marginTop: 18 }}>{createForm}</div>
          </div>
        )}

        {apps && apps.length > 0 && (
          <div className="applist">
            {apps.map(a => {
              const subs = a.subscriptionCount ?? 0
              return (
                <Link key={a.id} to={`/applications/${a.id}`} className="app-card">
                  <span className="mg" style={{ background: glyphGradient(a.id) }}>{initials(a.name)}</span>
                  <div className="ac-main">
                    <div className="ac-top">
                      <b>{a.name}</b>
                      <span className="ac-ref mono">{appRef(a.id)}</span>
                    </div>
                    {a.description && <div className="ac-desc">{a.description}</div>}
                    <div className="ac-meta">
                      <span>{subs} abonnement{subs > 1 ? 's' : ''}</span>
                      <span className="ac-sep">·</span>
                      <span>Créée le <span className="mono">{frDate(a.createdAt)}</span></span>
                      {a.teamName && <span className="pill team">{a.teamName}</span>}
                    </div>
                  </div>
                  <span className="ac-go">Ouvrir →</span>
                </Link>
              )
            })}
          </div>
        )}
      </div>
    </>
  )
}
