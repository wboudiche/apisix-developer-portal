import type { InputHTMLAttributes } from 'react'

/**
 * A styled file picker: a ghost button wrapping a visually-hidden native
 * <input type="file">, so it stays keyboard- and screen-reader-accessible
 * while matching the form's buttons instead of the browser default.
 */
export function FileButton({ label, disabled, ...input }: { label: string } & Omit<InputHTMLAttributes<HTMLInputElement>, 'type'>) {
  return (
    <label className={`filebtn${disabled ? ' is-disabled' : ''}`}>
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} aria-hidden="true">
        <path d="M12 15V4m0 0L7.5 8.5M12 4l4.5 4.5M5 15v3a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2v-3" strokeLinecap="round" strokeLinejoin="round" />
      </svg>
      {label}
      <input type="file" className="sr-only" disabled={disabled} {...input} />
    </label>
  )
}
