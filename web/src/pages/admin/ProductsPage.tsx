import { useCallback, useEffect, useRef, useState } from 'react'
import type { AdminProduct, ChangelogEntry } from '../../api/types'
import { adminGetProducts, adminCreateProduct, adminUpdateProduct, adminDeleteProduct, adminGetChangelog, addChangelogEntry, deleteChangelogEntry, ApiError } from '../../api/client'
import { useAuth } from '../../auth/AuthProvider'
import { AdminShell } from './AdminShell'
import { Composer } from './Composer'
import { catMeta, slugify } from './meta'
import { Toast, useToast } from '../../components/Toast'
import { Pagination } from '../../components/Pagination'
import { ConfirmModal, type ModalSpec } from '../../components/ConfirmModal'
import { ImportModal } from './ImportModal'

interface FormState {
  name: string; slug: string; category: string; contextPath: string
  upstreamUrl: string; sandboxUpstreamUrl: string; authType: string; version: string; published: boolean; openapiSpec: string
  lifecycleStatus: string; sunsetDate: string
}
const EMPTY: FormState = { name: '', slug: '', category: '', contextPath: '', upstreamUrl: '', sandboxUpstreamUrl: '', authType: 'key-auth', version: '1.0.0', published: true, openapiSpec: '', lifecycleStatus: 'active', sunsetDate: '' }

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
    } catch (e) { notify(e instanceof Error ? e.message : "Échec de l'ajout au journal.", 'warn') }
  }

  async function del(entryId: number) {
    try {
      await deleteChangelogEntry(token, productId, entryId)
      reload()
    } catch (e) { notify(e instanceof Error ? e.message : 'Échec de la suppression.', 'warn') }
  }

  // Enter in an add-form field must add the changelog entry, not implicitly
  // submit the surrounding product <form> (which would save+close the
  // composer and discard the half-typed entry).
  function onEntryKeyDown(e: React.KeyboardEvent) {
    if (e.key === 'Enter') { e.preventDefault(); add() }
  }

  return (
    <div className="field" style={{ gridColumn: '1 / -1' }}>
      <label>Journal des modifications</label>
      {entries.length > 0 && (
        <div className="changelog">
          <ul>
            {entries.map(e => (
              <li key={e.id}>
                <span className={`ctag ${e.kind}`}>{e.kind}</span>
                <b>{e.version}</b> <span className="cdate mono">{e.date}</span>
                {e.notes && <p>{e.notes}</p>}
                <button type="button" className="btn btn-ghost btn-sm" aria-label="Supprimer une entrée du journal" onClick={() => del(e.id)}>Supprimer</button>
              </li>
            ))}
          </ul>
        </div>
      )}
      <div className="grid2">
        <div className="field">
          <label htmlFor="cl-version">Nouvelle version</label>
          <input id="cl-version" className="ipt mono" autoComplete="off" value={cVersion} onChange={e => setCVersion(e.target.value)} onKeyDown={onEntryKeyDown} />
        </div>
        <div className="field">
          <label htmlFor="cl-kind">Type de changement</label>
          <select id="cl-kind" className="ipt" value={cKind} onChange={e => setCKind(e.target.value)}>
            {CHANGELOG_KINDS.map(k => <option key={k} value={k}>{k}</option>)}
          </select>
        </div>
        <div className="field">
          <label htmlFor="cl-date">Date de publication</label>
          <input id="cl-date" type="date" className="ipt" value={cDate} onChange={e => setCDate(e.target.value)} onKeyDown={onEntryKeyDown} />
        </div>
        <div className="field" style={{ gridColumn: '1 / -1' }}>
          <label htmlFor="cl-notes">Notes du changelog</label>
          <input id="cl-notes" className="ipt" autoComplete="off" value={cNotes} onChange={e => setCNotes(e.target.value)} onKeyDown={onEntryKeyDown} />
        </div>
      </div>
      <button type="button" className="btn btn-ghost btn-sm" onClick={add}>Ajouter</button>
    </div>
  )
}

