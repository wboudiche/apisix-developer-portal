import { useEffect, useState, type FormEvent } from 'react'
import { Navigate, useNavigate } from 'react-router-dom'
import { getApplications, createApplication } from '../../api/client'
import { useAuth } from '../../auth/AuthProvider'
import { TopBar } from '../../components/TopBar'
import type { Application } from '../../api/types'
import '../../styles/appdetail.css'

export function ApplicationsIndex() {
  const { token } = useAuth()
  const nav = useNavigate()
  const [apps, setApps] = useState<Application[] | null>(null)
  const [name, setName] = useState('')
  const [err, setErr] = useState('')

  useEffect(() => {
    if (!token) return
    getApplications(token).then(setApps).catch(() => setErr('Impossible de charger les applications.'))
  }, [token])

  if (!token) return <Navigate to="/login" replace />

  async function onCreate(e: FormEvent) {
    e.preventDefault()
    if (!token || !name.trim()) return
    try {
      const a = await createApplication(token, name.trim(), '')
      nav(`/applications/${a.id}`)
    } catch {
      setErr("Création impossible. Réessayez.")
    }
  }

  if (apps && apps.length > 0) return <Navigate to={`/applications/${apps[0].id}`} replace />

  return (
    <>
      <TopBar search="" onSearch={() => {}} />
      <div className="appdetail">
        {err && <p className="autherr" role="alert">{err}</p>}
        {apps && apps.length === 0 && (
          <div className="dcard" style={{ maxWidth: 520, margin: '40px auto', padding: 26 }}>
            <h3 style={{ fontFamily: 'var(--font-display)', fontSize: 19, fontWeight: 700 }}>Créez votre première application</h3>
            <p style={{ fontSize: 14, color: 'var(--muted)', marginTop: 8, lineHeight: 1.5 }}>
              Une application porte sa clé d'API et ses abonnements aux API du catalogue.
            </p>
            <form onSubmit={onCreate} style={{ display: 'flex', gap: 10, marginTop: 18 }}>
              <input
                aria-label="Nom de la nouvelle application" placeholder="Nom de la nouvelle application"
                value={name} onChange={e => setName(e.target.value)}
                style={{ flex: 1, height: 40, padding: '0 12px', border: '1px solid var(--border-2)', borderRadius: 10, background: 'var(--bg)', color: 'var(--fg)' }}
              />
              <button className="btn primary" type="submit">Créer</button>
            </form>
          </div>
        )}
      </div>
    </>
  )
}
