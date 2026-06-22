import type { CSSProperties } from 'react'

interface PaginationProps {
  page: number
  pageSize: number
  total: number
  onPage: (page: number) => void
}

// Minimal prev/next pager. Renders nothing when the full set fits on one page.
export function Pagination({ page, pageSize, total, onPage }: PaginationProps) {
  if (total <= pageSize) return null
  const lastPage = Math.max(1, Math.ceil(total / pageSize))
  const btn: CSSProperties = {
    fontSize: 13, padding: '5px 12px', borderRadius: 8,
    border: '1px solid var(--border-2)', background: 'var(--surface)',
    color: 'var(--fg)', cursor: 'pointer',
  }
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 12, justifyContent: 'center', margin: '20px 0' }}>
      <button style={btn} onClick={() => onPage(page - 1)} disabled={page <= 1}>Préc.</button>
      <span style={{ fontSize: 13, color: 'var(--muted)' }}>Page {page} · {total} au total</span>
      <button style={btn} onClick={() => onPage(page + 1)} disabled={page >= lastPage}>Suiv.</button>
    </div>
  )
}
