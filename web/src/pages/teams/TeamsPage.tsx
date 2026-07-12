import { useEffect, useState, type FormEvent } from 'react'
import { Navigate } from 'react-router-dom'
import { TopBar } from '../../components/TopBar'
import { useAuth } from '../../auth/AuthProvider'
import {
  getTeams, createTeam, getTeamMembers, addTeamMember, removeTeamMember, deleteTeam,
} from '../../api/client'
import type { Team, TeamMember } from '../../api/types'
import { useT } from '../../i18n/LanguageProvider'
import '../../styles/teams.css'

export default function TeamsPage() {
  const t = useT()
  const { token, user } = useAuth()
  const [teams, setTeams] = useState<Team[] | null>(null)
  const [selected, setSelected] = useState<Team | null>(null)
  const [name, setName] = useState('')
  const [err, setErr] = useState('')

  const reload = () => {
    if (!token) return
    getTeams(token).then(list => { setTeams(list); setErr('') }).catch(() => setErr(t('teams.loadError')))
  }
  useEffect(reload, [token])

  if (!token) return <Navigate to="/login" replace />

  const onCreate = async (e: FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return
    setErr('')
    try {
      await createTeam(token, name.trim())
      setName('')
      reload()
    } catch (x) { setErr((x as Error).message) }
  }

  return (
    <>
    <TopBar search="" onSearch={() => {}} />
    <div className="teams-page">
      <h1>{t('nav.teams')}</h1>
      {err && <p className="err">{err}</p>}
      <form onSubmit={onCreate} className="team-create">
        <input placeholder={t('teams.createNamePlaceholder')} value={name} onChange={e => setName(e.target.value)} />
        <button className="btn primary" type="submit">{t('app.create')}</button>
      </form>
      <ul className="team-list">
        {teams?.map(team => (
          <li key={team.id}>
            <button className="team-row" onClick={() => setSelected(team)}>
              <b>{team.name}</b>
              {team.personal && <span className="pill">{t('teams.personal')}</span>}
              <span className="team-role">{team.role === 'owner' ? t('teams.roleOwner') : t('teams.roleMember')}</span>
              <span className="team-count">{t(team.memberCount > 1 ? 'teams.memberCount_other' : 'teams.memberCount_one', { n: team.memberCount })}</span>
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
          onDeleted={() => { setSelected(null); reload() }}
        />
      )}
    </div>
    </>
  )
}

function TeamDetail({ team, token, meId, onChanged, onDeleted }: { team: Team; token: string; meId: number; onChanged: () => void; onDeleted: () => void }) {
  const t = useT()
  const [members, setMembers] = useState<TeamMember[] | null>(null)
  const [email, setEmail] = useState('')
  const [err, setErr] = useState('')
  const canManage = team.role === 'owner' && !team.personal

  const reload = () => { getTeamMembers(token, team.id).then(setMembers).catch(() => setErr(t('teams.membersLoadError'))) }
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
    try { await deleteTeam(token, team.id); onDeleted() }
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
            <span className="team-role">{m.role === 'owner' ? t('teams.roleOwner') : t('teams.roleMember')}</span>
            {canManage && m.userId !== meId && m.role !== 'owner' && (
              <button className="btn ghost" onClick={() => onRemove(m.userId)}>{t('teams.remove')}</button>
            )}
          </li>
        ))}
      </ul>
      {canManage && (
        <>
          <form onSubmit={onAdd} className="member-add">
            <input placeholder={t('teams.addMemberEmailPlaceholder')} value={email} onChange={e => setEmail(e.target.value)} />
            <button className="btn" type="submit">{t('common.add')}</button>
          </form>
          <button className="btn danger" onClick={onDelete}>{t('teams.deleteTeam')}</button>
        </>
      )}
    </div>
  )
}
