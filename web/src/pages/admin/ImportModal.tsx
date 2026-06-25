import '../../styles/overlays.css'
import { useEffect, useState } from 'react'
import type { AdminProduct } from '../../api/types'
import { adminImportProduct } from '../../api/client'
import { useAuth } from '../../auth/AuthProvider'

type Tab = 'file' | 'url'

export function ImportModal({ open, onClose, onImported }: {
  open: boolean
  onClose: () => void
  onImported: (draft: AdminProduct) => void
}) {
  const { token } = useAuth()
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
      setErr(tab === 'url' ? 'Saisissez une URL.' : 'Choisissez un fichier de spécification.')
      return
    }
    setBusy(true); setErr('')
    try {
      const draft = await adminImportProduct(token, src)
      onImported(draft)
      onClose()
    } catch (e) {
      setErr(e instanceof Error ? e.message : "Échec de l'import.")
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="appdetail-scrim" onClick={e => { if (e.target === e.currentTarget) onClose() }}>
      <div className="dmodal" role="dialog" aria-modal="true" aria-label="Importer une API">
        <div className="composer-head">
          <span className="dot" />
          <h2>Importer une API</h2>
          <span className="hint">OpenAPI 3.x ou Swagger 2.0</span>
        </div>

        <div className="tabs" role="tablist" aria-label="Source de la spécification">
          <button role="tab" aria-selected={tab === 'file'} className={`tab ${tab === 'file' ? 'on' : ''}`}
            onClick={() => { setTab('file'); setErr('') }}>Fichier</button>
          <button role="tab" aria-selected={tab === 'url'} className={`tab ${tab === 'url' ? 'on' : ''}`}
            onClick={() => { setTab('url'); setErr('') }}>URL</button>
        </div>

        <div className="composer-body">
          {tab === 'file' ? (
            <div className="field">
              <label htmlFor="imp-file">Fichier de spécification</label>
              <input id="imp-file" type="file" accept=".json,.yaml,.yml" onChange={onFile} />
              {fileName && <div className="help">{fileName}</div>}
            </div>
          ) : (
            <div className="field">
              <label htmlFor="imp-url">URL de la spécification</label>
              <input id="imp-url" className="ipt mono" placeholder="https://api.example.com/openapi.json"
                autoComplete="off" value={url} onChange={e => setUrl(e.target.value)} />
            </div>
          )}

          {err && <p className="autherr" role="alert">{err}</p>}

          <div className="composer-foot">
            <div className="foot-acts">
              <button type="button" className="btn btn-ghost btn-sm" onClick={onClose}>Annuler</button>
              <button type="button" className="btn btn-primary btn-sm" onClick={submit} disabled={busy}>
                {busy ? 'Import…' : 'Importer'}
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
