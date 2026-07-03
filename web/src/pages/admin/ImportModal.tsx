import '../../styles/overlays.css'
import { useEffect, useState } from 'react'
import type { AdminProduct } from '../../api/types'
import { adminImportProduct } from '../../api/client'
import { useAuth } from '../../auth/AuthProvider'
import { useT } from '../../i18n/LanguageProvider'

type Tab = 'file' | 'url'

export function ImportModal({ open, onClose, onImported }: {
  open: boolean
  onClose: () => void
  onImported: (draft: AdminProduct) => void
}) {
  const { token } = useAuth()
  const t = useT()
  const [tab, setTab] = useState<Tab>('file')
  const [url, setUrl] = useState('')
  const [spec, setSpec] = useState('')
  const [fileName, setFileName] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (!open) return
    function onKey(e: KeyboardEvent) { if (e.key === 'Escape') onClose() }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [open, onClose])

  if (!open) return null

  async function onFile(e: React.ChangeEvent<HTMLInputElement>) {
    const f = e.target.files?.[0]
    if (!f) return
    setFileName(f.name)
    setSpec(await f.text())
    setErr('')
  }

  async function submit() {
    if (!token || busy) return
    const src = tab === 'url' ? { url: url.trim() } : { spec: spec.trim() }
    if ((tab === 'url' && !src.url) || (tab === 'file' && !('spec' in src && src.spec))) {
      setErr(tab === 'url' ? t('admin.enterUrlError') : t('admin.chooseFileError'))
      return
    }
    setBusy(true); setErr('')
    try {
      const draft = await adminImportProduct(token, src)
      onImported(draft)
      onClose()
    } catch (e) {
      setErr(e instanceof Error ? e.message : t('admin.importFailed'))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="appdetail-scrim" onClick={e => { if (e.target === e.currentTarget) onClose() }}>
      <div className="dmodal" role="dialog" aria-modal="true" aria-label={t('admin.importApi')}>
        <div className="composer-head">
          <span className="dot" />
          <h2>{t('admin.importApi')}</h2>
          <span className="hint">{t('admin.importHint')}</span>
        </div>

        <div className="tabs" role="tablist" aria-label={t('admin.importSourceAriaLabel')}>
          <button id="imp-tab-file" role="tab" aria-selected={tab === 'file'} aria-controls="imp-tabpanel"
            className={`tab ${tab === 'file' ? 'on' : ''}`}
            onClick={() => { setTab('file'); setErr('') }}>{t('admin.fileTab')}</button>
          <button id="imp-tab-url" role="tab" aria-selected={tab === 'url'} aria-controls="imp-tabpanel"
            className={`tab ${tab === 'url' ? 'on' : ''}`}
            onClick={() => { setTab('url'); setErr('') }}>{t('admin.urlTab')}</button>
        </div>

        <div className="composer-body">
          <div role="tabpanel" id="imp-tabpanel" aria-labelledby={tab === 'file' ? 'imp-tab-file' : 'imp-tab-url'}>
          {tab === 'file' ? (
            <div className="field">
              <label htmlFor="imp-file">{t('admin.specFileLabel')}</label>
              <input id="imp-file" type="file" accept=".json,.yaml,.yml" onChange={onFile} />
              {fileName && <div className="help">{fileName}</div>}
            </div>
          ) : (
            <div className="field">
              <label htmlFor="imp-url">{t('admin.specUrlLabel')}</label>
              <input id="imp-url" className="ipt mono" placeholder={t('admin.specUrlPlaceholderEx')}
                autoComplete="off" value={url} onChange={e => setUrl(e.target.value)} />
            </div>
          )}
          </div>

          {err && <p className="autherr" role="alert">{err}</p>}

          <div className="composer-foot">
            <div className="foot-acts">
              <button type="button" className="btn btn-ghost btn-sm" onClick={onClose}>{t('common.cancel')}</button>
              <button type="button" className="btn btn-primary btn-sm" onClick={submit} disabled={busy}>
                {busy ? t('admin.importingLabel') : t('admin.importCta')}
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
