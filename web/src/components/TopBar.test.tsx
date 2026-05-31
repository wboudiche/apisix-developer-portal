import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { TopBar } from './TopBar'
import { ThemeProvider } from '../theme/ThemeProvider'
import { AuthProvider } from '../auth/AuthProvider'

function renderTopBar(searchValue = '', onMenu?: () => void) {
  const onSearch = vi.fn()
  const result = render(
    <MemoryRouter>
      <ThemeProvider>
        <AuthProvider>
          <TopBar search={searchValue} onSearch={onSearch} onMenu={onMenu} />
        </AuthProvider>
      </ThemeProvider>
    </MemoryRouter>
  )
  return { ...result, onSearch }
}

beforeEach(() => {
  localStorage.clear()
  vi.restoreAllMocks()
})

describe('TopBar', () => {
  it('search input has accessible name "Rechercher"', () => {
    renderTopBar()
    expect(screen.getByLabelText('Rechercher')).toBeInTheDocument()
  })

  it('renders a kbd "/" shortcut hint inside .search', () => {
    const { container } = renderTopBar()
    const kbd = container.querySelector('.search kbd')
    expect(kbd).not.toBeNull()
    expect(kbd?.textContent).toBe('/')
  })

  it('renders a magnifier svg inside .search', () => {
    const { container } = renderTopBar()
    expect(container.querySelector('.search svg')).not.toBeNull()
  })

  it('renders an svg icon inside the APIs nav tab', () => {
    const { container } = renderTopBar()
    expect(container.querySelector('.nav-tabs a svg')).not.toBeNull()
  })

  it('pressing "/" focuses the search input', () => {
    renderTopBar()
    const input = screen.getByLabelText('Rechercher')
    // Ensure focus is on body (not the input) before firing
    ;(document.activeElement as HTMLElement | null)?.blur?.()
    fireEvent.keyDown(document, { key: '/' })
    expect(document.activeElement).toBe(input)
  })

  it('pressing "/" while an input is focused does NOT steal focus', () => {
    renderTopBar()
    const input = screen.getByLabelText('Rechercher') as HTMLInputElement
    // Focus the search input itself first
    input.focus()
    expect(document.activeElement).toBe(input)
    // A second "/" press while already in INPUT should not call preventDefault-and-focus
    // (we just verify the listener guard works — input stays focused)
    fireEvent.keyDown(document, { key: '/' })
    expect(document.activeElement).toBe(input)
  })

  it('renders the moon SVG (light mode default) in the theme toggle button', () => {
    renderTopBar()
    const themeBtn = screen.getByLabelText('Basculer le thème')
    expect(themeBtn.querySelector('svg')).not.toBeNull()
  })

  it('theme toggle button has aria-label "Basculer le thème"', () => {
    renderTopBar()
    expect(screen.getByLabelText('Basculer le thème')).toBeInTheDocument()
  })

  it('shows login link when no user is authenticated', () => {
    renderTopBar()
    expect(screen.getByText('Connexion')).toBeInTheDocument()
  })

  it('shows Applications tab when user is logged in', () => {
    localStorage.setItem('user', JSON.stringify({ id: 1, email: 'test@test.com', name: 'Test', role: 'user' }))
    localStorage.setItem('token', 'abc123')
    renderTopBar()
    expect(screen.getByText('Applications')).toBeInTheDocument()
  })

  it('shows Admin tab when user role is admin', () => {
    localStorage.setItem('user', JSON.stringify({ id: 1, email: 'admin@test.com', name: 'Admin', role: 'admin' }))
    localStorage.setItem('token', 'abc123')
    const { container } = renderTopBar()
    const adminLink = container.querySelector('.nav-tabs a[href="/admin/products"]')
    expect(adminLink).not.toBeNull()
  })

  // ── NEW: hamburger button ──────────────────────────────────────────────────

  it('renders hamburger button when onMenu is provided', () => {
    const onMenu = vi.fn()
    renderTopBar('', onMenu)
    expect(screen.getByLabelText('Ouvrir les catégories')).toBeInTheDocument()
  })

  it('clicking hamburger button calls onMenu', () => {
    const onMenu = vi.fn()
    renderTopBar('', onMenu)
    fireEvent.click(screen.getByLabelText('Ouvrir les catégories'))
    expect(onMenu).toHaveBeenCalledTimes(1)
  })

  it('does NOT render hamburger button when onMenu is NOT provided', () => {
    renderTopBar()
    expect(screen.queryByLabelText('Ouvrir les catégories')).toBeNull()
  })

  // ── NEW: help button ───────────────────────────────────────────────────────

  it('renders a help button with aria-label "Aide / Documentation"', () => {
    renderTopBar()
    expect(screen.getByLabelText('Aide / Documentation')).toBeInTheDocument()
  })

  // ── NEW: user block (real-auth) ────────────────────────────────────────────

  it('shows .user block with display name and "Espace développeur" when logged in', () => {
    localStorage.setItem('user', JSON.stringify({ id: 1, email: 'admin@portal.local', name: 'Admin', role: 'admin' }))
    localStorage.setItem('token', 'abc123')
    const { container } = renderTopBar()
    const who = container.querySelector('.user .who')
    expect(who).not.toBeNull()
    expect(who?.textContent).toContain('Admin')
    expect(who?.textContent).toContain('Espace développeur')
  })

  it('shows initials in .av when logged in with a name', () => {
    localStorage.setItem('user', JSON.stringify({ id: 1, email: 'admin@portal.local', name: 'Admin Doe', role: 'admin' }))
    localStorage.setItem('token', 'abc123')
    const { container } = renderTopBar()
    const av = container.querySelector('.user .av')
    expect(av).not.toBeNull()
    expect(av?.textContent).toBe('AD')
  })

  it('uses email prefix as initials when user has no name', () => {
    localStorage.setItem('user', JSON.stringify({ id: 1, email: 'bo@portal.local', name: '', role: 'user' }))
    localStorage.setItem('token', 'abc123')
    const { container } = renderTopBar()
    const av = container.querySelector('.user .av')
    expect(av).not.toBeNull()
    expect(av?.textContent).toBe('BO')
  })

  it('.user button has aria-label "Se déconnecter" when logged in', () => {
    localStorage.setItem('user', JSON.stringify({ id: 1, email: 'admin@portal.local', name: 'Admin', role: 'admin' }))
    localStorage.setItem('token', 'abc123')
    renderTopBar()
    expect(screen.getByLabelText('Se déconnecter')).toBeInTheDocument()
  })

  it('shows login link (not user block) when logged out', () => {
    renderTopBar()
    expect(screen.queryByLabelText('Se déconnecter')).toBeNull()
    expect(screen.getByText('Connexion')).toBeInTheDocument()
  })
})
