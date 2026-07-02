import { useRef, useEffect, useState, useCallback } from 'react'
import { useTheme } from '../theme/ThemeProvider'
import { useAuth } from '../auth/AuthProvider'
import { Link, useLocation } from 'react-router-dom'
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

// ── Profile dropdown icons ─────────────────────────────────────────────────────

function IconChevron() {
  return (
    <svg className="chev" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
      <path d="M6 9l6 6 6-6" strokeLinecap="round" strokeLinejoin="round"/>
    </svg>
  )
}

function IconLogout() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8}>
      <path d="M9 21H5a2 2 0 01-2-2V5a2 2 0 012-2h4M16 17l5-5-5-5M21 12H9" strokeLinecap="round" strokeLinejoin="round"/>
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
  const { pathname } = useLocation()
  // Route-derived active tab. Admin spans several sub-routes (/admin/products,
  // /admin/plans, /admin/approvals), so match the whole section, not one path.
  const tab = (active: boolean) => (active ? 'active' : undefined)
  const searchRef = useRef<HTMLInputElement>(null)
  const [menuOpen, setMenuOpen] = useState(false)
  const wrapRef = useRef<HTMLDivElement>(null)
  const menuOpenRef = useRef(menuOpen)

  useEffect(() => { menuOpenRef.current = menuOpen }, [menuOpen])

  // Close menu when user becomes null (logout from outside or session expiry)
  useEffect(() => { if (!user) setMenuOpen(false) }, [user])

  const handleLogout = useCallback(() => {
    setMenuOpen(false)
    logout()
  }, [logout])

  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if (menuOpenRef.current) return
      if (
        e.key === '/' &&
        document.activeElement?.tagName !== 'INPUT' &&
        document.activeElement?.tagName !== 'TEXTAREA' &&
        !(document.activeElement as HTMLElement)?.isContentEditable
      ) {
        e.preventDefault()
        searchRef.current?.focus()
      }
    }
    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [])

  // Close menu on outside click or Escape
  useEffect(() => {
    if (!menuOpen) return

    function handleMouseDown(e: MouseEvent) {
      if (wrapRef.current && !wrapRef.current.contains(e.target as Node)) {
        setMenuOpen(false)
      }
    }
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        setMenuOpen(false)
      }
    }

    document.addEventListener('mousedown', handleMouseDown)
    document.addEventListener('keydown', handleKeyDown)
    return () => {
      document.removeEventListener('mousedown', handleMouseDown)
      document.removeEventListener('keydown', handleKeyDown)
    }
  }, [menuOpen])

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
          <img src="/brand-apisix.svg" alt="" aria-hidden="true" />
        </span>
        <span>
          <span className="name">APISIX</span>{' '}
          <span className="sub">Portail Développeur</span>
        </span>
      </Link>

      <nav className="nav-tabs">
        <Link className={tab(pathname === '/')} to="/"><IconGrid />APIs</Link>
        {user && <Link className={tab(pathname.startsWith('/applications'))} to="/applications"><IconDoc />Applications</Link>}
        {user && <Link className={tab(pathname.startsWith('/teams'))} to="/teams"><IconDoc />Équipes</Link>}
        {user?.role === 'admin' && <Link className={tab(pathname.startsWith('/admin'))} to="/admin/products"><IconShield />Admin</Link>}
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

      {user ? (
        <div className="usermenu-wrap" ref={wrapRef}>
          <button
            className="user"
            onClick={() => setMenuOpen(o => !o)}
            aria-label={`Menu de ${displayName(user)}`}
            aria-haspopup="menu"
            aria-expanded={menuOpen}
          >
            <span className="av">{initials(user)}</span>
            <span className="who">{displayName(user)}<small>Espace développeur</small></span>
            <IconChevron />
          </button>

          {menuOpen && (
            <div className="usermenu" role="menu">
              <div className="head">
                <span className="email">{user.email}</span>
                <span className="role">
                  <span
                    className="dot"
                    style={{ background: user.role === 'admin' ? 'var(--accent)' : 'var(--c-finance)' }}
                  />
                  {user.role === 'admin' ? 'Admin' : 'Développeur'}
                </span>
              </div>
              <div className="sep" />
              <button
                className="item"
                role="menuitem"
                onClick={handleLogout}
              >
                <IconLogout />
                Se déconnecter
              </button>
            </div>
          )}
        </div>
      ) : (
        <Link className="login-cta" to="/login">Connexion</Link>
      )}
    </header>
  )
}
