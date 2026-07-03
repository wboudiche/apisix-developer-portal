import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { ConfirmModal, type ModalSpec } from './ConfirmModal'
import { LanguageProvider } from '../i18n/LanguageProvider'

beforeEach(() => {
  // jsdom's navigator.language defaults to 'en-US', which would auto-detect to
  // English; force French so existing assertions (against French strings) hold.
  localStorage.setItem('lang', 'fr')
})

function renderModal(props: Parameters<typeof ConfirmModal>[0]) {
  return render(<LanguageProvider><ConfirmModal {...props} /></LanguageProvider>)
}

describe('ConfirmModal', () => {
  it('renders nothing when spec is null', () => {
    const { container } = renderModal({ spec: null, onClose: () => {} })
    expect(container.firstChild).toBeNull()
  })
  it('confirms then closes', async () => {
    const onConfirm = vi.fn(); const onClose = vi.fn()
    renderModal({ spec: { title: 'Résilier ?', body: 'corps', confirmLabel: 'Résilier', danger: true, onConfirm }, onClose })
    expect(screen.getByText('Résilier ?')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Résilier' }))
    expect(onConfirm).toHaveBeenCalledOnce()
    expect(onClose).toHaveBeenCalledOnce()
  })
  it('cancels without confirming', async () => {
    const onConfirm = vi.fn(); const onClose = vi.fn()
    renderModal({ spec: { title: 't', body: 'b', onConfirm }, onClose })
    await userEvent.click(screen.getByRole('button', { name: 'Annuler' }))
    expect(onClose).toHaveBeenCalledOnce()
    expect(onConfirm).not.toHaveBeenCalled()
  })
  it('moves focus into confirm button on open and restores focus to trigger on close', async () => {
    const baseSpec: ModalSpec = { title: 'Supprimer ?', body: 'Action irréversible.', onConfirm: vi.fn() }
    function Wrapper() {
      const [spec, setSpec] = useState<ModalSpec | null>(null)
      return (
        <LanguageProvider>
          <button id="trigger" onClick={() => setSpec(baseSpec)}>Ouvrir</button>
          <ConfirmModal spec={spec} onClose={() => setSpec(null)} />
        </LanguageProvider>
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
