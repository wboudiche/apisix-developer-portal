import { useEffect, useState, type ReactNode } from 'react'
import { getProducts } from '../api/client'
import '../styles/auth.css'

// Blueprint fallback numbers, used when the public catalog can't be fetched.
const FALLBACK = { apis: 9, categories: 4 }

// APISIX triangle mark from the login.html blueprint (white square chrome is CSS).
function Mark() {
  return (
    <svg viewBox="0 0 48 48" aria-hidden="true">
      <path fill="currentColor" d="M24 3 L45 45 L24 45 Z" />
      <path fill="currentColor" fillOpacity=".62" d="M24 3 L24 45 L3 45 Z" />
      <path fill="#fff" d="M24 16 L31 31 L17 31 Z" />
    </svg>
  )
}

function Brand() {
  return (
    <>
      <span className="a-mark"><Mark /></span>
      <span>
        <span className="name">APISIX</span>
        <span className="sub">Portail Développeur</span>
      </span>
    </>
  )
}

export function AuthShell({ children }: { children: ReactNode }) {
  const [stats, setStats] = useState(FALLBACK)

  useEffect(() => {
    let alive = true
    getProducts({})
      .then(ps => {
        if (!alive) return
        setStats({ apis: ps.length, categories: new Set(ps.map(p => p.category)).size })
      })
      .catch(() => { /* keep blueprint fallback */ })
    return () => { alive = false }
  }, [])

  return (
    <div className="auth-shell">
      <aside className="aside">
        <div className="a-brand"><Brand /></div>
        <div className="a-body">
          <span className="a-eyebrow"><span className="dot" /> Tous les services · 100 % disponibles</span>
          <h1>Vos API, un seul portail.</h1>
          <p>Parcourez le catalogue, testez les points de terminaison et gérez vos abonnements — tout au même endroit.</p>
        </div>
        <div className="a-stats">
          <div className="s"><b>{stats.apis}</b><span>API publiées</span></div>
          <div className="s"><b>{stats.categories}</b><span>catégories</span></div>
          <div className="s"><b>99.9%</b><span>disponibilité</span></div>
        </div>
      </aside>

      <main className="auth-main">
        <div className="acard">
          <div className="m-brand"><Brand /></div>
          {children}
        </div>
      </main>
    </div>
  )
}
