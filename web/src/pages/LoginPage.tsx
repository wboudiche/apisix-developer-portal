import { useState, type FormEvent } from 'react'
import { useNavigate, Link } from 'react-router-dom'
import { useAuth } from '../auth/AuthProvider'
import { AuthShell } from '../components/AuthShell'
import { useT } from '../i18n/LanguageProvider'
import { ApiError, resendVerification } from '../api/client'

export function EyeIcon({ off }: { off: boolean }) {
  return off ? (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.7} aria-hidden="true">
      <path d="M3 3l18 18" strokeLinecap="round" />
      <path d="M10.6 10.6a3 3 0 0 0 4.2 4.2M9.4 5.3A9.6 9.6 0 0 1 12 5c6.5 0 10 7 10 7a17 17 0 0 1-3.2 4M6.2 6.2A17 17 0 0 0 2 12s3.5 7 10 7a9.7 9.7 0 0 0 3.4-.6" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  ) : (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.7} aria-hidden="true">
      <path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7-10-7-10-7Z" strokeLinecap="round" strokeLinejoin="round" />
      <circle cx="12" cy="12" r="3" />
    </svg>
  )
}

export function EnterpriseRow() {
  const t = useT()
  return (
    <div className="enterprise">
      <span>{t('auth.teamMember')}</span>
      <a href="#">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} aria-hidden="true">
          <path d="M12 2 4 6v6c0 5 3.4 8.5 8 10 4.6-1.5 8-5 8-10V6l-8-4Z" strokeLinejoin="round" />
        </svg>
        {t('auth.ssoLogin')}
      </a>
    </div>
  )
}

export function LegalLine() {
  const t = useT()
  return (
    <p className="legal">
      {t('auth.legalPre')}<a href="#">{t('auth.legalTerms')}</a>{t('auth.legalMid')}<a href="#">{t('auth.legalPrivacy')}</a>.
    </p>
  )
}

export function LoginPage() {
  const t = useT()
  const { login } = useAuth()
  const nav = useNavigate()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [err, setErr] = useState('')
  const [showPw, setShowPw] = useState(false)
  const [loading, setLoading] = useState(false)
  const [unverified, setUnverified] = useState(false)
  const [resent, setResent] = useState(false)

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setErr('')
    setUnverified(false)
    setResent(false)
    setLoading(true)
    try {
      await login(email, password)
      nav('/')
    } catch (e) {
      if (e instanceof ApiError && e.status === 403) {
        setUnverified(true)
      }
      setErr(e instanceof Error ? e.message : t('auth.loginFailed'))
      setLoading(false)
    }
  }

  return (
    <AuthShell>
      <form onSubmit={onSubmit}>
        <div className="m-head">
          <h2>{t('auth.loginHeading')}</h2>
          <p>{t('auth.noAccountPrefix')}<Link to="/register">{t('auth.registerHeading')}</Link></p>
        </div>

        {err && <p className="form-err" role="alert">{err}</p>}
        {unverified && (
          <p className="form-err" role="alert">
            {resent ? t('auth.resendSent') : (
              <button
                type="button" className="linklike"
                onClick={async () => {
                  try { await resendVerification(email) } catch { /* uniform "sent" copy regardless of outcome — anti-enumeration posture */ }
                  setResent(true)
                }}
              >
                {t('auth.resendVerification')}
              </button>
            )}
          </p>
        )}

        <div className="field">
          <label htmlFor="login-email">{t('auth.emailLabel')}</label>
          <div className="wrap">
            <input
              id="login-email" aria-label={t('auth.emailAriaLabel')} type="email" placeholder={t('auth.emailPlaceholder')}
              autoComplete="email" required value={email} onChange={e => setEmail(e.target.value)}
            />
          </div>
        </div>

        <div className="field">
          <label htmlFor="login-pw">{t('auth.passwordLabel')}</label>
          <div className="wrap">
            <input
              id="login-pw" aria-label={t('auth.passwordLabel')} type={showPw ? 'text' : 'password'} placeholder="••••••••"
              autoComplete="current-password" required value={password} onChange={e => setPassword(e.target.value)}
            />
            <button
              type="button" className="pw-toggle"
              aria-label={showPw ? t('auth.hidePassword') : t('auth.showPassword')}
              onClick={() => setShowPw(s => !s)}
            >
              <EyeIcon off={showPw} />
            </button>
          </div>
        </div>

        <div className="row-between">
          <label className="remember"><input type="checkbox" /> {t('auth.rememberMe')}</label>
          <a className="forgot" href="#">{t('auth.forgotPassword')}</a>
        </div>

        <button type="submit" className={`submit ${loading ? 'loading' : ''}`} disabled={loading}>
          <span className="spin" /><span className="label">{loading ? t('auth.loggingIn') : t('auth.login')}</span>
        </button>

        <EnterpriseRow />
        <LegalLine />
      </form>
    </AuthShell>
  )
}
