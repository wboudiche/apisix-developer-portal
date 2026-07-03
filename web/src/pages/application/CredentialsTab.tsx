import { useEffect, useState } from 'react'
import { maskKey, copyText } from './helpers'
import { formatRelative } from './activity'
import { rotateKey, enableSandbox, rotateSandboxKey, setOidcClient } from '../../api/client'
import type { ModalSpec } from '../../components/ConfirmModal'
import { useT } from '../../i18n/LanguageProvider'

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

export function CredentialsTab({ apiKey, appId, token, lastRotatedAt, notify, openModal, onRotated,
  sandboxEnabled, sandboxGatewayUrl, sandboxEligible,
  oauthEligible, oidcClientId, oidcIssuer }: {
  apiKey: string
  appId: number
  token: string
  lastRotatedAt?: string
  notify: (msg: string) => void
  openModal: (spec: ModalSpec) => void
  onRotated: () => void
  sandboxEnabled?: boolean
  sandboxGatewayUrl?: string
  sandboxEligible: boolean
  oauthEligible?: boolean
  oidcClientId?: string
  oidcIssuer?: string
}) {
  const t = useT()
  const [shownKey, setShownKey] = useState(apiKey)
  const [revealed, setRevealed] = useState(false)
  // Keep the displayed key in sync when the prop changes (parent refetch / app switch).
  useEffect(() => { setShownKey(apiKey) }, [apiKey])

  const [sbKey, setSbKey] = useState('')        // revealed once after enable/rotate
  const [sbRevealed, setSbRevealed] = useState(false)
  const [sbBusy, setSbBusy] = useState(false)
  const hasSandbox = (sandboxEnabled ?? false) || sbKey !== ''

  const [clientId, setClientId] = useState(oidcClientId ?? '')
  const [oauthBusy, setOauthBusy] = useState(false)
  useEffect(() => { setClientId(oidcClientId ?? '') }, [oidcClientId])

  async function onSaveClientId() {
    if (oauthBusy) return
    setOauthBusy(true)
    try {
      await setOidcClient(token, appId, clientId.trim())
      notify(t('app.oidcSavedNotify')); onRotated()
    } catch (e) {
      notify(e instanceof Error ? e.message : t('app.oidcSaveFailed'))
    } finally { setOauthBusy(false) }
  }

  async function onEnableSandbox() {
    if (sbBusy) return
    setSbBusy(true)
    try {
      const { sandboxApiKey } = await enableSandbox(token, appId)
      setSbKey(sandboxApiKey); setSbRevealed(true)
      notify(t('app.sandboxEnabledNotify')); onRotated()
    } catch (e) {
      notify(e instanceof Error ? e.message : t('app.sandboxEnableFailed'))
    } finally { setSbBusy(false) }
  }

  function onRotateSandbox() {
    openModal({
      title: t('app.sandboxRotateConfirmTitle'),
      body: t('app.sandboxRotateConfirmBody'),
      confirmLabel: t('app.regenerateKeyAction'), danger: true,
      onConfirm: async () => {
        try {
          const { sandboxApiKey } = await rotateSandboxKey(token, appId)
          setSbKey(sandboxApiKey); setSbRevealed(true)
          notify(t('app.sandboxRotatedNotify')); onRotated()
        } catch (e) {
          notify(e instanceof Error ? e.message : t('app.rotateFailed'))
        }
      },
    })
  }

  function copy() {
    void copyText(shownKey).then(() => notify(t('app.keyCopiedNotify')))
  }

  function onRotate() {
    openModal({
      title: t('app.prodRotateConfirmTitle'),
      body: t('app.prodRotateConfirmBody'),
      confirmLabel: t('app.regenerateKeyAction'),
      danger: true,
      onConfirm: async () => {
        try {
          const { apiKey: nk } = await rotateKey(token, appId)
          setShownKey(nk)
          setRevealed(true)        // reveal the fresh key once
          notify(t('app.newKeyGeneratedNotify'))
          onRotated()              // refresh the detail (events / timestamp)
        } catch (e) {
          notify(e instanceof Error ? e.message : t('app.rotateFailed'))
        }
      },
    })
  }

  return (
    <section className="panel">
      <p className="section-title">{t('app.credsTitle')}</p>
      <div className="keygrid">
        <div className="keycard prod">
          <div className="kh"><span className="env">{t('product.tryProd')} <span className="envtag">live</span></span></div>
          <div className="kb">
            <div className="keyrow">
              <code data-testid="key-prod">{revealed ? shownKey : maskKey(shownKey)}</code>
              <button className="iconbtn" aria-label={t('app.toggleReveal')} aria-pressed={revealed} onClick={() => setRevealed(r => !r)}><EyeIcon /></button>
              <button className="iconbtn" aria-label={t('subscribeModal.copy')} onClick={copy}><CopyIcon /></button>
            </div>
            <div className="keymeta">
              <span>{t('app.lastRotationPrefix')}<span className="mono">{lastRotatedAt ? formatRelative(lastRotatedAt) : '—'}</span></span>
              <button className="rotate" onClick={onRotate}><RotateIcon />{t('app.regenerate')}</button>
            </div>
          </div>
        </div>
        {sandboxEligible && (
          <div className="keycard sandbox">
            <div className="kh"><span className="env">{t('product.trySandbox')} <span className="envtag">test</span></span></div>
            <div className="kb">
              {hasSandbox ? (
                <>
                  <div className="keyrow">
                    <code data-testid="key-sandbox">{sbRevealed && sbKey ? sbKey : maskKey(sbKey || '••••••••••••••••')}</code>
                    {sbKey && (
                      <button className="iconbtn" aria-label={t('app.toggleReveal')} aria-pressed={sbRevealed} onClick={() => setSbRevealed(r => !r)}><EyeIcon /></button>
                    )}
                    {sbKey && (
                      <button className="iconbtn" aria-label={t('subscribeModal.copy')} onClick={() => void copyText(sbKey).then(() => notify(t('app.sandboxKeyCopiedNotify')))}><CopyIcon /></button>
                    )}
                  </div>
                  <div className="keymeta">
                    <span>{t('app.gatewayPrefix')}<span className="mono">{sandboxGatewayUrl || '—'}</span></span>
                    <button className="rotate" onClick={onRotateSandbox}><RotateIcon />{t('app.regenerate')}</button>
                  </div>
                  {!sbKey && <p className="keyhint">{t('app.sandboxRegenHint')}</p>}
                </>
              ) : (
                <div className="keymeta">
                  <span>{t('app.sandboxIntro')}</span>
                  <button className="rotate" disabled={sbBusy} onClick={onEnableSandbox}>{t('app.enableSandbox')}</button>
                </div>
              )}
            </div>
          </div>
        )}
        {oauthEligible && (
          <div className="keycard oauth">
            <div className="kh"><span className="env">OAuth2 <span className="envtag">OIDC</span></span></div>
            <div className="kb">
              <label htmlFor="oidc-cid" className="oauthlabel">{t('app.oauthClientIdLabel')}</label>
              <div className="keyrow">
                <input id="oidc-cid" className="ipt mono" placeholder={t('app.oauthClientIdPlaceholder')}
                  value={clientId} onChange={e => setClientId(e.target.value)} />
                <button className="btn btn-primary" disabled={oauthBusy} onClick={onSaveClientId}>{t('common.save')}</button>
              </div>
              <p className="keymeta">
                <span>{t('app.issuerPrefix')}<span className="mono">{oidcIssuer || '—'}</span></span>
                <span><span className="mono">grant_type=client_credentials</span></span>
              </p>
              <p className="keyhint">{t('app.oauthHintPre')}<span className="mono">client_id</span>{t('app.oauthHintPost')}</p>
            </div>
          </div>
        )}
      </div>

      <div className="dcard" style={{ marginTop: 20 }}>
        <div className="ch"><h3>{t('app.securityTitle')}</h3></div>
        <div className="cb" style={{ display: 'flex', gap: 30, flexWrap: 'wrap', fontSize: '13.5px', color: 'var(--muted)', lineHeight: 1.6 }}>
          <div style={{ flex: 1, minWidth: 240 }}><b style={{ color: 'var(--fg)', display: 'block', marginBottom: 5 }}>{t('app.secShareTitle')}</b>{t('app.secShareBody')}</div>
          <div style={{ flex: 1, minWidth: 240 }}><b style={{ color: 'var(--fg)', display: 'block', marginBottom: 5 }}>{t('app.secRotateTitle')}</b>{t('app.secRotateBodyPre')}<span className="mono">consumer</span>{t('app.secRotateBodyPost')}</div>
          <div style={{ flex: 1, minWidth: 240 }}><b style={{ color: 'var(--fg)', display: 'block', marginBottom: 5 }}>{t('app.secOauthTitle')}</b>{t('app.secOauthBodyPre')}<span className="mono">client_id</span>{t('app.secOauthBodyPost')}</div>
        </div>
      </div>
    </section>
  )
}
