import { useRef, useEffect } from 'react'
import { useTheme } from '../theme/ThemeProvider'
import { useAuth } from '../auth/AuthProvider'
import { Link } from 'react-router-dom'
import type { User } from '../api/types'

// ── Nav-tab icons ──────────────────────────────────────────────────────────────

function IconGrid() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.7}>
      <rect x="3" y="3" width="7" height="7" rx="1.5"/>
      <rect x="14" y="3" width="7" height="7" rx="1.5"/>
      <rect x="3" y="14" width="7" height="7" rx="1.5"/>
      <rect x="14" y="14" width="7" height="7" rx="1.5"/>
    </svg>
  )
}

function IconDoc() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.7}>
      <path d="M4 6a2 2 0 012-2h12a2 2 0 012 2v12a2 2 0 01-2 2H6a2 2 0 01-2-2z"/>
      <path d="M8 9h8M8 13h5" strokeLinecap="round"/>
    </svg>
  )
}

function IconShield() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.7}>
      <path d="M12 3l7 3v5c0 4-3 7-7 8-4-1-7-4-7-8V6z" strokeLinejoin="round"/>
      <path d="M9.5 12l1.8 1.8L15 10" strokeLinecap="round" strokeLinejoin="round"/>
    </svg>
  )
}

// ── Search magnifier ───────────────────────────────────────────────────────────

function IconSearch() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8}>
      <circle cx="11" cy="11" r="7"/>
      <path d="M21 21l-4-4" strokeLinecap="round"/>
    </svg>
  )
}

// ── Theme toggle glyphs ────────────────────────────────────────────────────────

function IconMoon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8}>
      <path d="M20 14.5A8 8 0 119.5 4a6.5 6.5 0 0010.5 10.5z" strokeLinejoin="round"/>
    </svg>
  )
}

function IconSun() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8}>
      <circle cx="12" cy="12" r="4"/>
      <path d="M12 2v2.5M12 19.5V22M2 12h2.5M19.5 12H22M5 5l1.8 1.8M17.2 17.2L19 19M19 5l-1.8 1.8M6.8 17.2L5 19" strokeLinecap="round"/>
    </svg>
  )
}

// ── User block helpers ─────────────────────────────────────────────────────────

function displayName(u: User): string {
  return u.name?.trim() || u.email
}

function initials(u: User): string {
  const name = u.name?.trim()
  if (name) {
    const words = name.split(/\s+/).filter(Boolean)
    return words.slice(0, 2).map(w => w[0]).join('').toUpperCase()
  }
  return u.email.slice(0, 2).toUpperCase()
}

// ── TopBar ─────────────────────────────────────────────────────────────────────

export function TopBar({
  search,
  onSearch,
  onMenu,
}: {
  search: string
  onSearch: (v: string) => void
  onMenu?: () => void
}) {
  const { theme, toggle } = useTheme()
  const { user, logout } = useAuth()
  const searchRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === '/' && document.activeElement?.tagName !== 'INPUT' && document.activeElement?.tagName !== 'TEXTAREA') {
        e.preventDefault()
        searchRef.current?.focus()
      }
    }
    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [])

  return (
    <header className="topbar">
      {onMenu && (
        <button className="icon-btn hamb" onClick={onMenu} aria-label="Ouvrir les catégories">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8}>
            <path d="M4 7h16M4 12h16M4 17h16" strokeLinecap="round"/>
          </svg>
        </button>
      )}

      <Link className="brand" to="/">
        <span className="mark">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8}><path d="M5 7l-3 5 3 5M19 7l3 5-3 5M14 4l-4 16" strokeLinecap="round" strokeLinejoin="round"/></svg>
        </span>
        <span>
          <span className="name">Atlas</span>
          <span className="sub">Portail Développeur</span>
        </span>
      </Link>

      <nav className="nav-tabs">
        <Link className="active" to="/"><IconGrid />APIs</Link>
        {user && <Link to="/applications"><IconDoc />Applications</Link>}
        {user?.role === 'admin' && <Link to="/admin/products"><IconShield />Admin</Link>}
      </nav>

      <div className="search">
        <IconSearch />
        <input
          ref={searchRef}
          value={search}
          onChange={e => onSearch(e.target.value)}
          placeholder="Rechercher une API, un tag, un contexte…"
          aria-label="Rechercher"
        />
        <kbd>/</kbd>
      </div>

      <button className="icon-btn" onClick={toggle} aria-label="Basculer le thème">
        {theme !== 'dark' ? <IconMoon /> : <IconSun />}
      </button>

      <button className="icon-btn" aria-label="Aide / Documentation">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8}>
          <circle cx="12" cy="12" r="9"/>
          <path d="M9.5 9a2.5 2.5 0 113.5 2.3c-.8.4-1 .9-1 1.7M12 17h.01" strokeLinecap="round"/>
        </svg>
      </button>

      {user ? (
        <button className="user" onClick={logout} aria-label="Se déconnecter" title={`Se déconnecter (${user.email})`}>
          <span className="av">{initials(user)}</span>
          <span className="who">{displayName(user)}<small>Espace développeur</small></span>
        </button>
      ) : (
        <Link className="icon-btn" to="/login" aria-label="Connexion">Connexion</Link>
      )}
    </header>
  )
}
