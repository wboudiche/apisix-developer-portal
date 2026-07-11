import { useEffect, useRef, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { AuthShell } from '../components/AuthShell'
import { useT } from '../i18n/LanguageProvider'
import { verifyEmail, resendVerification, ApiError } from '../api/client'

type State = 'loading' | 'success' | 'invalid' | 'error'

export function VerifyEmailPage() {
  const t = useT()
  const [params] = useSearchParams()
  const token = params.get('token') ?? ''
  const [state, setState] = useState<State>(token ? 'loading' : 'invalid')
  const [email, setEmail] = useState('')
  const [resent, setResent] = useState(false)
  // Dedupes the request per token: React 19 StrictMode double-invokes this
  // effect in dev, and the verification token is single-use server-side (the
  // second POST gets a 410), so without this guard a genuinely successful
  // verification could flip to "invalid" from the second, doomed request.
  // Guarding setState with attempted.current === token (rather than a
  // cancelled flag reset in cleanup) means the StrictMode remount's second
  // effect run just returns early, while the first run's in-flight promise
  // still lands and updates state normally.
  const attempted = useRef<string | null>(null)

  useEffect(() => {
    if (!token || attempted.current === token) return
    attempted.current = token
    setState('loading')
    verifyEmail(token)
      .then(() => { if (attempted.current === token) setState('success') })
      .catch((e) => {
        if (attempted.current !== token) return
        setState(e instanceof ApiError && e.status === 410 ? 'invalid' : 'error')
      })
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
        {state === 'error' && (
          <>
            <h2>{t('auth.verifyErrorTitle')}</h2>
            <p>{t('auth.verifyErrorBody')}</p>
            <p><Link to="/login">{t('auth.login')}</Link></p>
          </>
        )}
      </div>
    </AuthShell>
  )
}
