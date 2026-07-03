import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { LoginPage } from './LoginPage'
import { RegisterPage } from './RegisterPage'
import { AuthProvider } from '../auth/AuthProvider'
import { LanguageProvider } from '../i18n/LanguageProvider'
import * as api from '../api/client'

beforeEach(() => {
  localStorage.clear()
  vi.restoreAllMocks()
  // jsdom's navigator.language defaults to 'en-US', which would auto-detect to
  // English; force French so existing assertions (against French strings) hold.
  localStorage.setItem('lang', 'fr')
  // AuthShell fetches catalog stats on mount; neutralize it for page tests.
  vi.spyOn(api, 'getProducts').mockResolvedValue({ items: [], total: 0, page: 1, pageSize: 20 })
})

function renderLogin() {
  return render(
    <LanguageProvider>
      <MemoryRouter initialEntries={['/login']}>
        <AuthProvider>
          <Routes>
            <Route path="/login" element={<LoginPage />} />
            <Route path="/" element={<div>CATALOG HOME</div>} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    </LanguageProvider>
  )
}

describe('LoginPage', () => {
  it('submits credentials and navigates home on success', async () => {
    vi.spyOn(api, 'login').mockResolvedValue({ user: { id: 1, email: 'a@b.c', name: '', role: 'developer' }, token: 'jwt' })
    renderLogin()
    await userEvent.type(screen.getByLabelText('Email'), 'a@b.c')
    await userEvent.type(screen.getByLabelText('Mot de passe'), 'pw12345678')
    await userEvent.click(screen.getByRole('button', { name: 'Se connecter' }))
    await waitFor(() => expect(screen.getByText('CATALOG HOME')).toBeInTheDocument())
  })

  it('shows the server error on failure', async () => {
    vi.spyOn(api, 'login').mockRejectedValue(new Error('invalid credentials'))
    renderLogin()
    await userEvent.type(screen.getByLabelText('Email'), 'a@b.c')
    await userEvent.type(screen.getByLabelText('Mot de passe'), 'wrongpass')
    await userEvent.click(screen.getByRole('button', { name: 'Se connecter' }))
    await waitFor(() => expect(screen.getByText('invalid credentials')).toBeInTheDocument())
  })

  it('toggles password visibility with the eye button', async () => {
    renderLogin()
    const pw = screen.getByLabelText('Mot de passe')
    expect(pw).toHaveAttribute('type', 'password')
    await userEvent.click(screen.getByRole('button', { name: 'Afficher le mot de passe' }))
    expect(pw).toHaveAttribute('type', 'text')
    await userEvent.click(screen.getByRole('button', { name: 'Masquer le mot de passe' }))
    expect(pw).toHaveAttribute('type', 'password')
  })

  it('disables the submit button and shows loading label while pending', async () => {
    let resolveLogin!: (v: { user: { id: number; email: string; name: string; role: string }; token: string }) => void
    vi.spyOn(api, 'login').mockImplementation(() => new Promise(res => { resolveLogin = res }))
    renderLogin()
    await userEvent.type(screen.getByLabelText('Email'), 'a@b.c')
    await userEvent.type(screen.getByLabelText('Mot de passe'), 'pw12345678')
    await userEvent.click(screen.getByRole('button', { name: 'Se connecter' }))
    const pending = await screen.findByRole('button', { name: 'Connexion…' })
    expect(pending).toBeDisabled()
    resolveLogin({ user: { id: 1, email: 'a@b.c', name: '', role: 'developer' }, token: 'jwt' })
    await waitFor(() => expect(screen.getByText('CATALOG HOME')).toBeInTheDocument())
  })

  it('renders the blueprint placeholder controls', () => {
    renderLogin()
    expect(screen.getByText('Rester connecté')).toBeInTheDocument()
    expect(screen.getByText('Mot de passe oublié ?')).toBeInTheDocument()
    expect(screen.getByText('Se connecter via votre entreprise')).toBeInTheDocument()
    expect(screen.getByText(/Conditions/)).toBeInTheDocument()
  })
})

function renderRegister() {
  return render(
    <LanguageProvider>
      <MemoryRouter initialEntries={['/register']}>
        <AuthProvider>
          <Routes>
            <Route path="/register" element={<RegisterPage />} />
            <Route path="/" element={<div>CATALOG HOME</div>} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    </LanguageProvider>
  )
}

describe('RegisterPage', () => {
  it('registers and navigates home on success', async () => {
    vi.spyOn(api, 'register').mockResolvedValue({ user: { id: 1, email: 'a@b.c', name: 'Ana', role: 'developer' }, token: 'jwt' })
    renderRegister()
    await userEvent.type(screen.getByLabelText('Nom'), 'Ana')
    await userEvent.type(screen.getByLabelText('Email'), 'a@b.c')
    await userEvent.type(screen.getByLabelText('Mot de passe'), 'pw12345678')
    await userEvent.click(screen.getByRole('button', { name: 'Créer le compte' }))
    await waitFor(() => expect(screen.getByText('CATALOG HOME')).toBeInTheDocument())
  })

  it('shows a field error when the password is under 8 characters', async () => {
    const spy = vi.spyOn(api, 'register')
    renderRegister()
    await userEvent.type(screen.getByLabelText('Email'), 'a@b.c')
    await userEvent.type(screen.getByLabelText('Mot de passe'), 'short')
    await userEvent.click(screen.getByRole('button', { name: 'Créer le compte' }))
    expect(await screen.findByText('Mot de passe : 8 caractères minimum')).toBeInTheDocument()
    expect(spy).not.toHaveBeenCalled()
  })

  it('shows the server error on failure', async () => {
    vi.spyOn(api, 'register').mockRejectedValue(new Error('email already used'))
    renderRegister()
    await userEvent.type(screen.getByLabelText('Email'), 'a@b.c')
    await userEvent.type(screen.getByLabelText('Mot de passe'), 'pw12345678')
    await userEvent.click(screen.getByRole('button', { name: 'Créer le compte' }))
    await waitFor(() => expect(screen.getByText('email already used')).toBeInTheDocument())
  })

  it('clears the password field error as soon as the user edits the field', async () => {
    renderRegister()
    await userEvent.type(screen.getByLabelText('Email'), 'a@b.c')
    await userEvent.type(screen.getByLabelText('Mot de passe'), 'short')
    await userEvent.click(screen.getByRole('button', { name: 'Créer le compte' }))
    expect(await screen.findByText('Mot de passe : 8 caractères minimum')).toBeInTheDocument()
    await userEvent.type(screen.getByLabelText('Mot de passe'), 'x')
    expect(screen.queryByText('Mot de passe : 8 caractères minimum')).not.toBeInTheDocument()
  })
})
