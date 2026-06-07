import '../styles/overlays.css'
import { useEffect, useRef } from 'react'

export interface ModalSpec {
  title: string
  body: string
  confirmLabel?: string
  danger?: boolean
  onConfirm: () => void
}

function TrashIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} aria-hidden="true">
      <path d="M3 6h18M8 6V4a2 2 0 012-2h4a2 2 0 012 2v2M19 6l-1 14a2 2 0 01-2 2H8a2 2 0 01-2-2L5 6" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

function RotateIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} aria-hidden="true">
      <path d="M21 2v6h-6M3 12a9 9 0 0115-6.7L21 8" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

export function ConfirmModal({ spec, onClose }: { spec: ModalSpec | null; onClose: () => void }) {
  const triggerRef = useRef<HTMLElement | null>(null)
  const prevSpecRef = useRef<ModalSpec | null>(null)

  // Capture the trigger during render, before autoFocus runs
  if (spec && !prevSpecRef.current) {
    triggerRef.current = document.activeElement as HTMLElement
  }
  prevSpecRef.current = spec

  useEffect(() => {
    if (spec) return
    triggerRef.current?.focus()
    triggerRef.current = null
  }, [spec])

  useEffect(() => {
    if (!spec) return
    function onKey(e: KeyboardEvent) { if (e.key === 'Escape') onClose() }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [spec, onClose])

  if (!spec) return null
  return (
    <div className="appdetail-scrim" onClick={e => { if (e.target === e.currentTarget) onClose() }}>
      <div className="dmodal" role="dialog" aria-modal="true" aria-label={spec.title}>
        <div
          className="mi"
          style={{
            background: spec.danger ? 'var(--danger-soft)' : 'var(--warn-soft)',
            color: spec.danger ? 'var(--danger)' : 'oklch(52% 0.14 70)',
          }}
        >
          {spec.danger ? <TrashIcon /> : <RotateIcon />}
        </div>
        <h3>{spec.title}</h3>
        <p>{spec.body}</p>
        <div className="ma">
          <button className="btn ghost" onClick={onClose}>Annuler</button>
          <button
            autoFocus
            className={`btn ${spec.danger ? 'danger' : 'primary'}`}
            onClick={() => { const fn = spec.onConfirm; onClose(); fn() }}
          >
            {spec.confirmLabel ?? 'Confirmer'}
          </button>
        </div>
      </div>
    </div>
  )
}
