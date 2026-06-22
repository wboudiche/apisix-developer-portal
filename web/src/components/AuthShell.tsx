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
      .then(r => {
        if (!alive) return
        setStats({ apis: r.total, categories: new Set(r.items.map(p => p.category)).size })
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

          <ul className="a-feats">
            <li>
              <span className="fi">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><circle cx="11" cy="11" r="7" /><path d="m21 21-4.3-4.3" /></svg>
              </span>
              <span className="ft"><b>Catalogue unifié</b><span>9 API documentées, recherche et filtres par catégorie.</span></span>
            </li>
            <li>
              <span className="fi">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="M21 2l-2 2m-7.6 7.6a5.5 5.5 0 1 0-1 1l8.6-8.6" /><circle cx="7.5" cy="15.5" r="1.5" /></svg>
              </span>
              <span className="ft"><b>Clés en libre-service</b><span>Identifiants Prod & Sandbox en <code>key-auth</code>, révocables.</span></span>
            </li>
            <li>
              <span className="fi">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="M3 3v18h18" /><path d="m7 14 4-4 3 3 5-6" /></svg>
              </span>
              <span className="ft"><b>Quotas & abonnements</b><span>Paliers Free, Silver, Gold avec suivi de consommation.</span></span>
            </li>
          </ul>
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
