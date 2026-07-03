import { useEffect, useRef, useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import type { Application } from '../../api/types'
import { appRef, initials, glyphGradient } from './helpers'
import { useT } from '../../i18n/LanguageProvider'

export function AppSwitcher({ apps, currentId, onCreate }: {
  apps: Application[]
  currentId: number
  onCreate: () => void
}) {
  const t = useT()
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLSpanElement>(null)

  useEffect(() => {
    if (!open) return
    function onDoc(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', onDoc)
    return () => document.removeEventListener('mousedown', onDoc)
  }, [open])

  return (
    <span className={`switch ${open ? 'open' : ''}`} ref={ref}>
      <button className="trigger" onClick={() => setOpen(o => !o)} aria-haspopup="menu" aria-expanded={open}>
        {t('app.switchApp')}
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} aria-hidden="true"><path d="M6 9l6 6 6-6" strokeLinecap="round" strokeLinejoin="round" /></svg>
      </button>
      <div className="menu" role="menu">
        {apps.map(a => (
          <Link key={a.id} to={`/applications/${a.id}`} className={a.id === currentId ? 'cur' : ''} onClick={() => setOpen(false)} role="menuitem">
            <span className="mg" style={{ background: glyphGradient(a.id) }}>{initials(a.name)}</span>
            <span className="mt">{a.name}<small>{appRef(a.id)}</small></span>
          </Link>
        ))}
        <div className="div" />
        <button type="button" className="new" onClick={() => { setOpen(false); onCreate() }} role="menuitem">
          <span className="mg" style={{ background: 'var(--accent-soft)', color: 'var(--accent)' }}>+</span>
          <span className="mt">{t('app.newApplicationTitle')}</span>
        </button>
      </div>
    </span>
  )
}

export function CreateAppModal({ open, onClose, onCreate }: {
  open: boolean
  onClose: () => void
  onCreate: (name: string) => Promise<void>
}) {
  const t = useT()
  const [name, setName] = useState('')
  const [busy, setBusy] = useState(false)

  // Same a11y contract as ConfirmModal (85c35f4): capture the trigger during
  // render (before the input's autoFocus fires at commit), restore on close.
  const triggerRef = useRef<HTMLElement | null>(null)
  const prevOpenRef = useRef(false)
  if (open && !prevOpenRef.current) triggerRef.current = document.activeElement as HTMLElement | null
  prevOpenRef.current = open

  useEffect(() => {
    if (open) return
    triggerRef.current?.focus()
    triggerRef.current = null
  }, [open])

  useEffect(() => {
    if (!open) return
    function onKey(e: KeyboardEvent) { if (e.key === 'Escape') onClose() }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [open, onClose])

  if (!open) return null

  async function submit(e: FormEvent) {
    e.preventDefault()
    if (!name.trim()) return
    setBusy(true)
    try { await onCreate(name.trim()); setName('') } finally { setBusy(false) }
  }

  return (
    <div className="appdetail-scrim" onClick={e => { if (e.target === e.currentTarget) onClose() }}>
      <form className="dmodal" onSubmit={submit} role="dialog" aria-modal="true" aria-label={t('app.newApplicationTitle')}>
        <h3>{t('app.newApplicationTitle')}</h3>
        <p>{t('app.newAppModalDesc')}</p>
        <div className="field">
          <label htmlFor="new-app-name">{t('app.appNameLabel')}</label>
          <input id="new-app-name" value={name} onChange={e => setName(e.target.value)} placeholder={t('app.appNamePlaceholderEx')} autoFocus />
        </div>
        <div className="ma">
          <button type="button" className="btn ghost" onClick={onClose}>{t('common.cancel')}</button>
          <button type="submit" className="btn primary" disabled={busy || !name.trim()}>{t('app.create')}</button>
        </div>
      </form>
    </div>
  )
}
