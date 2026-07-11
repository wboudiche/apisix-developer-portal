import { useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { AuthShell } from '../components/AuthShell'
import { useT } from '../i18n/LanguageProvider'
import { verifyEmail, resendVerification } from '../api/client'

type State = 'loading' | 'success' | 'invalid'

export function VerifyEmailPage() {
  const t = useT()
  const [params] = useSearchParams()
  const token = params.get('token') ?? ''
  const [state, setState] = useState<State>(token ? 'loading' : 'invalid')
  const [email, setEmail] = useState('')
  const [resent, setResent] = useState(false)

  useEffect(() => {
    if (!token) return
    let cancelled = false
    verifyEmail(token)
      .then(() => { if (!cancelled) setState('success') })
      .catch(() => { if (!cancelled) setState('invalid') })
    return () => { cancelled = true }
  }, [token])

  return (
    <AuthShell>
      <div className="m-head">
        {state === 'loading' && <p>{t('auth.verifying')}</p>}
        {state === 'success' && (
          <>
            <h2>{t('auth.verifySuccessTitle')}</h2>
            <p>{t('auth.verifySuccessBody')}</p>
            <p><Link to="/login">{t('auth.login')}</Link></p>
          </>
        )}
        {state === 'invalid' && (
          <>
            <h2>{t('auth.verifyFailedTitle')}</h2>
            <p>{t('auth.verifyFailedBody')}</p>
            {resent ? <p>{t('auth.resendSent')}</p> : (
              <form onSubmit={async e => {
                e.preventDefault()
                try { await resendVerification(email) } catch { /* endpoint always answers 204 */ }
                setResent(true)
              }}>
                <div className="field">
                  <div className="wrap">
                    <input type="email" required placeholder={t('auth.emailPlaceholder')}
                      aria-label={t('auth.emailAriaLabel')}
                      value={email} onChange={e => setEmail(e.target.value)} />
                  </div>
                </div>
                <button type="submit" className="submit"><span className="label">{t('auth.resendVerification')}</span></button>
              </form>
            )}
            <p><Link to="/login">{t('auth.login')}</Link></p>
          </>
        )}
      </div>
    </AuthShell>
  )
}
