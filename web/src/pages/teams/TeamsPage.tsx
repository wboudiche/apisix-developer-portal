import { useEffect, useState, type FormEvent } from 'react'
import { Navigate } from 'react-router-dom'
import { useAuth } from '../../auth/AuthProvider'
import {
  getTeams, createTeam, getTeamMembers, addTeamMember, removeTeamMember, deleteTeam,
} from '../../api/client'
import type { Team, TeamMember } from '../../api/types'

export default function TeamsPage() {
  const { token, user } = useAuth()
  const [teams, setTeams] = useState<Team[] | null>(null)
  const [selected, setSelected] = useState<Team | null>(null)
  const [name, setName] = useState('')
  const [err, setErr] = useState('')

  const reload = () => {
    if (!token) return
    getTeams(token).then(setTeams).catch(() => setErr('Impossible de charger les équipes.'))
  }
  useEffect(reload, [token])

  if (!token) return <Navigate to="/login" replace />

  const onCreate = async (e: FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return
    try {
      await createTeam(token, name.trim())
      setName('')
      reload()
    } catch (x) { setErr((x as Error).message) }
  }

  return (
    <div className="teams-page">
      <h1>Équipes</h1>
      {err && <p className="err">{err}</p>}
      <form onSubmit={onCreate} className="team-create">
        <input placeholder="Nom de l'équipe" value={name} onChange={e => setName(e.target.value)} />
        <button className="btn primary" type="submit">Créer</button>
      </form>
      <ul className="team-list">
        {teams?.map(t => (
          <li key={t.id}>
            <button className="team-row" onClick={() => setSelected(t)}>
              <b>{t.name}</b>
              {t.personal && <span className="pill">Personnelle</span>}
              <span className="team-role">{t.role === 'owner' ? 'Propriétaire' : 'Membre'}</span>
              <span className="team-count">{t.memberCount} membre{t.memberCount > 1 ? 's' : ''}</span>
            </button>
          </li>
        ))}
      </ul>
      {selected && (
        <TeamDetail
          key={selected.id}
          team={selected}
          token={token}
          meId={user?.id ?? 0}
          onChanged={reload}
        />
      )}
    </div>
  )
}

function TeamDetail({ team, token, meId, onChanged }: { team: Team; token: string; meId: number; onChanged: () => void }) {
  const [members, setMembers] = useState<TeamMember[] | null>(null)
  const [email, setEmail] = useState('')
  const [err, setErr] = useState('')
  const canManage = team.role === 'owner' && !team.personal

  const reload = () => { getTeamMembers(token, team.id).then(setMembers).catch(() => setErr('Impossible de charger les membres.')) }
  useEffect(reload, [token, team.id])

  const onAdd = async (e: FormEvent) => {
    e.preventDefault()
    if (!email.trim()) return
    setErr('')
    try {
      await addTeamMember(token, team.id, email.trim())
      setEmail('')
      reload(); onChanged()
    } catch (x) { setErr((x as Error).message) }
  }
  const onRemove = async (userId: number) => {
    setErr('')
    try { await removeTeamMember(token, team.id, userId); reload(); onChanged() }
    catch (x) { setErr((x as Error).message) }
  }
  const onDelete = async () => {
    setErr('')
    try { await deleteTeam(token, team.id); onChanged() }
    catch (x) { setErr((x as Error).message) }
  }

  return (
    <div className="team-detail">
      <h2>{team.name}</h2>
      {err && <p className="err">{err}</p>}
      <ul className="member-list">
        {members?.map(m => (
          <li key={m.userId}>
            <span>{m.name} · <span className="mono">{m.email}</span></span>
            <span className="team-role">{m.role === 'owner' ? 'Propriétaire' : 'Membre'}</span>
            {canManage && m.userId !== meId && m.role !== 'owner' && (
              <button className="btn ghost" onClick={() => onRemove(m.userId)}>Retirer</button>
            )}
          </li>
        ))}
      </ul>
      {canManage && (
        <>
          <form onSubmit={onAdd} className="member-add">
            <input placeholder="Email d'un utilisateur" value={email} onChange={e => setEmail(e.target.value)} />
            <button className="btn" type="submit">Ajouter</button>
          </form>
          <button className="btn danger" onClick={onDelete}>Supprimer l'équipe</button>
        </>
      )}
    </div>
  )
}
