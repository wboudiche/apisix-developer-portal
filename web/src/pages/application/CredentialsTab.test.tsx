import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, act } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { CredentialsTab } from './CredentialsTab'
import type { ModalSpec } from './ConfirmModal'

const KEY = 'ax_live_a3f9c1e7b240d8e5f6a1b9c4d7e2f8a0'

beforeEach(() => {
  Object.assign(navigator, { clipboard: { writeText: vi.fn().mockResolvedValue(undefined) } })
})

function setup() {
  const notify = vi.fn()
  let lastModal: ModalSpec | null = null
  const openModal = vi.fn((s: ModalSpec) => { lastModal = s })
  render(<CredentialsTab apiKey={KEY} notify={notify} openModal={openModal} />)
  return { notify, openModal, getModal: () => lastModal }
}

describe('CredentialsTab', () => {
  it('masks the production key by default and reveals on toggle', async () => {
    setup()
    const code = screen.getByTestId('key-prod')
    expect(code.textContent).toBe('ax_live_' + '•'.repeat(KEY.length - 10) + 'a0')
    await userEvent.click(screen.getAllByRole('button', { name: 'Afficher / masquer' })[0])
    expect(code.textContent).toBe(KEY)
  })
  it('copies the real key and notifies', async () => {
    const { notify } = setup()
    await userEvent.click(screen.getAllByRole('button', { name: 'Copier' })[0])
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(KEY)
    expect(notify).toHaveBeenCalledWith('Clé copiée dans le presse-papiers')
  })
  it('production rotate confirms into a coming-soon toast, key untouched', async () => {
    const { notify, openModal, getModal } = setup()
    await userEvent.click(screen.getAllByRole('button', { name: /Régénérer/ })[0])
    expect(openModal).toHaveBeenCalled()
    act(() => { getModal()!.onConfirm() })
    expect(notify).toHaveBeenCalledWith('Rotation des clés à venir')
    expect(screen.getByTestId('key-prod').textContent).toContain('ax_live_')
  })
  it('sandbox rotate visually replaces the demo key', async () => {
    const { getModal } = setup()
    const before = screen.getByTestId('key-sbx').textContent
    await userEvent.click(screen.getAllByRole('button', { name: /Régénérer/ })[1])
    act(() => { getModal()!.onConfirm() })
    const after = screen.getByTestId('key-sbx').textContent
    expect(after).toMatch(/^ax_test_[0-9a-f]{32}$/)   // revealed fresh key
    expect(after).not.toBe(before)
  })
})
