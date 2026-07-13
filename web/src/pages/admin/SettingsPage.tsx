import { useCallback, useEffect, useState } from 'react'
import { AdminShell } from './AdminShell'
import { useAuth } from '../../auth/AuthProvider'
import { useT } from '../../i18n/LanguageProvider'
import {
  adminGetSettings, adminPutSettings, adminResetSetting, adminTestSettings, SettingsSaveError,
} from '../../api/client'
import type { SettingsGroup, SettingItem, ProbeResult } from '../../api/types'
import { Toast, useToast } from '../../components/Toast'
import { ConfirmModal, type ModalSpec } from '../../components/ConfirmModal'
import '../../styles/admin-settings.css'

// The server group is fixed at process boot (env-only) — never editable
// from the UI regardless of what the registry reports per-item.
const READ_ONLY_GROUPS = new Set(['server'])

type TFunc = ReturnType<typeof useT>

function LockIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} aria-hidden="true">
      <rect x="5" y="11" width="14" height="9" rx="2" /><path d="M8 11V8a4 4 0 0 1 8 0v3" strokeLinecap="round" />
    </svg>
  )
}
function ResetIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.9} aria-hidden="true">
      <path d="M4 4v6h6" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M4 10a8 8 0 1 1-1.6 4.8" strokeLinecap="round" />
    </svg>
  )
}
function WarnIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.9} aria-hidden="true">
      <path d="M12 8v5" strokeLinecap="round" /><circle cx="12" cy="16.5" r=".5" fill="currentColor" stroke="none" />
      <path d="M10.3 3.9 2.7 17a2 2 0 0 0 1.7 3h15.2a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0Z" strokeLinejoin="round" />
    </svg>
  )
}

// One row: a readable label with the env-var key as a mono chip beneath it, a
// type-appropriate control bound to `draft` (bool -> toggle, else -> input),
// the value source, and — for overridden keys — a reset-to-default button.
function SettingRow({ item, group, draft, onChange, error, onReset, t }: {
  item: SettingItem
  group: string
  draft: Record<string, string>
  onChange: (item: SettingItem, value: string) => void
  error?: string
  onReset: (item: SettingItem) => void
  t: TFunc
}) {
  const readOnly = READ_ONLY_GROUPS.has(group) || !item.editable
  const isBool = item.type === 'bool'
  const draftValue = draft[item.key]

  let control
  if (isBool) {
    // Wire contract: a bool is "1" (on) or "" (off) — the backend's Validate
    // rejects any other encoding (sending "0" would 422).
    const checked = (draftValue ?? (item.value === '1' ? '1' : '')) === '1'
    control = (
      <label className="sswitch">
        <input
          type="checkbox"
          aria-label={item.key}
          checked={checked}
          disabled={readOnly}
          onChange={e => onChange(item, e.target.checked ? '1' : '')}
        />
        <span className={checked ? 'son' : ''}>{checked ? t('settings.on') : t('settings.off')}</span>
      </label>
    )
  } else {
    const shown = draftValue ?? (item.secret ? '' : (item.value ?? ''))
    control = (
      <input
        className="ipt"
        type={item.secret ? 'password' : 'text'}
        aria-label={item.key}
        disabled={readOnly}
        placeholder={item.secret && item.set ? t('settings.secretSet') : ''}
        autoComplete="off"
        value={shown}
        onChange={e => onChange(item, e.target.value)}
      />
    )
  }

  return (
    <div className={`srow${readOnly ? ' ro' : ''}`}>
      <div className="slabel">
        <span className="sname">{t(`settings.desc.${item.key}`)}</span>
        <span className="skey">{item.key}</span>
      </div>
      <div className={`sctl${error ? ' invalid' : ''}`}>
        {control}
        {error && <span className="serr"><WarnIcon />{error}</span>}
      </div>
      <div className="sright">
        {item.source === 'db'
          ? <span className="sbadge db" title={t('settings.badgeDbTitle')}><span className="pdot" />{t('settings.badgeDb')}</span>
          : <span className="sbadge" title={t('settings.badgeEnvTitle')}>{t('settings.badgeEnv')}</span>}
        {item.source === 'db' && (
          <button
            type="button" className="sreset"
            aria-label={t('settings.reset')} title={t('settings.reset')}
            onClick={() => onReset(item)}
          ><ResetIcon /></button>
        )}
      </div>
    </div>
  )
}

function ProbeChips({ probes }: { probes: ProbeResult[] }) {
  return (
    <div className="probes">
      {probes.map(p => (
        <div key={p.name} className={`probe ${p.ok ? 'ok' : 'ko'}`}>
          <span className="ptag">{p.name}</span>
          <span className="pdetail">{p.detail}</span>
        </div>
      ))}
    </div>
  )
}

