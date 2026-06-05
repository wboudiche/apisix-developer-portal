import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ConfirmModal } from './ConfirmModal'

describe('ConfirmModal', () => {
  it('renders nothing when spec is null', () => {
    const { container } = render(<ConfirmModal spec={null} onClose={() => {}} />)
    expect(container.firstChild).toBeNull()
  })
  it('confirms then closes', async () => {
    const onConfirm = vi.fn(); const onClose = vi.fn()
    render(<ConfirmModal spec={{ title: 'Résilier ?', body: 'corps', confirmLabel: 'Résilier', danger: true, onConfirm }} onClose={onClose} />)
    expect(screen.getByText('Résilier ?')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Résilier' }))
    expect(onConfirm).toHaveBeenCalledOnce()
    expect(onClose).toHaveBeenCalledOnce()
  })
  it('cancels without confirming', async () => {
    const onConfirm = vi.fn(); const onClose = vi.fn()
    render(<ConfirmModal spec={{ title: 't', body: 'b', onConfirm }} onClose={onClose} />)
    await userEvent.click(screen.getByRole('button', { name: 'Annuler' }))
    expect(onClose).toHaveBeenCalledOnce()
    expect(onConfirm).not.toHaveBeenCalled()
  })
})
