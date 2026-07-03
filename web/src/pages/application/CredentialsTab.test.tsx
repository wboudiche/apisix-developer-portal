import type { ReactElement } from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, act, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { CredentialsTab } from './CredentialsTab'
import { LanguageProvider } from '../../i18n/LanguageProvider'
import type { ModalSpec } from '../../components/ConfirmModal'
import * as api from '../../api/client'

const KEY = 'ax_live_a3f9c1e7b240d8e5f6a1b9c4d7e2f8a0'

const base = {
  apiKey: 'prod-key-123', appId: 7, token: 'jwt',
  notify: vi.fn(), openModal: vi.fn(), onRotated: vi.fn(),
}

beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('lang', 'fr')
  Object.assign(navigator, { clipboard: { writeText: vi.fn().mockResolvedValue(undefined) } })
  vi.restoreAllMocks()
})

function renderCreds(ui: ReactElement) {
  return render(<LanguageProvider>{ui}</LanguageProvider>)
}

function setup() {
  const notify = vi.fn()
  const onRotated = vi.fn()
  let lastModal: ModalSpec | null = null
  const openModal = vi.fn((s: ModalSpec) => { lastModal = s })
  renderCreds(<CredentialsTab apiKey={KEY} appId={7} token="jwt" notify={notify} openModal={openModal} onRotated={onRotated} sandboxEligible={false} />)
  return { notify, openModal, onRotated, getModal: () => lastModal }
}

describe('CredentialsTab', () => {
  it('shows only the Production key (no Sandbox card)', () => {
    setup()
    expect(screen.getByTestId('key-prod')).toBeInTheDocument()
    expect(screen.queryByTestId('key-sandbox')).not.toBeInTheDocument()
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

it('shows an enable button when sandbox-eligible but not enabled, and reveals the key', async () => {
  const spy = vi.spyOn(api, 'enableSandbox').mockResolvedValue({ sandboxApiKey: 'sb-new' })
  renderCreds(<CredentialsTab {...base} sandboxEligible sandboxEnabled={false} sandboxGatewayUrl="http://localhost:9081" />)
  await userEvent.click(screen.getByRole('button', { name: /Activer le sandbox/i }))
  await waitFor(() => expect(spy).toHaveBeenCalledWith('jwt', 7))
  expect(await screen.findByText('sb-new')).toBeInTheDocument()
})

it('shows the sandbox base URL + Régénérer when already enabled', () => {
  renderCreds(<CredentialsTab {...base} sandboxEligible sandboxEnabled sandboxGatewayUrl="http://localhost:9081" />)
  expect(screen.getByText(/localhost:9081/)).toBeInTheDocument()
  const sandboxCard = screen.getByText('Sandbox').closest('.keycard')!
  expect(within(sandboxCard as HTMLElement).getByRole('button', { name: /Régénérer/i })).toBeInTheDocument()
})

it('hides the sandbox card entirely when not eligible', () => {
  renderCreds(<CredentialsTab {...base} sandboxEligible={false} sandboxEnabled={false} sandboxGatewayUrl="" />)
  expect(screen.queryByText(/sandbox/i)).not.toBeInTheDocument()
})

it('shows the OAuth2 card when oauthEligible and saves the client id', async () => {
  const spy = vi.spyOn(api, 'setOidcClient').mockResolvedValue(undefined)
  renderCreds(<CredentialsTab {...base} sandboxEligible={false} oauthEligible oidcIssuer="https://idp.example" oidcClientId="" />)
  expect(screen.getByText(/https:\/\/idp.example/)).toBeInTheDocument()
  await userEvent.type(screen.getByLabelText(/Client ID/i), 'client-abc')
  await userEvent.click(screen.getByRole('button', { name: /Enregistrer/i }))
  await waitFor(() => expect(spy).toHaveBeenCalledWith('jwt', 7, 'client-abc'))
})

it('prefills the client id input from oidcClientId', () => {
  renderCreds(<CredentialsTab {...base} sandboxEligible={false} oauthEligible oidcIssuer="https://idp.example" oidcClientId="existing-client" />)
  expect((screen.getByLabelText(/Client ID/i) as HTMLInputElement).value).toBe('existing-client')
})

it('hides the OAuth2 card when not oauthEligible', () => {
  renderCreds(<CredentialsTab {...base} sandboxEligible={false} oauthEligible={false} />)
  expect(screen.queryByLabelText(/Client ID/i)).not.toBeInTheDocument()
})
