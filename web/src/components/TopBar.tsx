import { useTheme } from '../theme/ThemeProvider'
import { useAuth } from '../auth/AuthProvider'
import { Link } from 'react-router-dom'

export function TopBar({ search, onSearch }: { search: string; onSearch: (v: string) => void }) {
  const { theme, toggle } = useTheme()
  const { user, logout } = useAuth()
  return (
    <header className="topbar">
      <Link className="brand" to="/"><span className="name">Atlas</span></Link>
      <nav className="nav-tabs"><Link className="active" to="/">APIs</Link>{user && <Link to="/applications">Applications</Link>}</nav>
      <div className="search">
        <input value={search} onChange={e => onSearch(e.target.value)} placeholder="Rechercher une API, un tag…" aria-label="Rechercher" />
      </div>
      <button className="icon-btn" onClick={toggle} aria-label="Basculer le thème">{theme === 'dark' ? '☀' : '☾'}</button>
      {user
        ? <button className="icon-btn" onClick={logout} aria-label="Se déconnecter" title={`Se déconnecter (${user.email})`}>{user.email.slice(0, 2).toUpperCase()}</button>
        : <Link className="icon-btn" to="/login">Connexion</Link>}
    </header>
  )
}
