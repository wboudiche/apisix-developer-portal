import { useState, type FormEvent } from 'react'
import { useNavigate, Link } from 'react-router-dom'
import { useAuth } from '../auth/AuthProvider'
import { AuthShell } from '../components/AuthShell'
import { useT } from '../i18n/LanguageProvider'
import { EyeIcon, EnterpriseRow, LegalLine } from './LoginPage'

export function RegisterPage() {
  const t = useT()
  const { register } = useAuth()
  const nav = useNavigate()
  const [name, setName] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [err, setErr] = useState('')
  const [pwErr, setPwErr] = useState('')
  const [showPw, setShowPw] = useState(false)
  const [loading, setLoading] = useState(false)
  const [sent, setSent] = useState(false)

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setErr('')
    setPwErr('')
    if (password.length < 8) {
      setPwErr(t('auth.passwordMinLength'))
      return
    }
    setLoading(true)
    try {
      const needsVerification = await register(email, password, name)
      if (needsVerification) {
        setSent(true)
        setLoading(false)
      } else {
        nav('/')
      }
    } catch (e) {
      setErr(e instanceof Error ? e.message : t('auth.registerFailed'))
      setLoading(false)
    }
  }

  if (sent) {
    return (
      <AuthShell>
        <div className="m-head">
          <h2>{t('auth.checkInboxTitle')}</h2>
          <p>{t('auth.checkInboxBody')}</p>
          <p><Link to="/login">{t('auth.login')}</Link></p>
        </div>
      </AuthShell>
    )
  }

  return (
    <AuthShell>
      <form onSubmit={onSubmit}>
        <div className="m-head">
          <h2>{t('auth.registerHeading')}</h2>
          <p>{t('auth.alreadyRegisteredPrefix')}<Link to="/login">{t('auth.login')}</Link></p>
        </div>

        {err && <p className="form-err" role="alert">{err}</p>}

        <div className="field">
          <label htmlFor="reg-name">{t('auth.nameLabel')}</label>
          <div className="wrap">
            <input
              id="reg-name" aria-label={t('auth.nameLabel')} placeholder={t('auth.namePlaceholder')}
              autoComplete="name" value={name} onChange={e => setName(e.target.value)}
            />
          </div>
        </div>

        <div className="field">
          <label htmlFor="reg-email">{t('auth.emailLabel')}</label>
          <div className="wrap">
            <input
              id="reg-email" aria-label={t('auth.emailAriaLabel')} type="email" placeholder={t('auth.emailPlaceholder')}
              autoComplete="email" required value={email} onChange={e => setEmail(e.target.value)}
            />
          </div>
        </div>

        <div className={`field ${pwErr ? 'invalid' : ''}`}>
          <label htmlFor="reg-pw">{t('auth.passwordLabel')}</label>
          <div className="wrap">
            <input
              id="reg-pw" aria-label={t('auth.passwordLabel')} type={showPw ? 'text' : 'password'} placeholder={t('auth.passwordPlaceholderMin')}
              autoComplete="new-password" required value={password}
              onChange={e => { setPassword(e.target.value); if (pwErr) setPwErr('') }}
            />
            <button
              type="button" className="pw-toggle"
              aria-label={showPw ? t('auth.hidePassword') : t('auth.showPassword')}
              onClick={() => setShowPw(s => !s)}
            >
              <EyeIcon off={showPw} />
            </button>
          </div>
          <div className="err">{pwErr}</div>
        </div>

        <button type="submit" className={`submit ${loading ? 'loading' : ''}`} disabled={loading}>
          <span className="spin" /><span className="label">{loading ? t('auth.creatingAccount') : t('auth.createAccount')}</span>
        </button>

        <EnterpriseRow />
        <LegalLine />
      </form>
    </AuthShell>
  )
}
