import { useState } from 'react'
import { maskKey, copyText } from './helpers'
import { DEMO_SANDBOX_KEY, DEMO_ROTATION, demoRotatedKey } from './demo'
import type { ModalSpec } from './ConfirmModal'

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

function KeyCard({ kind, label, tag, fullKey, revealed, onToggle, rotatedAt, onCopy, onRotate, testId }: {
  kind: 'prod' | 'sbx'
  label: string
  tag: string
  fullKey: string
  revealed: boolean
  onToggle: () => void
  rotatedAt: string
  onCopy: () => void
  onRotate: () => void
  testId: string
}) {
  return (
    <div className={`keycard ${kind}`}>
      <div className="kh">
        <span className="env">{label} <span className="envtag">{tag}</span></span>
      </div>
      <div className="kb">
        <div className="keyrow">
          <code data-testid={testId}>{revealed ? fullKey : maskKey(fullKey)}</code>
          <button className="iconbtn" aria-label="Afficher / masquer" onClick={onToggle}><EyeIcon /></button>
          <button className="iconbtn" aria-label="Copier" onClick={onCopy}><CopyIcon /></button>
        </div>
        <div className="keymeta">
          <span>Dernière rotation · <span className="mono">{rotatedAt}</span></span>
          <button className="rotate" onClick={onRotate}><RotateIcon />Régénérer</button>
        </div>
      </div>
    </div>
  )
}

export function CredentialsTab({ apiKey, notify, openModal }: {
  apiKey: string
  notify: (msg: string) => void
  openModal: (spec: ModalSpec) => void
}) {
  const [prodRevealed, setProdRevealed] = useState(false)
  const [sbxRevealed, setSbxRevealed] = useState(false)
  // DEMO: sandbox environments don't exist server-side yet (see demo.ts).
  const [sbxKey, setSbxKey] = useState(DEMO_SANDBOX_KEY)

  function copy(key: string) {
    void copyText(key).then(() => notify('Clé copiée dans le presse-papiers'))
  }

  return (
    <section className="panel">
      <p className="section-title">Clés API · key-auth</p>
      <div className="keygrid">
        <KeyCard
          kind="prod" label="Production" tag="live" testId="key-prod"
          fullKey={apiKey} revealed={prodRevealed} onToggle={() => setProdRevealed(r => !r)}
          rotatedAt={DEMO_ROTATION.prod}
          onCopy={() => copy(apiKey)}
          onRotate={() => openModal({
            title: 'Régénérer la clé production ?',
            body: 'L’ancienne clé serait révoquée immédiatement dans APISIX (consumer key-auth). La rotation de clé arrive bientôt côté portail.',
            confirmLabel: 'Régénérer la clé',
            // Real credential: no backend rotation endpoint yet — never fake it visually.
            onConfirm: () => notify('Rotation des clés à venir'),
          })}
        />
        <KeyCard
          kind="sbx" label="Sandbox" tag="test" testId="key-sbx"
          fullKey={sbxKey} revealed={sbxRevealed} onToggle={() => setSbxRevealed(r => !r)}
          rotatedAt={DEMO_ROTATION.sbx}
          onCopy={() => copy(sbxKey)}
          onRotate={() => openModal({
            title: 'Régénérer la clé sandbox ?',
            body: 'L’ancienne clé sera révoquée immédiatement dans APISIX (consumer key-auth). Les requêtes qui l’utilisent recevront un 401 — pensez à redéployer.',
            confirmLabel: 'Régénérer la clé',
            onConfirm: () => {
              const nk = demoRotatedKey('ax_test_')
              setSbxKey(nk)
              setSbxRevealed(true)   // blueprint reveals the fresh key once
              notify('Nouvelle clé sandbox générée')
            },
          })}
        />
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
