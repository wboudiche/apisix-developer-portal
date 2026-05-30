import { useEffect, useState } from 'react'
import type { AdminProduct } from '../api/types'
import { adminGetProducts, adminCreateProduct, adminUpdateProduct, adminDeleteProduct } from '../api/client'
import { useAuth } from '../auth/AuthProvider'
import { AdminNav } from '../components/AdminNav'
import '../styles/catalog.css'

const empty: AdminProduct = { name: '', slug: '', category: '', version: '', contextPath: '', description: '', tags: [], icon: '', upstreamUrl: '', published: true }

const field: React.CSSProperties = { height: 38, padding: '0 12px', border: '1px solid var(--border-2)', borderRadius: 10, background: 'var(--bg)', color: 'var(--fg)' }

export function AdminProductsPage() {
  const { token } = useAuth()
  const [products, setProducts] = useState<AdminProduct[]>([])
  const [form, setForm] = useState<AdminProduct>(empty)
  const [editingId, setEditingId] = useState<number | null>(null)
  const [err, setErr] = useState('')

  function reload() {
    if (!token) return
    adminGetProducts(token).then(setProducts).catch(() => setErr('Impossible de charger les produits.'))
  }
  useEffect(reload, [token])

  function set<K extends keyof AdminProduct>(k: K, v: AdminProduct[K]) { setForm(f => ({ ...f, [k]: v })) }

  async function onSubmit() {
    if (!token) return
    setErr('')
    try {
      if (editingId == null) await adminCreateProduct(token, form)
      else await adminUpdateProduct(token, editingId, form)
      setForm(empty); setEditingId(null); reload()
    } catch (e) { setErr(e instanceof Error ? e.message : "Échec de l'enregistrement.") }
  }

  function onEdit(p: AdminProduct) { setEditingId(p.id ?? null); setForm({ ...p }) }

  async function onDelete(p: AdminProduct) {
    if (!token || p.id == null) return
    setErr('')
    try { await adminDeleteProduct(token, p.id); reload() }
    catch (e) { setErr(e instanceof Error ? e.message : 'Échec de la suppression.') }
  }

  return (
    <>
      <AdminNav active="products" />
      <div className="content">
        <div className="chead"><div className="titlewrap"><h1>Produits</h1></div></div>
        {err && <p className="autherr" role="alert">{err}</p>}

        <div className="card" style={{ padding: 18, marginBottom: 22, display: 'grid', gap: 10, gridTemplateColumns: '1fr 1fr' }}>
          <label style={{ display: 'grid', gap: 4 }}>Nom<input aria-label="Nom" style={field} value={form.name} onChange={e => set('name', e.target.value)} /></label>
          <label style={{ display: 'grid', gap: 4 }}>Slug<input aria-label="Slug" style={field} value={form.slug} onChange={e => set('slug', e.target.value)} /></label>
          <label style={{ display: 'grid', gap: 4 }}>Catégorie<input aria-label="Catégorie" style={field} value={form.category} onChange={e => set('category', e.target.value)} /></label>
          <label style={{ display: 'grid', gap: 4 }}>Context path<input aria-label="Context path" style={field} value={form.contextPath} onChange={e => set('contextPath', e.target.value)} /></label>
          <label style={{ display: 'grid', gap: 4 }}>Upstream (host:port)<input aria-label="Upstream" style={field} value={form.upstreamUrl} onChange={e => set('upstreamUrl', e.target.value)} /></label>
          <label style={{ display: 'grid', gap: 4 }}>Version<input aria-label="Version" style={field} value={form.version} onChange={e => set('version', e.target.value)} placeholder="1.0.0" /></label>
          <label style={{ gridColumn: '1 / -1', display: 'flex', gap: 8, alignItems: 'center' }}>
            <input type="checkbox" aria-label="Publié" checked={form.published} onChange={e => set('published', e.target.checked)} /> Publié
          </label>
          <div style={{ gridColumn: '1 / -1', display: 'flex', gap: 10 }}>
            <button className="subbtn" onClick={onSubmit}>{editingId == null ? 'Créer le produit' : 'Enregistrer'}</button>
            {editingId != null && <button className="subbtn ghost" onClick={() => { setForm(empty); setEditingId(null) }}>Annuler</button>}
          </div>
        </div>

        {products.length === 0 && <p className="rescount">Aucun produit.</p>}
        {products.map(p => (
          <div key={p.id} className="cfoot" style={{ justifyContent: 'space-between', padding: '10px 0', borderBottom: '1px solid var(--border)' }}>
            <span>
              <b>{p.name}</b> <span className="ctx">{p.contextPath}</span>
              {' · '}<span className="pill">{p.upstreamUrl || "pas d'upstream"}</span>
              {' · '}<span className="pill">{p.published ? 'publié' : 'masqué'}</span>
            </span>
            <span style={{ display: 'flex', gap: 8 }}>
              <button className="subbtn ghost" onClick={() => onEdit(p)} aria-label={`Modifier ${p.name}`}>Modifier</button>
              <button className="subbtn ghost" onClick={() => onDelete(p)} aria-label={`Supprimer ${p.name}`}>Supprimer</button>
            </span>
          </div>
        ))}
      </div>
    </>
  )
}
