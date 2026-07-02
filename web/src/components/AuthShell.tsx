import { useEffect, useState, type ReactNode } from 'react'
import { getProducts } from '../api/client'
import { useT } from '../i18n/LanguageProvider'
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
  const t = useT()
  return (
    <>
      <span className="a-mark"><Mark /></span>
      <span>
        <span className="name">APISIX</span>
        <span className="sub">{t('nav.brandSub')}</span>
      </span>
    </>
  )
}

export function AuthShell({ children }: { children: ReactNode }) {
  const t = useT()
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
          <span className="a-eyebrow"><span className="dot" /> {t('authShell.eyebrow')}</span>
          <h1>{t('authShell.heading')}</h1>
          <p>{t('authShell.lead')}</p>

          <ul className="a-feats">
            <li>
              <span className="fi">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><circle cx="11" cy="11" r="7" /><path d="m21 21-4.3-4.3" /></svg>
              </span>
              <span className="ft"><b>{t('authShell.feat1Title')}</b><span>{t('authShell.feat1Desc')}</span></span>
            </li>
            <li>
              <span className="fi">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="M21 2l-2 2m-7.6 7.6a5.5 5.5 0 1 0-1 1l8.6-8.6" /><circle cx="7.5" cy="15.5" r="1.5" /></svg>
              </span>
              <span className="ft"><b>{t('authShell.feat2Title')}</b><span>{t('authShell.feat2Pre')}<code>key-auth</code>{t('authShell.feat2Post')}</span></span>
            </li>
            <li>
              <span className="fi">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="M3 3v18h18" /><path d="m7 14 4-4 3 3 5-6" /></svg>
              </span>
              <span className="ft"><b>{t('authShell.feat3Title')}</b><span>{t('authShell.feat3Desc')}</span></span>
            </li>
          </ul>
        </div>
        <div className="a-stats">
          <div className="s"><b>{stats.apis}</b><span>{t('authShell.statApis')}</span></div>
          <div className="s"><b>{stats.categories}</b><span>{t('authShell.statCategories')}</span></div>
          <div className="s"><b>99.9%</b><span>{t('authShell.statUptime')}</span></div>
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
