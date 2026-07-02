import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { Pagination } from './Pagination'
import { LanguageProvider } from '../i18n/LanguageProvider'

beforeEach(() => {
  // jsdom's navigator.language defaults to 'en-US', which would auto-detect to
  // English; force French so existing assertions (against French strings) hold.
  localStorage.setItem('lang', 'fr')
})

function renderPagination(props: Parameters<typeof Pagination>[0]) {
  return render(<LanguageProvider><Pagination {...props} /></LanguageProvider>)
}

describe('Pagination', () => {
  it('renders nothing when everything fits on one page', () => {
    const { container } = renderPagination({ page: 1, pageSize: 20, total: 20, onPage: () => {} })
    expect(container.firstChild).toBeNull()
  })

  it('shows page info and total', () => {
    renderPagination({ page: 2, pageSize: 20, total: 45, onPage: () => {} })
    expect(screen.getByText(/Page 2/)).toBeInTheDocument()
    expect(screen.getByText(/45/)).toBeInTheDocument()
  })

  it('disables Préc. on first page and advances on Suiv.', () => {
    const onPage = vi.fn()
    renderPagination({ page: 1, pageSize: 20, total: 45, onPage })
    expect(screen.getByRole('button', { name: /Préc/ })).toBeDisabled()
    fireEvent.click(screen.getByRole('button', { name: /Suiv/ }))
    expect(onPage).toHaveBeenCalledWith(2)
  })

  it('disables Suiv. on the last page', () => {
    renderPagination({ page: 3, pageSize: 20, total: 45, onPage: () => {} })
    expect(screen.getByRole('button', { name: /Suiv/ })).toBeDisabled()
  })
})
