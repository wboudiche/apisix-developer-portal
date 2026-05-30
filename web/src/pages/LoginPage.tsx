import { useState, type FormEvent } from 'react'
import { useNavigate, Link } from 'react-router-dom'
import { useAuth } from '../auth/AuthProvider'

export function LoginPage() {
  const { login } = useAuth()
  const nav = useNavigate()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [err, setErr] = useState('')

  async function onSubmit(e: FormEvent) {
    e.preventDefault(); setErr('')
    try { await login(email, password); nav('/') }
    catch (e) { setErr(e instanceof Error ? e.message : 'Échec de connexion') }
  }

  return (
    <form className="authcard" onSubmit={onSubmit}>
      <h1>Connexion</h1>
      <label>Email<input aria-label="Email" type="email" value={email} onChange={e => setEmail(e.target.value)} required /></label>
      <label>Mot de passe<input aria-label="Mot de passe" type="password" value={password} onChange={e => setPassword(e.target.value)} required /></label>
      {err && <p className="autherr" role="alert">{err}</p>}
      <button className="subbtn" type="submit">Connexion</button>
      <p>Pas de compte ? <Link to="/register">Créer un compte</Link></p>
    </form>
  )
}
