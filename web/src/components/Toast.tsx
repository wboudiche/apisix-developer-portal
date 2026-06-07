import { useCallback, useEffect, useRef, useState } from 'react'
import '../styles/overlays.css'

export type ToastKind = 'ok' | 'warn'

export function Toast({ msg, kind = 'ok' }: { msg: string | null; kind?: ToastKind }) {
  return (
    <div className={`appdetail-toast ${kind} ${msg ? 'show' : ''}`} role="status" aria-live="polite">
      {kind === 'warn' ? (
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} aria-hidden="true">
          <path d="M12 9v4M12 17h.01M10.3 3.9L2 18a2 2 0 001.7 3h16.6a2 2 0 001.7-3L13.7 3.9a2 2 0 00-3.4 0z" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
      ) : (
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2.2} aria-hidden="true">
          <path d="M20 6L9 17l-5-5" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
      )}
      <span>{msg}</span>
    </div>
  )
}

// Shared toast state: message + kind with auto-hide (blueprint: 2.6s) and
// timer cleanup on unmount.
export function useToast() {
  const [toast, setToast] = useState<{ msg: string; kind: ToastKind } | null>(null)
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined)
  useEffect(() => () => clearTimeout(timer.current), [])
  const notify = useCallback((msg: string, kind: ToastKind = 'ok') => {
    setToast({ msg, kind })
    clearTimeout(timer.current)
    timer.current = setTimeout(() => setToast(null), 2600)
  }, [])
  return { toast, notify }
}
