import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { ConfirmModal, ModalSpec } from './ConfirmModal'

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
  it('moves focus into confirm button on open and restores focus to trigger on close', async () => {
    const baseSpec: ModalSpec = { title: 'Supprimer ?', body: 'Action irréversible.', onConfirm: vi.fn() }
    function Wrapper() {
      const [spec, setSpec] = useState<ModalSpec | null>(null)
      return (
        <>
          <button id="trigger" onClick={() => setSpec(baseSpec)}>Ouvrir</button>
          <ConfirmModal spec={spec} onClose={() => setSpec(null)} />
        </>
      )
    }
    render(<Wrapper />)
    const trigger = screen.getByRole('button', { name: 'Ouvrir' })
    trigger.focus()
    expect(document.activeElement).toBe(trigger)

    await userEvent.click(trigger)
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Confirmer' }))

    await userEvent.click(screen.getByRole('button', { name: 'Annuler' }))
    expect(document.activeElement).toBe(trigger)
  })
})
