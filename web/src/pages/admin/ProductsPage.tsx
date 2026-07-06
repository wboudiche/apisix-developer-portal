import { useCallback, useEffect, useRef, useState } from 'react'
import type { AdminProduct, ChangelogEntry } from '../../api/types'
import { adminGetProducts, adminCreateProduct, adminUpdateProduct, adminDeleteProduct, adminGetChangelog, addChangelogEntry, deleteChangelogEntry, adminUploadProductIcon, adminFetchProductIcon, ApiError } from '../../api/client'
import { useAuth } from '../../auth/AuthProvider'
import { AdminShell } from './AdminShell'
import { Composer } from './Composer'
import { catMeta, slugify } from './meta'
import { Toast, useToast } from '../../components/Toast'
import { Pagination } from '../../components/Pagination'
import { ConfirmModal, type ModalSpec } from '../../components/ConfirmModal'
import { ImportModal } from './ImportModal'
import { useT } from '../../i18n/LanguageProvider'
import { BUILTIN_ICON_KEYS, ApiIcon } from '../../components/apiIcons'

interface FormState {
  name: string; slug: string; category: string; contextPath: string
  upstreamUrl: string; sandboxUpstreamUrl: string; authType: string; version: string; published: boolean; openapiSpec: string
  lifecycleStatus: string; sunsetDate: string; icon: string
}
const EMPTY: FormState = { name: '', slug: '', category: '', contextPath: '', upstreamUrl: '', sandboxUpstreamUrl: '', authType: 'key-auth', version: '1.0.0', published: true, openapiSpec: '', lifecycleStatus: 'active', sunsetDate: '', icon: '' }

const CHANGELOG_KINDS = ['added', 'changed', 'fixed', 'removed', 'deprecated', 'security'] as const

// Editor for a product's changelog: shown only while editing an existing
// product (needs its numeric id). Owns its own fetch/add/delete cycle,
// independent from the surrounding product-form state. Uses the ADMIN list
// endpoint (not the public published-only one) so drafts show their entries too.
function ChangelogEditor({ productId, token, notify }: {
  productId: number
  token: string
  notify: (msg: string, kind?: 'ok' | 'warn') => void
}) {
  const t = useT()
  const [entries, setEntries] = useState<ChangelogEntry[]>([])
  const [cVersion, setCVersion] = useState('')
  const [cKind, setCKind] = useState<string>('added')
  const [cDate, setCDate] = useState('')
  const [cNotes, setCNotes] = useState('')

  const reload = useCallback(() => {
    adminGetChangelog(token, productId).then(setEntries).catch(() => {})
  }, [token, productId])
  useEffect(reload, [reload])

  async function add() {
    if (!cVersion.trim() || !cDate.trim()) return
    try {
      await addChangelogEntry(token, productId, { version: cVersion.trim(), kind: cKind, notes: cNotes.trim(), date: cDate })
      setCVersion(''); setCNotes(''); setCDate(''); setCKind('added')
      reload()
    } catch (e) { notify(e instanceof Error ? e.message : t('admin.addChangelogEntryFailed'), 'warn') }
  }

  async function del(entryId: number) {
    try {
      await deleteChangelogEntry(token, productId, entryId)
      reload()
    } catch (e) { notify(e instanceof Error ? e.message : t('admin.deleteFailed'), 'warn') }
  }

  // Enter in an add-form field must add the changelog entry, not implicitly
  // submit the surrounding product <form> (which would save+close the
  // composer and discard the half-typed entry).
  function onEntryKeyDown(e: React.KeyboardEvent) {
    if (e.key === 'Enter') { e.preventDefault(); add() }
  }

  return (
    <div className="field" style={{ gridColumn: '1 / -1' }}>
      <label>{t('product.changelogHeading')}</label>
      {entries.length > 0 && (
        <div className="changelog">
          <ul>
            {entries.map(e => (
              <li key={e.id}>
                <span className={`ctag ${e.kind}`}>{e.kind}</span>
                <b>{e.version}</b> <span className="cdate mono">{e.date}</span>
                {e.notes && <p>{e.notes}</p>}
                <button type="button" className="btn btn-ghost btn-sm" aria-label={t('admin.changelogEntryDeleteAriaLabel')} onClick={() => del(e.id)}>{t('common.delete')}</button>
              </li>
            ))}
          </ul>
        </div>
      )}
      <div className="grid2">
        <div className="field">
          <label htmlFor="cl-version">{t('admin.newVersionLabel')}</label>
          <input id="cl-version" className="ipt mono" autoComplete="off" value={cVersion} onChange={e => setCVersion(e.target.value)} onKeyDown={onEntryKeyDown} />
        </div>
        <div className="field">
          <label htmlFor="cl-kind">{t('admin.changeKindLabel')}</label>
          <select id="cl-kind" className="ipt" value={cKind} onChange={e => setCKind(e.target.value)}>
            {CHANGELOG_KINDS.map(k => <option key={k} value={k}>{k}</option>)}
          </select>
        </div>
        <div className="field">
          <label htmlFor="cl-date">{t('admin.publishDateLabel')}</label>
          <input id="cl-date" type="date" className="ipt" value={cDate} onChange={e => setCDate(e.target.value)} onKeyDown={onEntryKeyDown} />
        </div>
        <div className="field" style={{ gridColumn: '1 / -1' }}>
          <label htmlFor="cl-notes">{t('admin.changelogNotesLabel')}</label>
          <input id="cl-notes" className="ipt" autoComplete="off" value={cNotes} onChange={e => setCNotes(e.target.value)} onKeyDown={onEntryKeyDown} />
        </div>
      </div>
      <button type="button" className="btn btn-ghost btn-sm" onClick={add}>{t('common.add')}</button>
    </div>
  )
}

