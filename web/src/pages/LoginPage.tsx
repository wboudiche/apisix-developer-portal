import { useState, type FormEvent } from 'react'
import { useNavigate, Link } from 'react-router-dom'
import { useAuth } from '../auth/AuthProvider'
import { AuthShell } from '../components/AuthShell'

export function EyeIcon({ off }: { off: boolean }) {
  return off ? (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.7}>
      <path d="M3 3l18 18" strokeLinecap="round" />
      <path d="M10.6 10.6a3 3 0 0 0 4.2 4.2M9.4 5.3A9.6 9.6 0 0 1 12 5c6.5 0 10 7 10 7a17 17 0 0 1-3.2 4M6.2 6.2A17 17 0 0 0 2 12s3.5 7 10 7a9.7 9.7 0 0 0 3.4-.6" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  ) : (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.7}>
      <path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7-10-7-10-7Z" strokeLinecap="round" strokeLinejoin="round" />
      <circle cx="12" cy="12" r="3" />
    </svg>
  )
}

export function EnterpriseRow() {
  return (
    <div className="enterprise">
      <span>Membre d'une équipe ?</span>
      <a href="#">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8}>
          <path d="M12 2 4 6v6c0 5 3.4 8.5 8 10 4.6-1.5 8-5 8-10V6l-8-4Z" strokeLinejoin="round" />
        </svg>
        Se connecter via votre entreprise
      </a>
    </div>
  )
}

export function LegalLine() {
  return (
    <p className="legal">
      En continuant, vous acceptez nos <a href="#">Conditions</a> et notre <a href="#">Politique de confidentialité</a>.
    </p>
  )
}

export function LoginPage() {
  const { login } = useAuth()
  const nav = useNavigate()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [err, setErr] = useState('')
  const [showPw, setShowPw] = useState(false)
  const [loading, setLoading] = useState(false)

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setErr('')
    setLoading(true)
    try {
      await login(email, password)
      nav('/')
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Échec de connexion')
      setLoading(false)
    }
  }

  return (
    <AuthShell>
      <form onSubmit={onSubmit}>
        <div className="m-head">
          <h2>Bon retour</h2>
          <p>Pas encore de compte ? <Link to="/register">Créer un compte</Link></p>
        </div>

        {err && <p className="form-err" role="alert">{err}</p>}

        <div className="field">
          <label htmlFor="login-email">Adresse email</label>
          <div className="wrap">
            <input
              id="login-email" aria-label="Email" type="email" placeholder="vous@entreprise.com"
              autoComplete="email" required value={email} onChange={e => setEmail(e.target.value)}
            />
          </div>
        </div>

        <div className="field">
          <label htmlFor="login-pw">Mot de passe</label>
          <div className="wrap">
            <input
              id="login-pw" aria-label="Mot de passe" type={showPw ? 'text' : 'password'} placeholder="••••••••"
              autoComplete="current-password" required value={password} onChange={e => setPassword(e.target.value)}
            />
            <button
              type="button" className="pw-toggle"
              aria-label={showPw ? 'Masquer le mot de passe' : 'Afficher le mot de passe'}
              onClick={() => setShowPw(s => !s)}
            >
              <EyeIcon off={showPw} />
            </button>
          </div>
        </div>

        <div className="row-between">
          <label className="remember"><input type="checkbox" /> Rester connecté</label>
          <a className="forgot" href="#">Mot de passe oublié ?</a>
        </div>

        <button type="submit" className={`submit ${loading ? 'loading' : ''}`} disabled={loading}>
          <span className="spin" /><span className="label">{loading ? 'Connexion…' : 'Se connecter'}</span>
        </button>

        <EnterpriseRow />
        <LegalLine />
      </form>
    </AuthShell>
  )
}
