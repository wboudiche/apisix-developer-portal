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
})
