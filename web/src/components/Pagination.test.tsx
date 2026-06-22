import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { Pagination } from './Pagination'

describe('Pagination', () => {
  it('renders nothing when everything fits on one page', () => {
    const { container } = render(<Pagination page={1} pageSize={20} total={20} onPage={() => {}} />)
    expect(container.firstChild).toBeNull()
  })

  it('shows page info and total', () => {
    render(<Pagination page={2} pageSize={20} total={45} onPage={() => {}} />)
    expect(screen.getByText(/Page 2/)).toBeInTheDocument()
    expect(screen.getByText(/45/)).toBeInTheDocument()
  })

  it('disables Préc. on first page and advances on Suiv.', () => {
    const onPage = vi.fn()
    render(<Pagination page={1} pageSize={20} total={45} onPage={onPage} />)
    expect(screen.getByRole('button', { name: /Préc/ })).toBeDisabled()
    fireEvent.click(screen.getByRole('button', { name: /Suiv/ }))
    expect(onPage).toHaveBeenCalledWith(2)
  })

  it('disables Suiv. on the last page', () => {
    render(<Pagination page={3} pageSize={20} total={45} onPage={() => {}} />)
    expect(screen.getByRole('button', { name: /Suiv/ })).toBeDisabled()
  })
})
