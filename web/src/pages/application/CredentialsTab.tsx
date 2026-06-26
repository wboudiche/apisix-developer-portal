import { useEffect, useState } from 'react'
import { maskKey, copyText } from './helpers'
import { formatRelative } from './activity'
import { rotateKey } from '../../api/client'
import type { ModalSpec } from '../../components/ConfirmModal'

function EyeIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} aria-hidden="true">
      <path d="M2 12s4-7 10-7 10 7 10 7-4 7-10 7-10-7-10-7z" /><circle cx="12" cy="12" r="3" />
    </svg>
  )
}
function CopyIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} aria-hidden="true">
      <rect x="9" y="9" width="11" height="11" rx="2" /><path d="M5 15V5a2 2 0 012-2h10" strokeLinecap="round" />
    </svg>
  )
}
function RotateIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.9} aria-hidden="true">
      <path d="M21 2v6h-6M3 12a9 9 0 0115-6.7L21 8M3 22v-6h6M21 12a9 9 0 01-15 6.7L3 16" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

export function CredentialsTab({ apiKey, appId, token, lastRotatedAt, notify, openModal, onRotated }: {
  apiKey: string
  appId: number
  token: string
  lastRotatedAt?: string
  notify: (msg: string) => void
  openModal: (spec: ModalSpec) => void
  onRotated: () => void
}) {
  const [shownKey, setShownKey] = useState(apiKey)
  const [revealed, setRevealed] = useState(false)
  // Keep the displayed key in sync when the prop changes (parent refetch / app switch).
  useEffect(() => { setShownKey(apiKey) }, [apiKey])

  function copy() {
    void copyText(shownKey).then(() => notify('Clé copiée dans le presse-papiers'))
  }

  function onRotate() {
    openModal({
      title: 'Régénérer la clé production ?',
      body: 'L’ancienne clé sera révoquée immédiatement dans APISIX (consumer key-auth). Les requêtes qui l’utilisent recevront un 401 — pensez à redéployer.',
      confirmLabel: 'Régénérer la clé',
      danger: true,
      onConfirm: async () => {
        try {
          const { apiKey: nk } = await rotateKey(token, appId)
          setShownKey(nk)
          setRevealed(true)        // reveal the fresh key once
          notify('Nouvelle clé générée')
          onRotated()              // refresh the detail (events / timestamp)
        } catch (e) {
          notify(e instanceof Error ? e.message : 'Échec de la rotation.')
        }
      },
    })
  }

  return (
    <section className="panel">
      <p className="section-title">Clés API · key-auth</p>
      <div className="keygrid">
        <div className="keycard prod">
          <div className="kh"><span className="env">Production <span className="envtag">live</span></span></div>
          <div className="kb">
            <div className="keyrow">
              <code data-testid="key-prod">{revealed ? shownKey : maskKey(shownKey)}</code>
              <button className="iconbtn" aria-label="Afficher / masquer" aria-pressed={revealed} onClick={() => setRevealed(r => !r)}><EyeIcon /></button>
              <button className="iconbtn" aria-label="Copier" onClick={copy}><CopyIcon /></button>
            </div>
            <div className="keymeta">
              <span>Dernière rotation · <span className="mono">{lastRotatedAt ? formatRelative(lastRotatedAt) : '—'}</span></span>
              <button className="rotate" onClick={onRotate}><RotateIcon />Régénérer</button>
            </div>
          </div>
        </div>
      </div>

      <div className="dcard" style={{ marginTop: 20 }}>
        <div className="ch"><h3>Sécurité de la clé</h3></div>
        <div className="cb" style={{ display: 'flex', gap: 30, flexWrap: 'wrap', fontSize: '13.5px', color: 'var(--muted)', lineHeight: 1.6 }}>
          <div style={{ flex: 1, minWidth: 240 }}><b style={{ color: 'var(--fg)', display: 'block', marginBottom: 5 }}>Ne la partagez jamais côté client</b>La clé porte tous les droits de l&apos;application. Gardez-la côté serveur ou dans un secret manager.</div>
          <div style={{ flex: 1, minWidth: 240 }}><b style={{ color: 'var(--fg)', display: 'block', marginBottom: 5 }}>Régénérer invalide l&apos;ancienne</b>La rotation révoque immédiatement le <span className="mono">consumer</span> précédent dans APISIX. Prévoyez le redéploiement.</div>
          <div style={{ flex: 1, minWidth: 240 }}><b style={{ color: 'var(--fg)', display: 'block', marginBottom: 5 }}>OAuth2 / JWT à venir</b>Le portail est prêt pour un second fournisseur d&apos;identifiants (<span className="mono">jwt-auth</span>) sans réécriture.</div>
        </div>
      </div>
    </section>
  )
}