function PlusIcon() {
  return <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} aria-hidden="true"><path d="M12 5v14M5 12h14" strokeLinecap="round" /></svg>
}

export function ProductsPage() {
  const { token } = useAuth()
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

  // Monotonic guard: only the latest list request may write state, so a slow
  // response can't overwrite a fresher one after rapid mutations.
  const reqSeq = useRef(0)
  const reload = useCallback(() => {
    if (!token) return
    const seq = ++reqSeq.current
    adminGetProducts(token, { page, pageSize })
      .then(r => { if (seq === reqSeq.current) { setProducts(r.items); setTotal(r.total) } })
      .catch(() => { if (seq === reqSeq.current) setErr('Impossible de charger les produits.') })
  }, [token, page])
  useEffect(reload, [reload])

  const categories = [...new Set(products.map(p => p.category).filter(Boolean))].sort()
  const q = filter.trim().toLowerCase()
  const shown = products.filter(p => !q || `${p.name} ${p.contextPath} ${p.category} ${p.upstreamUrl}`.toLowerCase().includes(q))

  function set<K extends keyof FormState>(k: K, v: FormState[K]) { setForm(f => ({ ...f, [k]: v })) }

  function openCreate() { setEditing(null); setForm(EMPTY); setSlugTouched(false); setOpen(true) }

  function onImported(draft: AdminProduct) {
    setEditing(null)
    setForm({
      name: draft.name, slug: draft.slug, category: draft.category,
      contextPath: draft.contextPath, upstreamUrl: draft.upstreamUrl,
      sandboxUpstreamUrl: draft.sandboxUpstreamUrl ?? '',
      authType: draft.authType ?? 'key-auth',
      version: draft.version, published: false, openapiSpec: draft.openapiSpec ?? '',
      lifecycleStatus: 'active', sunsetDate: '',
    })
    setSlugTouched(true)
    setOpen(true)
  }

  function openEdit(p: AdminProduct) {
    setEditing(p)
    setForm({ name: p.name, slug: p.slug, category: p.category, contextPath: p.contextPath, upstreamUrl: p.upstreamUrl, sandboxUpstreamUrl: p.sandboxUpstreamUrl ?? '', authType: p.authType ?? 'key-auth', version: p.version, published: p.published, openapiSpec: '', lifecycleStatus: p.lifecycleStatus ?? 'active', sunsetDate: p.sunsetDate ?? '' })
    setSlugTouched(true)
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
    }
    try {
      if (editing?.id != null) {
        await adminUpdateProduct(token, editing.id, payload)
        notify(`${payload.name} enregistré`)
      } else {
        await adminCreateProduct(token, payload)
        notify(`${payload.name} créé${payload.published ? '' : ' (brouillon)'}`)
      }
      setOpen(false)
      reload()
    } catch (e) { notify(e instanceof Error ? e.message : "Échec de l'enregistrement.", 'warn') }
  }

  async function togglePub(p: AdminProduct) {
    if (!token || p.id == null) return
    try {
      await adminUpdateProduct(token, p.id, { ...p, published: !p.published })
      notify(`${p.name}${p.published ? ' retiré du catalogue' : ' publié au catalogue'}`, p.published ? 'warn' : 'ok')
      reload()
    } catch (e) { notify(e instanceof Error ? e.message : 'Échec.', 'warn') }
  }

  function askDelete(p: AdminProduct) {
    setModal({
      title: 'Supprimer le produit ?',
      body: `La route APISIX ${p.contextPath} de ${p.name} sera retirée de la gateway. Les abonnements liés seront révoqués.`,
      confirmLabel: 'Supprimer', danger: true,
      onConfirm: () => {
        if (!token || p.id == null) return
        adminDeleteProduct(token, p.id)
          .then(() => { notify(`${p.name} supprimé`, 'warn'); reload() })
          .catch(e => notify(
            e instanceof ApiError && e.status === 409
              ? 'Suppression impossible : abonnements actifs.'
              : 'Échec de la suppression.',
            'warn'))
      },
    })
  }

  return (
    <AdminShell
      active="products"
      title="Produits"
      description="Les produits exposent vos services en amont (upstream) à travers la passerelle APISIX, avec un contexte de routage et une version publiables au catalogue développeur."
      counts={{ products: products.length }}
      action={
        <div className="phead-actions">
          <button className="btn btn-ghost" onClick={() => setImportOpen(true)}>Importer une API</button>
          <button className="btn btn-primary" onClick={() => open ? setOpen(false) : openCreate()}>
            <PlusIcon />Nouveau produit
          </button>
        </div>
      }
    >
      {err && <p className="autherr" role="alert">{err}</p>}

      <Composer
        open={open}
        title={editing ? 'Modifier le produit' : 'Créer un produit'}
        hint="Le routage APISIX est appliqué à la publication"
        submitLabel={editing ? 'Enregistrer' : 'Créer le produit'}
        onSubmit={submit}
        onCancel={() => setOpen(false)}
        footLeft={
          <label className="switch">
            <input type="checkbox" checked={form.published} onChange={e => set('published', e.target.checked)} />
            Publié au catalogue
          </label>
        }
      >
        <div className="grid2">
          <div className="field">
            <label htmlFor="f-name">Nom</label>
            <input id="f-name" className="ipt" placeholder="CurrencyConverterAPI" autoComplete="off" autoFocus
              value={form.name}
              onChange={e => { set('name', e.target.value); if (!slugTouched) set('slug', slugify(e.target.value)) }} />
          </div>
          <div className="field">
            <label htmlFor="f-slug">Slug</label>
            <input id="f-slug" className="ipt mono" placeholder="currency-converter" autoComplete="off"
              value={form.slug}
              onChange={e => { setSlugTouched(true); set('slug', e.target.value) }} />
            <div className="help">généré depuis le nom — modifiable</div>
          </div>
          <div className="field">
            <label htmlFor="f-cat">Catégorie</label>
            <input id="f-cat" className="ipt" list="cat-options" autoComplete="off"
              value={form.category} onChange={e => set('category', e.target.value)} />
            <datalist id="cat-options">
              {categories.map(c => <option key={c} value={c} />)}
            </datalist>
          </div>
          <div className="field">
            <label htmlFor="f-ctx">Context path</label>
            <input id="f-ctx" className="ipt mono" placeholder="/currencyconv" autoComplete="off"
              value={form.contextPath} onChange={e => set('contextPath', e.target.value)} />
            <div className="help">préfixe de route exposé par la gateway</div>
          </div>
          <div className="field">
            <label htmlFor="f-up">Upstream <span className="opt">host:port</span></label>
            <input id="f-up" className="ipt mono" placeholder="echo:8080" autoComplete="off"
              value={form.upstreamUrl} onChange={e => set('upstreamUrl', e.target.value)} />
          </div>
          <div className="field">
            <label htmlFor="f-sbup">Sandbox <span className="opt">host:port — optionnel</span></label>
            <input id="f-sbup" className="ipt mono" placeholder="ex. sandbox.example.com:443"
              value={form.sandboxUpstreamUrl} onChange={e => set('sandboxUpstreamUrl', e.target.value)} />
          </div>
          <div className="field">
            <label htmlFor="f-auth">Méthode d'authentification</label>
            <select id="f-auth" className="ipt" value={form.authType} onChange={e => set('authType', e.target.value)}>
              <option value="key-auth">Clé API (key-auth)</option>
              <option value="oauth2">OAuth2 (OIDC)</option>
            </select>
            {form.authType === 'oauth2' && (
              <p className="fieldhint">Les routes OAuth2 valident les jetons Bearer auprès de l'émetteur OIDC configuré ; les abonnés s'authentifient avec leur propre client.</p>
            )}
          </div>
          <div className="field">
            <label htmlFor="f-ver">Version</label>
            <input id="f-ver" className="ipt mono" autoComplete="off"
              value={form.version} onChange={e => set('version', e.target.value)} />
          </div>
          <div className="field">
            <label htmlFor="f-lifecycle">Statut</label>
            <select id="f-lifecycle" className="ipt" value={form.lifecycleStatus} onChange={e => set('lifecycleStatus', e.target.value)}>
              <option value="active">Actif</option>
              <option value="deprecated">Déprécié</option>
              <option value="sunset">Sunset</option>
            </select>
          </div>
          {form.lifecycleStatus === 'sunset' && (
            <div className="field">
              <label htmlFor="f-sunset">Date de retrait</label>
              <input id="f-sunset" type="date" className="ipt" value={form.sunsetDate} onChange={e => set('sunsetDate', e.target.value)} />
            </div>
          )}
          <div className="field" style={{ gridColumn: '1 / -1' }}>
            <label htmlFor="f-spec">Spécification OpenAPI <span className="opt">optionnel</span></label>
            <input id="f-spec-file" type="file" accept=".json,.yaml,.yml"
              onChange={async e => { const f = e.target.files?.[0]; if (f) set('openapiSpec', await f.text()) }} />
            <textarea id="f-spec" className="ipt mono" rows={4} placeholder="Collez une spec OpenAPI 3.x / Swagger 2.0…"
              value={form.openapiSpec} onChange={e => set('openapiSpec', e.target.value)} />
            <div className="help">{editing ? 'Laissez vide pour conserver la spécification existante.' : 'Alimente la documentation et le « Essayer » du produit.'}</div>
          </div>
          {editing?.id != null && token && (
            <ChangelogEditor key={editing.id} productId={editing.id} token={token} notify={notify} />
          )}
        </div>
      </Composer>

      <div className="list-head">
        <h3>Tous les produits</h3>
        <div className="list-filter">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} aria-hidden="true"><circle cx="11" cy="11" r="7" /><path d="M21 21l-4-4" strokeLinecap="round" /></svg>
          <input type="search" placeholder="Filtrer les produits…" aria-label="Filtrer"
            value={filter} onChange={e => setFilter(e.target.value)} />
        </div>
      </div>

      <div className="rows">
        {shown.length === 0 && (
          <div className="aempty"><h4>Aucun résultat</h4><p>Aucun produit ne correspond à ce filtre.</p></div>
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
                  <span className="up">{p.upstreamUrl || 'pas d’upstream'}</span>
                  <span className="asep">·</span>
                  <span className={`apill ${p.published ? 'pub' : 'draft'}`}><span className="pdot" />{p.published ? 'publié' : 'brouillon'}</span>
                </div>
              </div>
              <span className="ver-col">v{p.version}</span>
              <div className="actions">
                <button className="iact" title={p.published ? 'Dépublier' : 'Publier'} aria-label={p.published ? 'Dépublier' : 'Publier'} onClick={() => togglePub(p)}>
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} aria-hidden="true">
                    {p.published
                      ? <path d="M9.9 4.2A9 9 0 0121 12M14.1 19.8A9 9 0 013 12M2 2l20 20M9.9 9.9a3 3 0 004.2 4.2" strokeLinecap="round" />
                      : <><path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7-10-7-10-7z" /><circle cx="12" cy="12" r="3" /></>}
                  </svg>
                </button>
                <button className="iact" title="Modifier" aria-label="Modifier" onClick={() => openEdit(p)}>
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="M12 20h9M16.5 3.5a2.1 2.1 0 013 3L7 19l-4 1 1-4z" /></svg>
                </button>
                <button className="iact del" title="Supprimer" aria-label="Supprimer" onClick={() => askDelete(p)}>
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