function SaveBar({ dirtyCount, busy, probes, canForce, onTest, onSave, onForce, t }: {
  dirtyCount: number
  busy: boolean
  probes: ProbeResult[] | null
  canForce: boolean
  onTest: () => void
  onSave: () => void
  onForce: () => void
  t: TFunc
}) {
  if (dirtyCount === 0) return null
  return (
    <div className="savebar">
      {probes && <ProbeChips probes={probes} />}
      <div className="savebar-row">
        <span className="dirty"><span className="ddot" />{t('settings.dirtyCount', { n: dirtyCount })}</span>
        <button type="button" className="btn btn-ghost btn-sm" disabled={busy} onClick={onTest}>{t('settings.test')}</button>
        <button type="button" className="btn btn-primary btn-sm" disabled={busy} onClick={onSave}>{t('settings.save')}</button>
        {canForce && (
          <button type="button" className="btn btn-sm btn-force" disabled={busy} onClick={onForce}>{t('settings.saveForce')}</button>
        )}
      </div>
    </div>
  )
}

export function SettingsPage() {
  const { token } = useAuth()
  const t = useT()
  const { toast, notify } = useToast()
  const [groups, setGroups] = useState<SettingsGroup[]>([])
  const [draft, setDraft] = useState<Record<string, string>>({})
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [probes, setProbes] = useState<ProbeResult[] | null>(null)
  const [busy, setBusy] = useState(false)
  const [loadErr, setLoadErr] = useState('')
  const [modal, setModal] = useState<ModalSpec | null>(null)

  const reload = useCallback(() => {
    if (!token) return
    adminGetSettings(token).then(g => { setGroups(g); setLoadErr('') }).catch(() => setLoadErr(t('settings.loadError')))
  }, [token, t])
  useEffect(reload, [reload])

  const onChange = useCallback((item: SettingItem, value: string) => {
    setDraft(d => {
      const next = { ...d }
      if (item.secret) {
        // A secret only sends when non-empty: clearing the field back out
        // must not blank the stored secret, it just drops the edit.
        if (value === '') delete next[item.key]
        else next[item.key] = value
      } else {
        // Bools normalize to the wire encoding ("1" on / "" off) so an
        // edit-back-to-original correctly drops out of the draft.
        const original = item.type === 'bool' ? (item.value === '1' ? '1' : '') : (item.value ?? '')
        if (value === original) delete next[item.key]
        else next[item.key] = value
      }
      return next
    })
    setProbes(null)
    setErrors(e => {
      if (!(item.key in e)) return e
      const next = { ...e }
      delete next[item.key]
      return next
    })
  }, [])

  const dirtyCount = Object.keys(draft).length
  const canForce = !!probes?.some(p => !p.ok)

  async function save(force: boolean) {
    if (!token) return
    setBusy(true)
    try {
      await adminPutSettings(token, draft, force)
      notify(t('settings.saved'))
      setDraft({})
      setErrors({})
      setProbes(null)
      reload()
    } catch (e) {
      if (e instanceof SettingsSaveError) {
        setErrors(e.fields ?? {})
        setProbes(e.probe ?? null)
      } else {
        notify(e instanceof Error ? e.message : t('settings.loadError'), 'warn')
      }
    } finally {
      setBusy(false)
    }
  }

  async function test() {
    if (!token) return
    setBusy(true)
    try {
      setProbes(await adminTestSettings(token, draft))
    } catch (e) {
      notify(e instanceof Error ? e.message : t('settings.loadError'), 'warn')
    } finally {
      setBusy(false)
    }
  }

  function askReset(item: SettingItem) {
    setModal({
      title: t('settings.reset'),
      body: t('settings.resetConfirm', { key: item.key, value: item.envDefault ?? '' }),
      onConfirm: () => {
        if (!token) return
        adminResetSetting(token, item.key)
          .then(() => { notify(t('settings.saved')); reload() })
          .catch(() => notify(t('settings.loadError'), 'warn'))
      },
    })
  }

  return (
    <AdminShell active="settings" title={t('settings.title')} description={t('settings.description')}>
      {loadErr && <p className="autherr" role="alert">{loadErr}</p>}
      <div className="settings">
        {groups.map(g => {
          const locked = READ_ONLY_GROUPS.has(g.group)
          return (
            <div className="group" key={g.group}>
              <div className={`sgroup-head${locked ? ' locked' : ''}`}>
                <span className="dot" />
                <div className="titles">
                  <h3>{t(`settings.group.${g.group}`)}</h3>
                  <div className="gsub">{t(`settings.groupDesc.${g.group}`)}</div>
                </div>
                {locked && <span className="lock"><LockIcon />{t('settings.readOnlyHint')}</span>}
              </div>
              {g.items.map(item => (
                <SettingRow
                  key={item.key}
                  item={item}
                  group={g.group}
                  draft={draft}
                  onChange={onChange}
                  error={errors[item.key]}
                  onReset={askReset}
                  t={t}
                />
              ))}
            </div>
          )
        })}

        <SaveBar
          dirtyCount={dirtyCount}
          busy={busy}
          probes={probes}
          canForce={canForce}
          onTest={test}
          onSave={() => save(false)}
          onForce={() => save(true)}
          t={t}
        />
      </div>

      <Toast msg={toast?.msg ?? null} kind={toast?.kind} />
      <ConfirmModal spec={modal} onClose={() => setModal(null)} />
    </AdminShell>
  )
}
