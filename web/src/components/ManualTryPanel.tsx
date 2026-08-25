import { useState } from 'react'
import { useT } from '../i18n/LanguageProvider'

const METHODS = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE'] as const

interface Result {
  status: number
  statusText: string
  headers: [string, string][]
  body: string
}

// Fallback "try it" tester shown on a product's page when no OpenAPI spec is
// attached (so ScalarDocs has nothing to render): a minimal method/path/
// headers/body request builder that hits the same tryit proxy ScalarDocs
// would use, sharing its base `serverUrl` and injected app credentials —
// see internal/tryit/handler.go, which already accepts an arbitrary method
// and path suffix regardless of whether a spec exists.
export function ManualTryPanel({ serverUrl, contextPath, token }: { serverUrl: string; contextPath: string; token: string }) {
  const t = useT()
  const [method, setMethod] = useState<(typeof METHODS)[number]>('GET')
  const [path, setPath] = useState('')
  const [headersText, setHeadersText] = useState('')
  const [body, setBody] = useState('')
  const [sending, setSending] = useState(false)
  const [error, setError] = useState('')
  const [result, setResult] = useState<Result | null>(null)

  // METHODS never offers HEAD as a selectable option, so GET is the only
  // no-body case reachable here.
  const hasBody = method !== 'GET'

  async function send() {
    setSending(true); setError(''); setResult(null)
    try {
      const suffix = path.trim()
      const url = serverUrl + (suffix ? (suffix.startsWith('/') ? suffix : `/${suffix}`) : '')
      const headers: Record<string, string> = {}
      for (const line of headersText.split('\n')) {
        const idx = line.indexOf(':')
        if (idx <= 0) continue
        const name = line.slice(0, idx).trim()
        // Never let a typed header override the injected portal token — the
        // gateway proxy authenticates the caller with it (see internal/tryit),
        // and a duplicate case-insensitive key would otherwise be comma-joined
        // onto the same header by fetch's Headers algorithm instead of cleanly
        // replaced.
        if (name.toLowerCase() === 'authorization') continue
        headers[name] = line.slice(idx + 1).trim()
      }
      headers.Authorization = `Bearer ${token}`
      const sendBody = hasBody && body.trim() !== ''
      if (sendBody && !Object.keys(headers).some(h => h.toLowerCase() === 'content-type')) {
        headers['Content-Type'] = 'application/json'
      }
      const res = await fetch(url, { method, headers, body: sendBody ? body : undefined })
      const text = await res.text()
      setResult({ status: res.status, statusText: res.statusText, headers: [...res.headers.entries()], body: text })
    } catch {
      setError(t('product.tryManualSendFailed'))
    } finally {
      setSending(false)
    }
  }

  return (
    <div className="manual-try">
      <h3>{t('product.tryManualHeading')}</h3>
      <p className="mt-hint">{t('product.tryManualHint', { contextPath })}</p>
      <div className="mt-row">
        <select className="mono" aria-label={t('product.tryManualMethodLabel')} value={method}
          onChange={e => setMethod(e.target.value as (typeof METHODS)[number])}>
          {METHODS.map(m => <option key={m} value={m}>{m}</option>)}
        </select>
        <span className="mt-base mono">{serverUrl}</span>
        <input className="mt-path mono" aria-label={t('product.tryManualPathLabel')}
          placeholder={t('product.tryManualPathPlaceholder')} value={path} onChange={e => setPath(e.target.value)} />
        <button type="button" className="btn btn-primary" onClick={send} disabled={sending}>
          {sending ? t('product.tryManualSending') : t('product.tryManualSend')}
        </button>
      </div>
      <details className="mt-headers">
        <summary>{t('product.tryManualHeadersHeading')}</summary>
        <textarea className="mono" rows={3} value={headersText} onChange={e => setHeadersText(e.target.value)}
          placeholder={t('product.tryManualHeadersPlaceholder')} aria-label={t('product.tryManualHeadersHeading')} />
      </details>
      {hasBody && (
        <textarea className="mt-body mono" rows={5} value={body} onChange={e => setBody(e.target.value)}
          placeholder={t('product.tryManualBodyPlaceholder')} aria-label={t('product.tryManualBodyLabel')} />
      )}
      {error && <p className="autherr" role="alert">{error}</p>}
      {result && (
        <div className="mt-result">
          <div className={`mt-status ${result.status < 300 ? 'ok' : result.status >= 400 ? 'err' : ''}`}>
            {result.status} {result.statusText}
          </div>
          <pre className="mt-body-out mono">{result.body}</pre>
        </div>
      )}
    </div>
  )
}
