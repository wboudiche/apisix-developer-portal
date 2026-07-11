import { StrictMode } from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { VerifyEmailPage } from './VerifyEmailPage'
import { LanguageProvider } from '../i18n/LanguageProvider'
import * as api from '../api/client'
import { ApiError } from '../api/client'

beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('lang', 'fr')
  vi.restoreAllMocks()
})

function renderAt(url: string) {
  return render(
    <MemoryRouter initialEntries={[url]}>
      <LanguageProvider><VerifyEmailPage /></LanguageProvider>
    </MemoryRouter>
  )
}

describe('VerifyEmailPage', () => {
  it('verifies the token and shows success with a login link', async () => {
    const verify = vi.spyOn(api, 'verifyEmail').mockResolvedValue(undefined)
    renderAt('/verify-email?token=tok123')
    await waitFor(() => expect(verify).toHaveBeenCalledWith('tok123'))
    expect(await screen.findByText(/vérifiée/i)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /connexion|se connecter/i })).toBeInTheDocument()
  })

  it('shows the expired state with a resend form on 410', async () => {
    vi.spyOn(api, 'verifyEmail').mockRejectedValue(new ApiError('lien de vérification invalide ou expiré', 410))
    const resend = vi.spyOn(api, 'resendVerification').mockResolvedValue(undefined)
    renderAt('/verify-email?token=stale')
    expect(await screen.findByRole('heading', { name: /invalide ou expiré/i })).toBeInTheDocument()
    await userEvent.type(screen.getByLabelText(/email/i), 'd@x.io')
    await userEvent.click(screen.getByRole('button', { name: /renvoyer/i }))
    await waitFor(() => expect(resend).toHaveBeenCalledWith('d@x.io'))
    expect(await screen.findByText(/envoyé/i)).toBeInTheDocument()
  })

  it('shows the invalid state immediately when no token is present', async () => {
    const verify = vi.spyOn(api, 'verifyEmail')
    renderAt('/verify-email')
    expect(await screen.findByRole('heading', { name: /invalide ou expiré/i })).toBeInTheDocument()
    expect(verify).not.toHaveBeenCalled()
  })

  it('calls verifyEmail exactly once even under StrictMode double-invocation', async () => {
    // The verification token is single-use server-side (a second POST gets a
    // 410), so StrictMode's dev-mode double-effect must not issue a second
    // request — otherwise a real success can flip to "invalid".
    const verify = vi.spyOn(api, 'verifyEmail').mockResolvedValue(undefined)
    render(
      <StrictMode>
        <MemoryRouter initialEntries={['/verify-email?token=tok123']}>
          <LanguageProvider><VerifyEmailPage /></LanguageProvider>
        </MemoryRouter>
      </StrictMode>
    )
    expect(await screen.findByText(/vérifiée/i)).toBeInTheDocument()
    expect(verify).toHaveBeenCalledTimes(1)
  })

  it('shows a generic error state (no resend form) when verification fails for a non-410 reason', async () => {
    vi.spyOn(api, 'verifyEmail').mockRejectedValue(new ApiError('server error', 500))
    renderAt('/verify-email?token=tok500')
    expect(await screen.findByText(/une erreur est survenue/i)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /renvoyer/i })).not.toBeInTheDocument()
    expect(screen.getByRole('link', { name: /connexion|se connecter/i })).toBeInTheDocument()
  })
})