function PlusIcon() {
  return <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} aria-hidden="true"><path d="M12 5v14M5 12h14" strokeLinecap="round" /></svg>
}

export function ProductsPage() {
  const { token } = useAuth()
  const t = useT()
  const [products, setProducts] = useState<AdminProduct[]>([])
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const pageSize = 20
  const [filter, setFilter] = useState('')
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<AdminProduct | null>(null)
  const [form, setForm] = useState<FormState>(EMPTY)
  const [slugTouched, setSlugTouched] = useState(false)
  const [modal, setModal] = useState<ModalSpec | null>(null)
  const [importOpen, setImportOpen] = useState(false)
  const [err, setErr] = useState('')
  const { toast, notify } = useToast()
  const [iconV, setIconV] = useState<number>(0)      // cache-bust after replace
  const [iconErr, setIconErr] = useState<string>('')
  const [iconBusy, setIconBusy] = useState(false)
  const [iconPreview, setIconPreview] = useState<string>('')

  // Load the Composer's draft-icon preview via an authenticated fetch (a
  // plain <img src> can't send the bearer token, and the public icon
  // endpoint is published-only) — re-runs after a fresh upload (iconV bump).
  useEffect(() => {
    if (form.icon !== 'upload' || !editing?.id || !token) { setIconPreview(''); return }
    let url = ''
    let cancelled = false
    adminFetchProductIcon(token, editing.id)
      .then(blob => { if (!cancelled) { url = URL.createObjectURL(blob); setIconPreview(url) } })
      .catch(() => { if (!cancelled) setIconPreview('') })
    return () => { cancelled = true; if (url) URL.revokeObjectURL(url) }
  }, [form.icon, editing?.id, token, iconV])

  // Monotonic guard: only the latest list request may write state, so a slow
  // response can't overwrite a fresher one after rapid mutations.
  const reqSeq = useRef(0)
  const reload = useCallback(() => {
    if (!token) return
    const seq = ++reqSeq.current
    adminGetProducts(token, { page, pageSize })
      .then(r => { if (seq === reqSeq.current) { setProducts(r.items); setTotal(r.total) } })
      .catch(() => { if (seq === reqSeq.current) setErr(t('admin.loadProductsError')) })
  }, [token, page])
  useEffect(reload, [reload])

  const categories = [...new Set(products.map(p => p.category).filter(Boolean))].sort()
  const q = filter.trim().toLowerCase()
  const shown = products.filter(p => !q || `${p.name} ${p.contextPath} ${p.category} ${p.upstreamUrl}`.toLowerCase().includes(q))

  function set<K extends keyof FormState>(k: K, v: FormState[K]) { setForm(f => ({ ...f, [k]: v })) }

  function openCreate() { setEditing(null); setForm(EMPTY); setSlugTouched(false); setIconV(0); setIconErr(''); setOpen(true) }

  function onImported(draft: AdminProduct) {
    setEditing(null)
    setForm({
      name: draft.name, slug: draft.slug, category: draft.category,
      contextPath: draft.contextPath, upstreamUrl: draft.upstreamUrl,
      sandboxUpstreamUrl: draft.sandboxUpstreamUrl ?? '',
      authType: draft.authType ?? 'key-auth',
      version: draft.version, published: false, openapiSpec: draft.openapiSpec ?? '',
      lifecycleStatus: 'active', sunsetDate: '', icon: draft.icon ?? '',
    })
    setSlugTouched(true)
    setOpen(true)
  }

  function openEdit(p: AdminProduct) {
    setEditing(p)
    setForm({ name: p.name, slug: p.slug, category: p.category, contextPath: p.contextPath, upstreamUrl: p.upstreamUrl, sandboxUpstreamUrl: p.sandboxUpstreamUrl ?? '', authType: p.authType ?? 'key-auth', version: p.version, published: p.published, openapiSpec: '', lifecycleStatus: p.lifecycleStatus ?? 'active', sunsetDate: p.sunsetDate ?? '', icon: p.icon ?? '' })
    setSlugTouched(true)
    setIconV(0); setIconErr('')
    setOpen(true)
  }

  async function submit() {
    if (!token || !form.name.trim()) return
    const slug = form.slug.trim() || slugify(form.name)
    const payload: AdminProduct = {
      ...(editing ?? { description: '', tags: [], icon: '' }),
      name: form.name.trim(),
      slug,
      category: form.category.trim(),
      contextPath: form.contextPath.trim() || `/${slug}`,
      upstreamUrl: form.upstreamUrl.trim(),
      sandboxUpstreamUrl: form.sandboxUpstreamUrl.trim(),
      authType: form.authType,
      version: form.version.trim() || '1.0.0',
      published: form.published,
      openapiSpec: form.openapiSpec,
      lifecycleStatus: form.lifecycleStatus as AdminProduct['lifecycleStatus'],
      sunsetDate: form.sunsetDate || null,
      icon: form.icon,
    }
    try {
      if (editing?.id != null) {
        await adminUpdateProduct(token, editing.id, payload)
        notify(t('admin.productSavedNotify', { name: payload.name }))
      } else {
        await adminCreateProduct(token, payload)
        notify(payload.published
          ? t('admin.productCreatedNotify', { name: payload.name })
          : t('admin.productCreatedDraftNotify', { name: payload.name }))
      }
      setOpen(false)
      reload()
    } catch (e) { notify(e instanceof Error ? e.message : t('admin.saveFailed'), 'warn') }
  }

  async function togglePub(p: AdminProduct) {
    if (!token || p.id == null) return
    try {
      await adminUpdateProduct(token, p.id, { ...p, published: !p.published })
      notify(
        p.published ? t('admin.productUnpublishedNotify', { name: p.name }) : t('admin.productPublishedNotify', { name: p.name }),
        p.published ? 'warn' : 'ok')
      reload()
    } catch (e) { notify(e instanceof Error ? e.message : t('admin.genericFailed'), 'warn') }
  }

  function askDelete(p: AdminProduct) {
    setModal({
      title: t('admin.deleteProductTitle'),
      body: t('admin.deleteProductBody', { contextPath: p.contextPath, name: p.name }),
      confirmLabel: t('common.delete'), danger: true,
      onConfirm: () => {
        if (!token || p.id == null) return
        adminDeleteProduct(token, p.id)
          .then(() => { notify(t('admin.productDeletedNotify', { name: p.name }), 'warn'); reload() })
          .catch(e => notify(
            e instanceof ApiError && e.status === 409
              ? t('admin.deleteProductConflict')
              : t('admin.deleteFailed'),
            'warn'))
      },
    })
  }

  return (
    <AdminShell
      active="products"
      title={t('admin.productsLabel')}
      description={t('admin.productsDescription')}
      counts={{ products: products.length }}
      action={
        <div className="phead-actions">
          <button className="btn btn-ghost" onClick={() => setImportOpen(true)}>{t('admin.importApi')}</button>
          <button className="btn btn-primary" onClick={() => open ? setOpen(false) : openCreate()}>
            <PlusIcon />{t('admin.newProductCta')}
          </button>
        </div>
      }
    >
      {err && <p className="autherr" role="alert">{err}</p>}

      <Composer
        open={open}
        title={editing ? t('admin.editProductTitle') : t('admin.createProductTitle')}
        hint={t('admin.productComposerHint')}
        submitLabel={editing ? t('common.save') : t('admin.createProductCta')}
        onSubmit={submit}
        onCancel={() => setOpen(false)}
        footLeft={
          <label className="switch">
            <input type="checkbox" checked={form.published} onChange={e => set('published', e.target.checked)} />
            {t('admin.publishedToCatalogLabel')}
          </label>
        }
      >
        <div className="grid2">
          <div className="field">
            <label htmlFor="f-name">{t('admin.nameLabel')}</label>
            <input id="f-name" className="ipt" placeholder={t('admin.namePlaceholderEx')} autoComplete="off" autoFocus
              value={form.name}
              onChange={e => { set('name', e.target.value); if (!slugTouched) set('slug', slugify(e.target.value)) }} />
          </div>
          <div className="field">
            <label htmlFor="f-slug">{t('admin.slugLabel')}</label>
            <input id="f-slug" className="ipt mono" placeholder={t('admin.slugPlaceholderEx')} autoComplete="off"
              value={form.slug}
              onChange={e => { setSlugTouched(true); set('slug', e.target.value) }} />
            <div className="help">{t('admin.slugHelp')}</div>
          </div>
          <div className="field">
            <label htmlFor="f-cat">{t('admin.categoryLabel')}</label>
            <input id="f-cat" className="ipt" list="cat-options" autoComplete="off"
              value={form.category} onChange={e => set('category', e.target.value)} />
            <datalist id="cat-options">
              {categories.map(c => <option key={c} value={c} />)}
            </datalist>
          </div>
          <div className="field">
            <label htmlFor="f-ctx">{t('admin.contextPathLabel')}</label>
            <input id="f-ctx" className="ipt mono" placeholder={t('admin.contextPathPlaceholderEx')} autoComplete="off"
              value={form.contextPath} onChange={e => set('contextPath', e.target.value)} />
            <div className="help">{t('admin.contextPathHelp')}</div>
          </div>
          <div className="field">
            <label htmlFor="f-up">{t('admin.upstreamLabel')} <span className="opt">{t('admin.hostPortHint')}</span></label>
            <input id="f-up" className="ipt mono" placeholder={t('admin.upstreamPlaceholderEx')} autoComplete="off"
              value={form.upstreamUrl} onChange={e => set('upstreamUrl', e.target.value)} />
          </div>
          <div className="field">
            <label htmlFor="f-sbup">{t('admin.sandboxLabel')} <span className="opt">{t('admin.hostPortOptionalHint')}</span></label>
            <input id="f-sbup" className="ipt mono" placeholder={t('admin.sandboxPlaceholderEx')}
              value={form.sandboxUpstreamUrl} onChange={e => set('sandboxUpstreamUrl', e.target.value)} />
          </div>
          <div className="field">
            <label htmlFor="f-auth">{t('admin.authMethodLabel')}</label>
            <select id="f-auth" className="ipt" value={form.authType} onChange={e => set('authType', e.target.value)}>
              <option value="key-auth">{t('admin.authKeyOption')}</option>
              <option value="oauth2">{t('admin.authOAuthOption')}</option>
            </select>
            {form.authType === 'oauth2' && (
              <p className="fieldhint">{t('admin.oauthRouteHint')}</p>
            )}
          </div>
          <div className="field">
            <label htmlFor="f-ver">{t('admin.versionLabel')}</label>
            <input id="f-ver" className="ipt mono" autoComplete="off"
              value={form.version} onChange={e => set('version', e.target.value)} />
          </div>
          <div className="field">
            <label htmlFor="f-lifecycle">{t('app.colStatus')}</label>
            <select id="f-lifecycle" className="ipt" value={form.lifecycleStatus} onChange={e => set('lifecycleStatus', e.target.value)}>
              <option value="active">{t('admin.statusActiveOption')}</option>
              <option value="deprecated">{t('lifecycleBadge.deprecated')}</option>
              <option value="sunset">{t('admin.statusSunsetOption')}</option>
            </select>
          </div>
          {form.lifecycleStatus === 'sunset' && (
            <div className="field">
              <label htmlFor="f-sunset">{t('admin.sunsetDateLabel')}</label>
              <input id="f-sunset" type="date" className="ipt" value={form.sunsetDate} onChange={e => set('sunsetDate', e.target.value)} />
            </div>
          )}
          <div className="field" style={{ gridColumn: '1 / -1' }}>
            <label>{t('admin.icon.label')}</label>
            <div className="icon-picker" data-testid="icon-picker">
              <button type="button" className="icon-tile" data-key="" aria-pressed={form.icon === ''}
                title={t('admin.icon.defaultOption')} onClick={() => set('icon', '')}>
                <span className="icon-default">—</span>
              </button>
              {BUILTIN_ICON_KEYS.map(k => (
                <button type="button" key={k} className="icon-tile" data-key={k} aria-pressed={form.icon === k}
                  title={k} onClick={() => set('icon', k)}>
                  <ApiIcon name={k} />
                </button>
              ))}
              {form.icon === 'upload' && editing && iconPreview && (
                <span className="icon-tile is-upload" aria-pressed="true">
                  <img className="ico-img" src={iconPreview} alt="" width={24} height={24} />
                </span>
              )}
            </div>
            <input
              type="file" data-testid="icon-upload" accept="image/png,image/jpeg,image/webp"
              disabled={!editing || iconBusy}
              onChange={async e => {
                const f = e.target.files?.[0]
                if (!f || !editing || editing.id == null || !token) return
                setIconErr(''); setIconBusy(true)
                try {
                  const { updatedAt } = await adminUploadProductIcon(token, editing.id, f)
                  set('icon', 'upload')
                  setIconV(new Date(updatedAt).getTime())
                } catch (err) {
                  setIconErr(err instanceof Error ? err.message : String(err))
                } finally {
                  setIconBusy(false)
                  e.target.value = ''
                }
              }}
            />
            {!editing && <p className="help" data-testid="icon-upload-hint">{t('admin.icon.uploadHint')}</p>}
            {iconBusy && <p className="help">{t('admin.icon.uploading')}</p>}
            {iconErr && <p className="autherr" role="alert">{iconErr}</p>}
          </div>
          <div className="field" style={{ gridColumn: '1 / -1' }}>
            <label htmlFor="f-spec">{t('admin.openapiSpecLabel')} <span className="opt">{t('admin.optionalTag')}</span></label>
            <input id="f-spec-file" type="file" accept=".json,.yaml,.yml"
              onChange={async e => { const f = e.target.files?.[0]; if (f) set('openapiSpec', await f.text()) }} />
            <textarea id="f-spec" className="ipt mono" rows={4} placeholder={t('admin.specPlaceholder')}
              value={form.openapiSpec} onChange={e => set('openapiSpec', e.target.value)} />
            <div className="help">{editing ? t('admin.specFileHelpEditing') : t('admin.specFileHelpCreating')}</div>
          </div>
          {editing?.id != null && token && (
            <ChangelogEditor key={editing.id} productId={editing.id} token={token} notify={notify} />
          )}
        </div>
      </Composer>

      <div className="list-head">
        <h3>{t('admin.allProductsHeading')}</h3>
        <div className="list-filter">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} aria-hidden="true"><circle cx="11" cy="11" r="7" /><path d="M21 21l-4-4" strokeLinecap="round" /></svg>
          <input type="search" placeholder={t('admin.filterProductsPlaceholder')} aria-label={t('admin.filterAriaLabel')}
            value={filter} onChange={e => setFilter(e.target.value)} />
        </div>
      </div>

      <div className="rows">
        {shown.length === 0 && (
          <div className="aempty"><h4>{t('admin.noResultsHeading')}</h4><p>{t('admin.noProductsMatchFilter')}</p></div>
        )}
        {shown.map(p => {
          const m = catMeta(p.category)
          return (
            <div className="row" key={p.id}>
              <div className="swatch" style={{ background: m.color }}>
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">{m.icon}</svg>
              </div>
              <div className="main">
                <div className="nm"><b>{p.name}</b><span className="actx">{p.contextPath}</span></div>
                <div className="meta">
                  <span className="acat" style={{ color: m.color }}>{p.category}</span>
                  <span className="asep">·</span>
                  <span className="up">{p.upstreamUrl || t('admin.noUpstream')}</span>
                  <span className="asep">·</span>
                  <span className={`apill ${p.published ? 'pub' : 'draft'}`}><span className="pdot" />{p.published ? t('admin.publishedPill') : t('admin.draftPill')}</span>
                </div>
              </div>
              <span className="ver-col">v{p.version}</span>
              <div className="actions">
                <button className="iact" title={p.published ? t('admin.unpublishAction') : t('admin.publishAction')} aria-label={p.published ? t('admin.unpublishAction') : t('admin.publishAction')} onClick={() => togglePub(p)}>
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} aria-hidden="true">
                    {p.published
                      ? <path d="M9.9 4.2A9 9 0 0121 12M14.1 19.8A9 9 0 013 12M2 2l20 20M9.9 9.9a3 3 0 004.2 4.2" strokeLinecap="round" />
                      : <><path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7-10-7-10-7z" /><circle cx="12" cy="12" r="3" /></>}
                  </svg>
                </button>
                <button className="iact" title={t('admin.editAction')} aria-label={t('admin.editAction')} onClick={() => openEdit(p)}>
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="M12 20h9M16.5 3.5a2.1 2.1 0 013 3L7 19l-4 1 1-4z" /></svg>
                </button>
                <button className="iact del" title={t('common.delete')} aria-label={t('common.delete')} onClick={() => askDelete(p)}>
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="M3 6h18M8 6V4a2 2 0 012-2h4a2 2 0 012 2v2m2 0v14a2 2 0 01-2 2H7a2 2 0 01-2-2V6" /></svg>
                </button>
              </div>
            </div>
          )
        })}
      </div>
      <Pagination page={page} pageSize={pageSize} total={total} onPage={setPage} />

      <Toast msg={toast?.msg ?? null} kind={toast?.kind} />
      <ConfirmModal spec={modal} onClose={() => setModal(null)} />
      <ImportModal open={importOpen} onClose={() => setImportOpen(false)} onImported={onImported} />
    </AdminShell>
  )
}
