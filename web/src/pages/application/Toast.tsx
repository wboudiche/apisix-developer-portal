export function Toast({ msg }: { msg: string | null }) {
  return (
    <div className={`appdetail-toast ${msg ? 'show' : ''}`} role="status" aria-live="polite">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2.2} aria-hidden="true">
        <path d="M20 6L9 17l-5-5" strokeLinecap="round" strokeLinejoin="round" />
      </svg>
      <span>{msg}</span>
    </div>
  )
}
