import { useState, type FormEvent } from 'react'
import { useNavigate, Link } from 'react-router-dom'
import { useAuth } from '../auth/AuthProvider'
import { AuthShell } from '../components/AuthShell'
import { EyeIcon, EnterpriseRow, LegalLine } from './LoginPage'

export function RegisterPage() {
  const { register } = useAuth()
  const nav = useNavigate()
  const [name, setName] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [err, setErr] = useState('')
  const [pwErr, setPwErr] = useState('')
  const [showPw, setShowPw] = useState(false)
  const [loading, setLoading] = useState(false)

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setErr('')
    setPwErr('')
    if (password.length < 8) {
      setPwErr('Mot de passe : 8 caractères minimum')
      return
    }
    setLoading(true)
    try {
      await register(email, password, name)
      nav('/')
    } catch (e) {
      setErr(e instanceof Error ? e.message : "Échec de l'inscription")
      setLoading(false)
    }
  }

  return (
    <AuthShell>
      <form onSubmit={onSubmit}>
        <div className="m-head">
          <h2>Créer un compte</h2>
          <p>Déjà inscrit ? <Link to="/login">Se connecter</Link></p>
        </div>

        {err && <p className="form-err" role="alert">{err}</p>}

        <div className="field">
          <label htmlFor="reg-name">Nom</label>
          <div className="wrap">
            <input
              id="reg-name" aria-label="Nom" placeholder="Prénom Nom"
              autoComplete="name" value={name} onChange={e => setName(e.target.value)}
            />
          </div>
        </div>

        <div className="field">
          <label htmlFor="reg-email">Adresse email</label>
          <div className="wrap">
            <input
              id="reg-email" aria-label="Email" type="email" placeholder="vous@entreprise.com"
              autoComplete="email" required value={email} onChange={e => setEmail(e.target.value)}
            />
          </div>
        </div>

        <div className={`field ${pwErr ? 'invalid' : ''}`}>
          <label htmlFor="reg-pw">Mot de passe</label>
          <div className="wrap">
            <input
              id="reg-pw" aria-label="Mot de passe" type={showPw ? 'text' : 'password'} placeholder="8 caractères minimum"
              autoComplete="new-password" required value={password}
              onChange={e => { setPassword(e.target.value); if (e.target.value.length >= 8) setPwErr('') }}
            />
            <button
              type="button" className="pw-toggle"
              aria-label={showPw ? 'Masquer le mot de passe' : 'Afficher le mot de passe'}
              onClick={() => setShowPw(s => !s)}
            >
              <EyeIcon off={showPw} />
            </button>
          </div>
          <div className="err">{pwErr}</div>
        </div>

        <button type="submit" className={`submit ${loading ? 'loading' : ''}`} disabled={loading}>
          <span className="spin" /><span className="label">{loading ? 'Création…' : 'Créer le compte'}</span>
        </button>

        <EnterpriseRow />
        <LegalLine />
      </form>
    </AuthShell>
  )
}
