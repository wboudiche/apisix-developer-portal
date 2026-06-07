import type { FormEvent, ReactNode } from 'react'

// Blueprint .composer: header (dot + title + hint), body (page-provided field
// grid), dashed-top foot (left slot + Annuler/submit). Unmounted when closed,
// so autoFocus on the page's first field fires on every open.
export function Composer({ open, title, hint, submitLabel, onSubmit, onCancel, footLeft, children }: {
  open: boolean
  title: string
  hint: string
  submitLabel: string
  onSubmit: () => void
  onCancel: () => void
  footLeft?: ReactNode
  children: ReactNode
}) {
  if (!open) return null
  function submit(e: FormEvent) { e.preventDefault(); onSubmit() }
  return (
    <form className="composer" onSubmit={submit}>
      <div className="composer-head">
        <span className="dot" />
        <h2>{title}</h2>
        <span className="hint">{hint}</span>
      </div>
      <div className="composer-body">
        {children}
        <div className="composer-foot">
          {footLeft}
          <div className="foot-acts">
            <button type="button" className="btn btn-ghost btn-sm" onClick={onCancel}>Annuler</button>
            <button type="submit" className="btn btn-primary btn-sm">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} aria-hidden="true"><path d="M20 6L9 17l-5-5" strokeLinecap="round" strokeLinejoin="round" /></svg>
              {submitLabel}
            </button>
          </div>
        </div>
      </div>
    </form>
  )
}
