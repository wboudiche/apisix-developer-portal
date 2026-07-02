import type { CSSProperties } from 'react'
import { useT } from '../i18n/LanguageProvider'

interface PaginationProps {
  page: number
  pageSize: number
  total: number
  onPage: (page: number) => void
}

// Minimal prev/next pager. Renders nothing when the full set fits on one page.
export function Pagination({ page, pageSize, total, onPage }: PaginationProps) {
  const t = useT()
  if (total <= pageSize) return null
  const lastPage = Math.max(1, Math.ceil(total / pageSize))
  const btn: CSSProperties = {
    fontSize: 13, padding: '5px 12px', borderRadius: 8,
    border: '1px solid var(--border-2)', background: 'var(--surface)',
    color: 'var(--fg)', cursor: 'pointer',
  }
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 12, justifyContent: 'center', margin: '20px 0' }}>
      <button style={btn} onClick={() => onPage(page - 1)} disabled={page <= 1}>{t('pagination.prev')}</button>
      <span style={{ fontSize: 13, color: 'var(--muted)' }}>{t('pagination.pageInfo', { page, total })}</span>
      <button style={btn} onClick={() => onPage(page + 1)} disabled={page >= lastPage}>{t('pagination.next')}</button>
    </div>
  )
}
