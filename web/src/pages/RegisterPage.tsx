import { useState, type FormEvent } from 'react'
import { useNavigate, Link } from 'react-router-dom'
import { useAuth } from '../auth/AuthProvider'

export function RegisterPage() {
  const { register } = useAuth()
  const nav = useNavigate()
  const [email, setEmail] = useState('')
  const [name, setName] = useState('')
  const [password, setPassword] = useState('')
  const [err, setErr] = useState('')

  async function onSubmit(e: FormEvent) {
    e.preventDefault(); setErr('')
    if (password.length < 8) { setErr('Mot de passe : 8 caractères minimum'); return }
    try { await register(email, password, name); nav('/') }
    catch (e) { setErr(e instanceof Error ? e.message : "Échec de l'inscription") }
  }

  return (
    <form className="authcard" onSubmit={onSubmit}>
      <h1>Créer un compte</h1>
      <label>Nom<input aria-label="Nom" value={name} onChange={e => setName(e.target.value)} /></label>
      <label>Email<input aria-label="Email" type="email" value={email} onChange={e => setEmail(e.target.value)} required /></label>
      <label>Mot de passe<input aria-label="Mot de passe" type="password" value={password} onChange={e => setPassword(e.target.value)} required /></label>
      {err && <p className="autherr" role="alert">{err}</p>}
      <button className="subbtn" type="submit">Créer le compte</button>
      <p>Déjà inscrit ? <Link to="/login">Se connecter</Link></p>
    </form>
  )
}
