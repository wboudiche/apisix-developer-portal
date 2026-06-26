import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, act, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { CredentialsTab } from './CredentialsTab'
import type { ModalSpec } from '../../components/ConfirmModal'
import * as api from '../../api/client'

const KEY = 'ax_live_a3f9c1e7b240d8e5f6a1b9c4d7e2f8a0'

beforeEach(() => {
  Object.assign(navigator, { clipboard: { writeText: vi.fn().mockResolvedValue(undefined) } })
  vi.restoreAllMocks()
})

function setup() {
  const notify = vi.fn()
  const onRotated = vi.fn()
  let lastModal: ModalSpec | null = null
  const openModal = vi.fn((s: ModalSpec) => { lastModal = s })
  render(<CredentialsTab apiKey={KEY} appId={7} token="jwt" notify={notify} openModal={openModal} onRotated={onRotated} />)
  return { notify, openModal, onRotated, getModal: () => lastModal }
}

describe('CredentialsTab', () => {
  it('shows only the Production key (no Sandbox card)', () => {
    setup()
    expect(screen.getByTestId('key-prod')).toBeInTheDocument()
    expect(screen.queryByTestId('key-sbx')).not.toBeInTheDocument()
  })

  it('reveals on toggle and copies the real key', async () => {
    const { notify } = setup()
    const code = screen.getByTestId('key-prod')
    expect(code.textContent).toBe('ax_live_' + '•'.repeat(KEY.length - 10) + 'a0')
    await userEvent.click(screen.getAllByRole('button', { name: 'Afficher / masquer' })[0])
    expect(code.textContent).toBe(KEY)
    await userEvent.click(screen.getAllByRole('button', { name: 'Copier' })[0])
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(KEY)
    expect(notify).toHaveBeenCalledWith('Clé copiée dans le presse-papiers')
  })

  it('rotate confirms, calls the API, reveals the new key, and notifies', async () => {
    const spy = vi.spyOn(api, 'rotateKey').mockResolvedValue({ apiKey: 'ax_live_NEWKEY00000000000000000000000000' })
    const { openModal, getModal, onRotated, notify } = setup()
    await userEvent.click(screen.getAllByRole('button', { name: /Régénérer/ })[0])
    expect(openModal).toHaveBeenCalled()
    await act(async () => { await getModal()!.onConfirm() })
    expect(spy).toHaveBeenCalledWith('jwt', 7)
    await waitFor(() => expect(screen.getByTestId('key-prod').textContent).toBe('ax_live_NEWKEY00000000000000000000000000'))
    expect(notify).toHaveBeenCalledWith('Nouvelle clé générée')
    expect(onRotated).toHaveBeenCalled()
  })
})
